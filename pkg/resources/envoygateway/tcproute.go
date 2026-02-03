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
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	apiutil "github.com/banzaicloud/koperator/api/util"
	"github.com/banzaicloud/koperator/api/v1beta1"
	"github.com/banzaicloud/koperator/pkg/resources/templates"
	envoygatewayutils "github.com/banzaicloud/koperator/pkg/util/envoygateway"
	"github.com/banzaicloud/koperator/pkg/util/kafka"
)

func (r *Reconciler) tcpRoute(brokerId int32, eListener v1beta1.ExternalListenerConfig,
	ingressConfig v1beta1.IngressConfig) client.Object {
	tcpRouteName := fmt.Sprintf(envoygatewayutils.TCPRouteNameTemplate, eListener.Name, fmt.Sprintf("%d", brokerId))

	gatewayName := fmt.Sprintf(envoygatewayutils.GatewayNameTemplate, eListener.Name)
	if ingressConfig.EnvoyGatewayConfig != nil && ingressConfig.EnvoyGatewayConfig.GatewayName != "" {
		gatewayName = ingressConfig.EnvoyGatewayConfig.GatewayName
	}

	labels := labelsForEnvoyGateway(r.KafkaCluster.Name, eListener.Name)
	if r.KafkaCluster.Spec.PropagateLabels {
		labels = apiutil.MergeLabels(r.KafkaCluster.Labels, labels)
	}

	// Backend service reference
	serviceName := fmt.Sprintf(kafka.AllBrokerServiceTemplate, r.KafkaCluster.Name)
	servicePort := eListener.ContainerPort

	tcpRoute := &gatewayv1alpha2.TCPRoute{
		ObjectMeta: templates.ObjectMeta(tcpRouteName, labels, r.KafkaCluster),
		Spec: gatewayv1alpha2.TCPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: []gatewayv1.ParentReference{
					{
						Name: gatewayv1.ObjectName(gatewayName),
						SectionName: func() *gatewayv1.SectionName {
							name := gatewayv1.SectionName(fmt.Sprintf("broker-%d", brokerId))
							return &name
						}(),
					},
				},
			},
			Rules: []gatewayv1alpha2.TCPRouteRule{
				{
					BackendRefs: []gatewayv1.BackendRef{
						{
							BackendObjectReference: gatewayv1.BackendObjectReference{
								Name: gatewayv1.ObjectName(serviceName),
								Port: func() *gatewayv1.PortNumber {
									port := servicePort
									return &port
								}(),
							},
						},
					},
				},
			},
		},
	}

	return tcpRoute
}

func (r *Reconciler) tcpRouteAllBroker(eListener v1beta1.ExternalListenerConfig,
	ingressConfig v1beta1.IngressConfig) client.Object {
	tcpRouteName := fmt.Sprintf(envoygatewayutils.TCPRouteNameTemplate, eListener.Name, "anycast")

	gatewayName := fmt.Sprintf(envoygatewayutils.GatewayNameTemplate, eListener.Name)
	if ingressConfig.EnvoyGatewayConfig != nil && ingressConfig.EnvoyGatewayConfig.GatewayName != "" {
		gatewayName = ingressConfig.EnvoyGatewayConfig.GatewayName
	}

	labels := labelsForEnvoyGateway(r.KafkaCluster.Name, eListener.Name)
	if r.KafkaCluster.Spec.PropagateLabels {
		labels = apiutil.MergeLabels(r.KafkaCluster.Labels, labels)
	}

	// Backend service reference
	serviceName := fmt.Sprintf(kafka.AllBrokerServiceTemplate, r.KafkaCluster.Name)
	servicePort := eListener.ContainerPort

	tcpRoute := &gatewayv1alpha2.TCPRoute{
		ObjectMeta: templates.ObjectMeta(tcpRouteName, labels, r.KafkaCluster),
		Spec: gatewayv1alpha2.TCPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: []gatewayv1.ParentReference{
					{
						Name: gatewayv1.ObjectName(gatewayName),
						SectionName: func() *gatewayv1.SectionName {
							name := gatewayv1.SectionName("anycast")
							return &name
						}(),
					},
				},
			},
			Rules: []gatewayv1alpha2.TCPRouteRule{
				{
					BackendRefs: []gatewayv1.BackendRef{
						{
							BackendObjectReference: gatewayv1.BackendObjectReference{
								Name: gatewayv1.ObjectName(serviceName),
								Port: func() *gatewayv1.PortNumber {
									port := servicePort
									return &port
								}(),
							},
						},
					},
				},
			},
		},
	}

	return tcpRoute
}
