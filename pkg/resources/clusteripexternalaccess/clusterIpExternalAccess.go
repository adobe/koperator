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

package clusteripexternalaccess

import (
	"context"
	"fmt"

	"emperror.dev/errors"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/banzaicloud/koperator/api/v1beta1"
	"github.com/banzaicloud/koperator/pkg/k8sutil"
	"github.com/banzaicloud/koperator/pkg/resources"
	"github.com/banzaicloud/koperator/pkg/util"
	contourutils "github.com/banzaicloud/koperator/pkg/util/contour"
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
	if r.KafkaCluster.Spec.GetIngressController() == contourutils.IngressControllerName {
		// create ClusterIP services for discovery service and brokers
		for _, eListener := range r.KafkaCluster.Spec.ListenersConfig.ExternalListeners {
			// create per ingressConfig services ClusterIP
			ingressConfigs, defaultControllerName, err := util.GetIngressConfigs(r.KafkaCluster.Spec, eListener)
			if err != nil {
				return err
			}
			for name, ingressConfig := range ingressConfigs {
				if !util.IsIngressConfigInUse(name, defaultControllerName, r.KafkaCluster, log) {
					continue
				}
				// TODO cleanup when RemoveUnusedIngressResources
				clusterService := r.clusterService(log, eListener, ingressConfig, name, defaultControllerName)
				err = k8sutil.Reconcile(log, r.Client, clusterService, r.KafkaCluster)
				if err != nil {
					return err
				}

				// create IngressRoutes for each ingressConfig
				fqdn := ingressConfig.ContourIngressConfig.GetAnycastFqdn()
				ingressRoute := r.ingressRoute(log, eListener, fqdn, ingressConfig, clusterService)
				err = k8sutil.Reconcile(log, r.Client, ingressRoute, r.KafkaCluster)
				if err != nil {
					return err
				}
				// create per broker services ClusterIP
				for _, broker := range r.KafkaCluster.Spec.Brokers {
					service := r.brokerService(log, broker.Id, eListener)

					fqdn := ingressConfig.ContourIngressConfig.GetBrokerFqdn(broker.Id)
					ingressRoute := r.ingressRoute(log, eListener, fqdn, ingressConfig, service)

					if eListener.GetAccessMethod() == corev1.ServiceTypeClusterIP {
						err = k8sutil.Reconcile(log, r.Client, service, r.KafkaCluster)
						if err != nil {
							return err
						}
						err = k8sutil.Reconcile(log, r.Client, ingressRoute, r.KafkaCluster)
						if err != nil {
							return err
						}
					} else if r.KafkaCluster.Spec.RemoveUnusedIngressResources {
						// Cleaning up unused nodeport services
						removeService := service.(client.Object)
						if err := r.Delete(context.Background(), removeService); client.IgnoreNotFound(err) != nil {
							return errors.Wrap(err, "error when removing unused nodeport services")
						}
						removeIngress := ingressRoute.(client.Object)
						if err := r.Delete(context.Background(), removeIngress); client.IgnoreNotFound(err) != nil {
							return errors.Wrap(err, "error when removing unused nodeport services")
						}
						log.V(1).Info(fmt.Sprintf("Deleted nodePort service '%s' for external listener '%s'", removeService.GetName(), eListener.Name))
					}
				}

			}

		}
	}

	log.V(1).Info("Reconciled")

	return nil
}
