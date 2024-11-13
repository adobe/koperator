// Copyright © 2020 Cisco Systems, Inc. and/or its affiliates
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

package contouringress

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"emperror.dev/errors"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	contour "github.com/projectcontour/contour/apis/projectcontour/v1"

	apiutil "github.com/banzaicloud/koperator/api/util"
	"github.com/banzaicloud/koperator/api/v1beta1"
	"github.com/banzaicloud/koperator/pkg/k8sutil"
	"github.com/banzaicloud/koperator/pkg/resources"
	"github.com/banzaicloud/koperator/pkg/resources/templates"
	"github.com/banzaicloud/koperator/pkg/util"
	contourutils "github.com/banzaicloud/koperator/pkg/util/contour"
	"github.com/banzaicloud/koperator/pkg/util/kafka"
)

const (
	componentName = "clusterIpExternalAccess"
)

// Reconciler implements the Component Reconciler
type Reconciler struct {
	resources.Reconciler
}

// New creates a new reconciler for NodePort based external access
func New(client client.Client, cluster *v1beta1.KafkaCluster) *Reconciler {
	return &Reconciler{
		Reconciler: resources.Reconciler{
			Client:       client,
			KafkaCluster: cluster,
		},
	}
}

// Reconcile implements the reconcile logic for NodePort based external access
func (r *Reconciler) Reconcile(log logr.Logger) error {
	log = log.WithValues("component", componentName)
	log.V(1).Info("Reconciling")
	var reconcileObjects []runtime.Object
	// create ClusterIP services for discovery service and brokers
	for _, eListener := range r.KafkaCluster.Spec.ListenersConfig.ExternalListeners {
		if r.KafkaCluster.Spec.GetIngressController() == contourutils.IngressControllerName && eListener.GetAccessMethod() == corev1.ServiceTypeClusterIP {
			// create per ingressConfig services ClusterIP
			ingressConfigs, defaultControllerName, err := util.GetIngressConfigs(r.KafkaCluster.Spec, eListener)
			if err != nil {
				return err
			}
			for name, ingressConfig := range ingressConfigs {
				if !util.IsIngressConfigInUse(name, defaultControllerName, r.KafkaCluster, log) {
					continue
				}

				clusterService := r.clusterService(log, eListener, ingressConfig, name, defaultControllerName)
				reconcileObjects = append(reconcileObjects, clusterService)

				// make sure the HostnameOverride is set otherwise the fqdn will be empty and HTTPProxy creation will fail.
				fqdn := ingressConfig.HostnameOverride
				ingressRoute := r.httpProxy(log, eListener, fqdn, ingressConfig, clusterService)
				reconcileObjects = append(reconcileObjects, ingressRoute)

				// create per broker services ClusterIP
				for _, broker := range r.KafkaCluster.Spec.Brokers {
					brokerService := r.brokerService(log, broker.Id, eListener)
					reconcileObjects = append(reconcileObjects, brokerService)

					fqdn := ingressConfig.ContourIngressConfig.GetBrokerFqdn(broker.Id)
					ingressRoute := r.httpProxy(log, eListener, fqdn, ingressConfig, brokerService)
					reconcileObjects = append(reconcileObjects, ingressRoute)
				}
			}

			for _, obj := range reconcileObjects {
				err = k8sutil.Reconcile(log, r.Client, obj, r.KafkaCluster)
				if err != nil {
					return err
				}
			}
		} else if r.KafkaCluster.Spec.RemoveUnusedIngressResources {
			// Cleaning up unused contour resources when ingress controller is not contour or externalListener access method is not ClusterIP
			deletionCounter := 0
			ctx := context.Background()
			contourResourcesGVK := []schema.GroupVersionKind{
				{
					Version: corev1.SchemeGroupVersion.Version,
					Group:   corev1.SchemeGroupVersion.Group,
					Kind:    reflect.TypeOf(corev1.Service{}).Name(),
				},
				{
					Version: corev1.SchemeGroupVersion.Version,
					Group:   corev1.SchemeGroupVersion.Group,
					Kind:    reflect.TypeOf(contour.HTTPProxy{}).Name(),
				},
			}
			var contourResources unstructured.UnstructuredList
			for _, gvk := range contourResourcesGVK {
				contourResources.SetGroupVersionKind(gvk)

				if err := r.List(ctx, &contourResources, client.InNamespace(r.KafkaCluster.GetNamespace()),
					client.MatchingLabels(labelsForContourIngressWithoutEListenerName(r.KafkaCluster.Name))); err != nil {
					return errors.Wrap(err, "error when getting list of envoy ingress resources for deletion")
				}

				for _, removeObject := range contourResources.Items {
					if !strings.Contains(removeObject.GetLabels()[util.ExternalListenerLabelNameKey], eListener.Name) ||
						util.ObjectManagedByClusterRegistry(&removeObject) ||
						!removeObject.GetDeletionTimestamp().IsZero() {
						continue
					}
					if err := r.Delete(ctx, &removeObject); client.IgnoreNotFound(err) != nil {
						return errors.Wrap(err, "error when removing contour ingress resources")
					}
					log.V(1).Info(fmt.Sprintf("Deleted contour ingress '%s' resource '%s' for externalListener '%s'", gvk.Kind, removeObject.GetName(), eListener.Name))
					deletionCounter++
				}
			}
			if deletionCounter > 0 {
				log.Info(fmt.Sprintf("Removed '%d' resources for contour ingress", deletionCounter))
			}
		}
	}

	log.V(1).Info("Reconciled")

	return nil
}

// generate service for broker
func (r *Reconciler) brokerService(_ logr.Logger, id int32, extListener v1beta1.ExternalListenerConfig) runtime.Object {
	service := &corev1.Service{
		ObjectMeta: templates.ObjectMetaWithAnnotations(
			fmt.Sprintf(kafka.NodePortServiceTemplate, r.KafkaCluster.GetName(), id, extListener.Name),
			apiutil.MergeLabels(
				apiutil.LabelsForKafka(r.KafkaCluster.Name),
				map[string]string{v1beta1.BrokerIdLabelKey: fmt.Sprintf("%d", id)},
				labelsForContourIngress(r.KafkaCluster.Name, extListener.Name)),
			extListener.GetServiceAnnotations(), r.KafkaCluster),
		Spec: corev1.ServiceSpec{
			Selector: apiutil.MergeLabels(apiutil.LabelsForKafka(r.KafkaCluster.Name),
				map[string]string{v1beta1.BrokerIdLabelKey: fmt.Sprintf("%d", id)}),
			Type: corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{{
				Name:       fmt.Sprintf("broker-%d", id),
				Port:       *extListener.AnyCastPort,
				TargetPort: intstr.FromInt(int(extListener.ContainerPort)),
				Protocol:   corev1.ProtocolTCP,
			},
			},
			ExternalTrafficPolicy: extListener.ExternalTrafficPolicy,
		},
	}

	return service
}

// generate service for anycast port
func (r *Reconciler) clusterService(_ logr.Logger, extListener v1beta1.ExternalListenerConfig,
	ingressConfig v1beta1.IngressConfig, ingressConfigName, _ string) runtime.Object {
	var serviceName string = util.GenerateEnvoyResourceName(contourutils.ContourServiceName, contourutils.ContourServiceNameWithScope,
		extListener, ingressConfig, ingressConfigName, r.KafkaCluster.GetName())

	service := &corev1.Service{
		ObjectMeta: templates.ObjectMetaWithAnnotations(
			serviceName,
			apiutil.MergeLabels(
				apiutil.LabelsForKafka(r.KafkaCluster.Name),
				labelsForContourIngress(r.KafkaCluster.Name, extListener.Name)),
			extListener.GetServiceAnnotations(), r.KafkaCluster),
		Spec: corev1.ServiceSpec{
			Selector: apiutil.MergeLabels(apiutil.LabelsForKafka(r.KafkaCluster.Name)),
			Type:     corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{{
				Name:       "tcp-all-broker",
				Port:       *extListener.AnyCastPort,
				TargetPort: intstr.FromInt(int(extListener.ContainerPort)),
				Protocol:   corev1.ProtocolTCP,
			},
			},
			ExternalTrafficPolicy: extListener.ExternalTrafficPolicy,
		},
	}

	return service
}

// generate httproxy resource for contour ingress
func (r *Reconciler) httpProxy(_ logr.Logger, extListener v1beta1.ExternalListenerConfig, fqdn string,
	ingressConfig v1beta1.IngressConfig, service runtime.Object) runtime.Object {
	svc := service.(*corev1.Service)
	ingressRoute := &contour.HTTPProxy{
		ObjectMeta: templates.ObjectMetaWithAnnotations(fqdn,
			apiutil.MergeLabels(
				apiutil.LabelsForKafka(r.KafkaCluster.Name),
				labelsForContourIngress(r.KafkaCluster.Name, extListener.Name)),
			extListener.GetServiceAnnotations(), r.KafkaCluster),
		Spec: contour.HTTPProxySpec{
			VirtualHost: &contour.VirtualHost{
				Fqdn: fqdn,
				TLS: &contour.TLS{
					SecretName: ingressConfig.ContourIngressConfig.TLSSecretName,
				},
			},
			TCPProxy: &contour.TCPProxy{
				Services: []contour.Service{{
					Name: svc.GetName(),
					Port: int(svc.Spec.Ports[0].Port),
				}},
			},
		},
	}

	return ingressRoute
}

func labelsForContourIngress(crName, eLName string) map[string]string {
	return apiutil.MergeLabels(labelsForContourIngressWithoutEListenerName(crName), map[string]string{util.ExternalListenerLabelNameKey: eLName})
}

func labelsForContourIngressWithoutEListenerName(crName string) map[string]string {
	return map[string]string{v1beta1.AppLabelKey: "contouringress", v1beta1.KafkaCRLabelKey: crName}
}
