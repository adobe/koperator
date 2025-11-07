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

	apiutil "github.com/banzaicloud/koperator/api/util"
	"github.com/banzaicloud/koperator/api/v1beta1"
	"github.com/banzaicloud/koperator/pkg/resources/templates"
	envoygatewayutils "github.com/banzaicloud/koperator/pkg/util/envoygateway"
)

func (r *Reconciler) gateway(eListener v1beta1.ExternalListenerConfig,
	ingressConfig v1beta1.IngressConfig) client.Object {
	gatewayName := fmt.Sprintf(envoygatewayutils.GatewayNameTemplate, eListener.Name)
	if ingressConfig.EnvoyGatewayConfig != nil && ingressConfig.EnvoyGatewayConfig.GatewayName != "" {
		gatewayName = ingressConfig.EnvoyGatewayConfig.GatewayName
	}

	gatewayClassName := "eg"
	if ingressConfig.EnvoyGatewayConfig != nil {
		gatewayClassName = ingressConfig.EnvoyGatewayConfig.GetGatewayClassName()
	}

	labels := labelsForEnvoyGateway(r.KafkaCluster.Name, eListener.Name)
	if r.KafkaCluster.Spec.PropagateLabels {
		labels = apiutil.MergeLabels(r.KafkaCluster.Labels, labels)
	}

	annotations := make(map[string]string)
	if ingressConfig.EnvoyGatewayConfig != nil {
		annotations = ingressConfig.EnvoyGatewayConfig.GetAnnotations()
	}

	// Build listeners for the Gateway
	var listeners []gatewayv1.Listener

	// Add listener for each broker
	for _, broker := range r.KafkaCluster.Spec.Brokers {
		listenerName := gatewayv1.SectionName(fmt.Sprintf("broker-%d", broker.Id))
		port := eListener.GetBrokerPort(broker.Id)

		listener := gatewayv1.Listener{
			Name:     listenerName,
			Port:     port,
			Protocol: gatewayv1.TCPProtocolType,
		}

		if eListener.TLSEnabled() {
			listener.Protocol = gatewayv1.TLSProtocolType

			// When TLS is enabled, use hostname-based routing (SNI)
			// Each broker needs a unique hostname to satisfy Gateway API uniqueness constraint
			if ingressConfig.EnvoyGatewayConfig != nil && ingressConfig.EnvoyGatewayConfig.BrokerHostnameTemplate != "" {
				hostname := gatewayv1.Hostname(envoygatewayutils.GetBrokerHostname(ingressConfig.EnvoyGatewayConfig.BrokerHostnameTemplate, broker.Id))
				listener.Hostname = &hostname
			}

			// EnvoyGateway only supports TLS termination at the gateway level
			// TLSSecretName is validated to be present in the Reconcile method
			listener.TLS = &gatewayv1.ListenerTLSConfig{
				Mode: func() *gatewayv1.TLSModeType {
					mode := gatewayv1.TLSModeTerminate
					return &mode
				}(),
				CertificateRefs: []gatewayv1.SecretObjectReference{
					{
						Name: gatewayv1.ObjectName(ingressConfig.EnvoyGatewayConfig.TLSSecretName),
					},
				},
			}
		}

		listeners = append(listeners, listener)
	}

	// Add anycast listener (all-broker)
	anycastListenerName := gatewayv1.SectionName("anycast")
	anycastPort := eListener.GetAnyCastPort()

	anycastListener := gatewayv1.Listener{
		Name:     anycastListenerName,
		Port:     anycastPort,
		Protocol: gatewayv1.TCPProtocolType,
	}

	if eListener.TLSEnabled() {
		anycastListener.Protocol = gatewayv1.TLSProtocolType

		// EnvoyGateway only supports TLS termination at the gateway level
		// TLSSecretName is validated to be present in the Reconcile method
		anycastListener.TLS = &gatewayv1.ListenerTLSConfig{
			Mode: func() *gatewayv1.TLSModeType {
				mode := gatewayv1.TLSModeTerminate
				return &mode
			}(),
			CertificateRefs: []gatewayv1.SecretObjectReference{
				{
					Name: gatewayv1.ObjectName(ingressConfig.EnvoyGatewayConfig.TLSSecretName),
				},
			},
		}
	}

	listeners = append(listeners, anycastListener)

	gateway := &gatewayv1.Gateway{
		ObjectMeta: templates.ObjectMetaWithAnnotations(gatewayName, labels, annotations, r.KafkaCluster),
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: gatewayv1.ObjectName(gatewayClassName),
			Listeners:        listeners,
		},
	}

	return gateway
}
