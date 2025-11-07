// Copyright 2025 Adobe. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package envoygateway

import (
	"context"
	"fmt"
	"strings"

	"emperror.dev/errors"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	apiutil "github.com/banzaicloud/koperator/api/util"
	"github.com/banzaicloud/koperator/api/v1beta1"
	"github.com/banzaicloud/koperator/pkg/k8sutil"
	"github.com/banzaicloud/koperator/pkg/resources"
	"github.com/banzaicloud/koperator/pkg/util"
	envoygatewayutils "github.com/banzaicloud/koperator/pkg/util/envoygateway"
)

const (
	componentName = "envoygateway"
)

// labelsForEnvoyGateway returns the labels for selecting the resources
// belonging to the given kafka CR name.
func labelsForEnvoyGateway(crName, eLName string) map[string]string {
	return apiutil.MergeLabels(labelsForEnvoyGatewayWithoutEListenerName(crName), map[string]string{util.ExternalListenerLabelNameKey: eLName})
}

func labelsForEnvoyGatewayWithoutEListenerName(crName string) map[string]string {
	return map[string]string{v1beta1.AppLabelKey: "envoygateway", v1beta1.KafkaCRLabelKey: crName}
}

// Reconciler implements the Component Reconciler
type Reconciler struct {
	resources.Reconciler
}

// New creates a new reconciler for Envoy Gateway
func New(client client.Client, cluster *v1beta1.KafkaCluster) *Reconciler {
	return &Reconciler{
		Reconciler: resources.Reconciler{
			Client:       client,
			KafkaCluster: cluster,
		},
	}
}

// Reconcile implements the reconcile logic for Envoy Gateway
func (r *Reconciler) Reconcile(log logr.Logger) error {
	log = log.WithValues("component", componentName)

	log.V(1).Info("Reconciling")
	for _, eListener := range r.KafkaCluster.Spec.ListenersConfig.ExternalListeners {
		if r.KafkaCluster.Spec.GetIngressController() == envoygatewayutils.IngressControllerName && eListener.GetAccessMethod() == corev1.ServiceTypeLoadBalancer {
			ingressConfigs, defaultControllerName, err := util.GetIngressConfigs(r.KafkaCluster.Spec, eListener)
			if err != nil {
				return err
			}

			for name, ingressConfig := range ingressConfigs {
				if !util.IsIngressConfigInUse(name, defaultControllerName, r.KafkaCluster, log) {
					continue
				}

				// Validate TLS configuration for envoygateway
				// EnvoyGateway ONLY supports TLS termination at the gateway level
				if eListener.TLSEnabled() {
					if ingressConfig.EnvoyGatewayConfig == nil || ingressConfig.EnvoyGatewayConfig.TLSSecretName == "" {
						return errors.New("envoygateway ingress controller requires TLSSecretName to be set in envoyGatewayConfig when TLS is enabled (externalStartingPort == -1). EnvoyGateway only supports TLS termination at the gateway level")
					}
				}

				// Create Gateway resource
				gateway := r.gateway(eListener, ingressConfig)
				err := k8sutil.Reconcile(log, r.Client, gateway, r.KafkaCluster)
				if err != nil {
					return err
				}

				// Create TCPRoute for each broker
				// Note: We always use TCPRoute because EnvoyGateway performs TLS termination
				// at the gateway level, so traffic to backends is plain TCP
				for _, broker := range r.KafkaCluster.Spec.Brokers {
					route := r.tcpRoute(broker.Id, eListener, ingressConfig)
					err := k8sutil.Reconcile(log, r.Client, route, r.KafkaCluster)
					if err != nil {
						return err
					}
				}

				// Create TCPRoute for anycast (all-broker) service
				anyCastRoute := r.tcpRouteAllBroker(eListener, ingressConfig)
				err = k8sutil.Reconcile(log, r.Client, anyCastRoute, r.KafkaCluster)
				if err != nil {
					return err
				}
			}
		} else if r.KafkaCluster.Spec.RemoveUnusedIngressResources {
			// Cleaning up unused envoy gateway resources when ingress controller is not envoygateway or externalListener access method is not LoadBalancer
			deletionCounter := 0
			ctx := context.Background()
			envoyGatewayResourcesGVK := []schema.GroupVersionKind{
				{
					Version: gatewayv1.GroupVersion.Version,
					Group:   gatewayv1.GroupVersion.Group,
					Kind:    "Gateway",
				},
				{
					Version: "v1alpha2",
					Group:   gatewayv1.GroupVersion.Group,
					Kind:    "TLSRoute",
				},
				{
					Version: "v1alpha2",
					Group:   gatewayv1.GroupVersion.Group,
					Kind:    "TCPRoute",
				},
			}

			for _, gvk := range envoyGatewayResourcesGVK {
				var envoyGatewayResources unstructured.UnstructuredList
				envoyGatewayResources.SetGroupVersionKind(gvk)
				err := r.List(ctx, &envoyGatewayResources,
					client.InNamespace(r.KafkaCluster.Namespace),
					client.MatchingLabels(labelsForEnvoyGatewayWithoutEListenerName(r.KafkaCluster.Name)))
				if err != nil {
					return errors.WrapIfWithDetails(err, "failed to list envoy gateway resources", "gvk", gvk)
				}

				for _, removeObject := range envoyGatewayResources.Items {
					if !strings.Contains(removeObject.GetLabels()[util.ExternalListenerLabelNameKey], eListener.Name) ||
						util.ObjectManagedByClusterRegistry(&removeObject) ||
						!removeObject.GetDeletionTimestamp().IsZero() {
						continue
					}
					if err := r.Delete(ctx, &removeObject); client.IgnoreNotFound(err) != nil {
						return errors.Wrap(err, "error when removing envoy gateway ingress resources")
					}
					log.V(1).Info(fmt.Sprintf("Deleted envoy gateway ingress '%s' resource '%s' for externalListener '%s'", gvk.Kind, removeObject.GetName(), eListener.Name))
					deletionCounter++
				}
			}
			if deletionCounter > 0 {
				log.Info(fmt.Sprintf("Removed '%d' resources for envoy gateway ingress", deletionCounter))
			}
		}
	}
	log.V(1).Info("Reconciled")

	return nil
}
