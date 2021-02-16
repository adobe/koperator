// Copyright © 2019 Banzai Cloud
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

package envoy

import (
	"fmt"

	"github.com/ghodss/yaml"
	"github.com/go-logr/logr"
	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/ptypes/duration"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/banzaicloud/kafka-operator/api/v1beta1"
	"github.com/banzaicloud/kafka-operator/pkg/resources/templates"
	"github.com/banzaicloud/kafka-operator/pkg/util"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoylistener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	envoybootstrap "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v2"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	ptypesstruct "github.com/golang/protobuf/ptypes/struct"
	corev1 "k8s.io/api/core/v1"
)

func (r *Reconciler) configMap(log logr.Logger, envoyConfig *v1beta1.EnvoyConfig) runtime.Object {
	configMap := &corev1.ConfigMap{
		ObjectMeta: templates.ObjectMeta(configName(envoyConfig), labelSelector(envoyConfig), r.KafkaCluster),
		Data:       map[string]string{"envoy.yaml": GenerateEnvoyConfig(r.KafkaCluster, envoyConfig, log)},
	}
	return configMap
}

func GenerateEnvoyConfig(kc *v1beta1.KafkaCluster, envoyConfig *v1beta1.EnvoyConfig, log logr.Logger) string {
	//TODO support multiple external listener by removing [0] (baluchicken)
	adminConfig := envoybootstrap.Admin{
		AccessLogPath: "/tmp/admin_access.log",
		Address: &envoycore.Address{
			Address: &envoycore.Address_SocketAddress{
				SocketAddress: &envoycore.SocketAddress{
					Address: "0.0.0.0",
					PortSpecifier: &envoycore.SocketAddress_PortValue{
						PortValue: 9901,
					},
				},
			},
		},
	}

	var listeners []*envoyapi.Listener
	var clusters []*envoyapi.Cluster

	for _, brokerId := range util.GetBrokerIdsFromStatus(kc.Status.BrokersState, log) {
		if envoyConfig.EnvoyPerBrokerGroup {
			// Since `EnvoyPerBrokerGroup` is enabled, we add only brokers having the same group as the envoy.
			// If we cannot retrieve a valid brokerConfigGroup from the Status, we will add the broker to all envoys
			// (this is a safe net to ensure that the brokers are always reachable through envoy)
			brokerConfigGroup := util.GetBrokerConfigGroupFromStatus(kc.Status.BrokersState, brokerId, log)
			if brokerConfigGroup != "" && brokerConfigGroup != envoyConfig.Id {
				continue
			}
		}
		if kc.Spec.ListenersConfig.ExternalListeners != nil {
			listeners = append(listeners, envoyListener(fmt.Sprintf("broker-%d-external", brokerId),
				uint32(kc.Spec.ListenersConfig.ExternalListeners[0].ExternalStartingPort+int32(brokerId))))
		}

		for _, internalListener := range kc.Spec.ListenersConfig.InternalListeners {
			if internalListener.IngressForwarded && internalListener.InternalStartingPort > 0 {
				if internalListener.UsedForInnerBrokerCommunication {
					listeners = append(listeners, envoyListener(fmt.Sprintf("broker-%d-internal", brokerId),
						uint32(internalListener.InternalStartingPort+int32(brokerId))))
				} else if internalListener.UsedForControllerCommunication {
					listeners = append(listeners, envoyListener(fmt.Sprintf("broker-%d-controller", brokerId),
						uint32(internalListener.InternalStartingPort+int32(brokerId))))
				}
			}
		}

		if kc.Spec.ListenersConfig.ExternalListeners != nil {
			clusters = append(clusters, envoyCluster(fmt.Sprintf("broker-%d-external", brokerId),
				serviceName(kc, uint32(brokerId)), uint32(kc.Spec.ListenersConfig.ExternalListeners[0].ContainerPort)))
		}

		for _, internalListener := range kc.Spec.ListenersConfig.InternalListeners {
			if internalListener.IngressForwarded && internalListener.InternalStartingPort > 0 {
				if internalListener.UsedForInnerBrokerCommunication {
					clusters = append(clusters, envoyCluster(fmt.Sprintf("broker-%d-controller", brokerId),
						serviceName(kc, uint32(brokerId)), uint32(internalListener.ContainerPort)))
				} else {
					clusters = append(clusters, envoyCluster(fmt.Sprintf("broker-%d-controller", brokerId),
						serviceName(kc, uint32(brokerId)), uint32(internalListener.ContainerPort)))
				}
			}
		}
	}

	if kc.Spec.ListenersConfig.ExternalListeners != nil {
		for _, externalListener := range kc.Spec.ListenersConfig.ExternalListeners {
			if externalListener.DiscoveryPort > 0 {
				name := fmt.Sprintf("%s-%s", "kafka-headless", externalListener.Name)
				listeners = append(listeners, envoyListener(name, uint32(externalListener.DiscoveryPort)))
				for _, internalListener := range kc.Spec.ListenersConfig.InternalListeners {
					if internalListener.UsedForInnerBrokerCommunication {
						clusters = append(clusters, envoyCluster(name,
							fmt.Sprintf("%s-headless.%s.svc.%s", kc.Name, kc.Namespace, kc.Spec.GetKubernetesClusterDomain()),
							uint32(internalListener.ContainerPort)))
						break // Stop at the first internal listener (no support for multiple internal listeners)
					}
				}
			}
		}
	}

	config := envoybootstrap.Bootstrap_StaticResources{
		Listeners: listeners,
		Clusters:  clusters,
	}
	generatedConfig := envoybootstrap.Bootstrap{
		Admin:           &adminConfig,
		StaticResources: &config,
	}
	marshaller := &jsonpb.Marshaler{}
	marshalledProtobufConfig, err := marshaller.MarshalToString(&generatedConfig)
	if err != nil {
		log.Error(err, "could not marshall envoy config")
		return ""
	}

	marshalledConfig, err := yaml.JSONToYAML([]byte(marshalledProtobufConfig))
	if err != nil {
		log.Error(err, "could not convert config from Json to Yaml")
		return ""
	}
	return string(marshalledConfig)
}

func envoyListener(name string, containerPort uint32) *envoyapi.Listener {
	return &envoyapi.Listener{
		Address: &envoycore.Address{
			Address: &envoycore.Address_SocketAddress{
				SocketAddress: &envoycore.SocketAddress{
					Address: "0.0.0.0",
					PortSpecifier: &envoycore.SocketAddress_PortValue{
						PortValue: containerPort,
					},
				},
			},
		},
		FilterChains: []*envoylistener.FilterChain{
			{
				Filters: []*envoylistener.Filter{
					{
						Name: wellknown.TCPProxy,
						ConfigType: &envoylistener.Filter_Config{
							Config: &ptypesstruct.Struct{
								Fields: map[string]*ptypesstruct.Value{
									"stat_prefix": {Kind: &ptypesstruct.Value_StringValue{StringValue: fmt.Sprintf("tcp_%s", name)}},
									"cluster":     {Kind: &ptypesstruct.Value_StringValue{StringValue: name}},
								},
							},
						},
					},
				},
			},
		},
	}
}

func serviceName(kc *v1beta1.KafkaCluster, brokerId uint32) string {
	return fmt.Sprintf("%s-%d.%s-headless.%s.svc.%s", kc.Name, brokerId, kc.Name, kc.Namespace, kc.Spec.GetKubernetesClusterDomain())
}

func envoyCluster(name, address string, containerPort uint32) *envoyapi.Cluster {
	return &envoyapi.Cluster{
		Name:                 name,
		ConnectTimeout:       &duration.Duration{Seconds: 1},
		ClusterDiscoveryType: &envoyapi.Cluster_Type{Type: envoyapi.Cluster_STRICT_DNS},
		LbPolicy:             envoyapi.Cluster_ROUND_ROBIN,
		Http2ProtocolOptions: &envoycore.Http2ProtocolOptions{},
		Hosts: []*envoycore.Address{
			{
				Address: &envoycore.Address_SocketAddress{
					SocketAddress: &envoycore.SocketAddress{
						Address: address,
						PortSpecifier: &envoycore.SocketAddress_PortValue{
							PortValue: containerPort,
						},
					},
				},
			},
		},
	}
}

func configName(envoyConfig *v1beta1.EnvoyConfig) string {
	if envoyConfig.Id == envoyGlobal {
		return envoyVolumeAndConfigName
	} else {
		return fmt.Sprintf("%s-%s", envoyVolumeAndConfigName, envoyConfig.Id)
	}
}
