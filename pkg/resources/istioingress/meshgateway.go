// Copyright © 2020 Cisco Systems, Inc. and/or its affiliates
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

package istioingress

import (
	"fmt"
	"math"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/banzaicloud/koperator/api/v1beta1"
	"github.com/banzaicloud/koperator/pkg/resources/templates"
	"github.com/banzaicloud/koperator/pkg/util"
	istioingressutils "github.com/banzaicloud/koperator/pkg/util/istioingress"
	kafkautils "github.com/banzaicloud/koperator/pkg/util/kafka"
)

func (r *Reconciler) meshgateway(log logr.Logger, externalListenerConfig v1beta1.ExternalListenerConfig,
	ingressConfig v1beta1.IngressConfig, ingressConfigName, defaultIngressConfigName, istioRevision string) runtime.Object {
	eListenerLabelName := util.ConstructEListenerLabelName(ingressConfigName, externalListenerConfig.Name)

	var meshgatewayName string
	if ingressConfigName == util.IngressConfigGlobalName {
		meshgatewayName = fmt.Sprintf(istioingressutils.MeshGatewayNameTemplate, externalListenerConfig.Name, r.KafkaCluster.GetName())
	} else {
		meshgatewayName = fmt.Sprintf(istioingressutils.MeshGatewayNameTemplateWithScope,
			externalListenerConfig.Name, ingressConfigName, r.KafkaCluster.GetName())
	}

	// Create a standard Kubernetes Deployment instead of IstioMeshGateway
	deployment := &appsv1.Deployment{
		ObjectMeta: templates.ObjectMeta(
			meshgatewayName,
			labelsForIstioIngress(r.KafkaCluster.Name, eListenerLabelName, istioRevision), r.KafkaCluster),
		Spec: appsv1.DeploymentSpec{
			Replicas: util.Int32Pointer(ingressConfig.IstioIngressConfig.GetReplicas()),
			Selector: &metav1.LabelSelector{
				MatchLabels: labelsForIstioIngress(r.KafkaCluster.Name, eListenerLabelName, istioRevision),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      labelsForIstioIngress(r.KafkaCluster.Name, eListenerLabelName, istioRevision),
					Annotations: ingressConfig.IstioIngressConfig.GetAnnotations(),
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:    "istio-proxy",
							Image:   v1beta1.DefaultIstioProxyImage, // Use a standard Istio proxy image
							Command: []string{"/usr/local/bin/pilot-agent"},
							Args: []string{
								"proxy",
								"router",
								"--domain", fmt.Sprintf("%s.svc.cluster.local", r.KafkaCluster.Namespace),
								"--proxyLogLevel=warning",
								"--proxyComponentLogLevel=misc:error",
								"--log_output_level=default:info",
							},
							Env:       append(convertEnvVars(ingressConfig.IstioIngressConfig.Envs), getIstioProxyEnvVars(meshgatewayName, r.KafkaCluster.Namespace)...),
							Resources: *ingressConfig.IstioIngressConfig.GetResources(),
							SecurityContext: &corev1.SecurityContext{
								RunAsNonRoot: util.BoolPointer(false),
							},
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 15090,
									Protocol:      corev1.ProtocolTCP,
									Name:          "http-envoy-prom",
								},
								{
									ContainerPort: 15021,
									Protocol:      corev1.ProtocolTCP,
									Name:          "status-port",
								},
							},
						},
					},
					NodeSelector: ingressConfig.IstioIngressConfig.NodeSelector,
					Tolerations:  convertTolerations(ingressConfig.IstioIngressConfig.Tolerations),
				},
			},
		},
	}

	return deployment
}

// meshgatewayService creates a Service for the mesh gateway deployment
func (r *Reconciler) meshgatewayService(log logr.Logger, externalListenerConfig v1beta1.ExternalListenerConfig,
	ingressConfig v1beta1.IngressConfig, ingressConfigName, defaultIngressConfigName, istioRevision string) runtime.Object {
	eListenerLabelName := util.ConstructEListenerLabelName(ingressConfigName, externalListenerConfig.Name)

	var meshgatewayName string
	if ingressConfigName == util.IngressConfigGlobalName {
		meshgatewayName = fmt.Sprintf(istioingressutils.MeshGatewayNameTemplate, externalListenerConfig.Name, r.KafkaCluster.GetName())
	} else {
		meshgatewayName = fmt.Sprintf(istioingressutils.MeshGatewayNameTemplateWithScope,
			externalListenerConfig.Name, ingressConfigName, r.KafkaCluster.GetName())
	}

	service := &corev1.Service{
		ObjectMeta: templates.ObjectMeta(
			meshgatewayName,
			labelsForIstioIngress(r.KafkaCluster.Name, eListenerLabelName, istioRevision), r.KafkaCluster),
		Spec: corev1.ServiceSpec{
			Type:                     ingressConfig.GetServiceType(),
			LoadBalancerSourceRanges: ingressConfig.IstioIngressConfig.GetLoadBalancerSourceRanges(),
			Ports: generateExternalPorts(r.KafkaCluster,
				util.GetBrokerIdsFromStatusAndSpec(r.KafkaCluster.Status.BrokersState, r.KafkaCluster.Spec.Brokers, log),
				externalListenerConfig, log, ingressConfigName, defaultIngressConfigName),
			Selector: labelsForIstioIngress(r.KafkaCluster.Name, eListenerLabelName, istioRevision),
		},
	}

	// Add service annotations
	if ingressConfig.GetServiceAnnotations() != nil {
		service.Annotations = ingressConfig.GetServiceAnnotations()
	}

	return service
}

func generateExternalPorts(kc *v1beta1.KafkaCluster, brokerIds []int,
	externalListenerConfig v1beta1.ExternalListenerConfig, log logr.Logger, ingressConfigName, defaultIngressConfigName string) []corev1.ServicePort {
	generatedPorts := make([]corev1.ServicePort, 0)
	for _, brokerId := range brokerIds {
		brokerConfig, err := kafkautils.GatherBrokerConfigIfAvailable(kc.Spec, brokerId)
		if err != nil {
			log.Error(err, "could not determine brokerConfig")
			continue
		}
		if util.ShouldIncludeBroker(brokerConfig, kc.Status, brokerId, defaultIngressConfigName, ingressConfigName) {
			generatedPorts = append(generatedPorts, corev1.ServicePort{
				Name:     fmt.Sprintf("tcp-broker-%d", brokerId),
				Protocol: corev1.ProtocolTCP,
				Port: func() int32 {
					// Broker IDs are always within valid range for int32 conversion
					if brokerId < 0 || brokerId > math.MaxInt32 {
						// This should never happen as broker IDs are small positive integers
						log.Error(fmt.Errorf("broker ID %d out of valid range for int32 conversion", brokerId), "Invalid broker ID detected in mesh gateway port")
						return 0
					}
					return externalListenerConfig.GetBrokerPort(int32(brokerId))
				}(),
				TargetPort: func() intstr.IntOrString {
					// Broker IDs are always within valid range for int32 conversion
					if brokerId < 0 || brokerId > math.MaxInt32 {
						// This should never happen as broker IDs are small positive integers
						log.Error(fmt.Errorf("broker ID %d out of valid range for int32 conversion", brokerId), "Invalid broker ID detected in mesh gateway target port")
						return intstr.FromInt(0)
					}
					brokerPort := externalListenerConfig.GetBrokerPort(int32(brokerId))
					// Port numbers are always within valid range for int conversion
					if brokerPort < 0 || brokerPort > 65535 {
						// This should never happen as GetBrokerPort returns valid port numbers
						log.Error(fmt.Errorf("broker port %d out of valid range [0-65535] for broker %d", brokerPort, brokerId), "Invalid broker port detected in mesh gateway target port")
						return intstr.FromInt(0)
					}
					return intstr.FromInt(int(brokerPort))
				}(),
			})
		}
	}

	generatedPorts = append(generatedPorts, corev1.ServicePort{
		Name:       fmt.Sprintf(kafkautils.AllBrokerServiceTemplate, "tcp"),
		Protocol:   corev1.ProtocolTCP,
		Port:       externalListenerConfig.GetAnyCastPort(),
		TargetPort: intstr.FromInt(int(externalListenerConfig.GetIngressControllerTargetPort())),
	})

	return generatedPorts
}

// convertEnvVars converts []*corev1.EnvVar to []corev1.EnvVar
func convertEnvVars(envVars []*corev1.EnvVar) []corev1.EnvVar {
	result := make([]corev1.EnvVar, 0, len(envVars))
	for _, envVar := range envVars {
		if envVar != nil {
			result = append(result, *envVar)
		}
	}
	return result
}

// convertTolerations converts []*corev1.Toleration to []corev1.Toleration
func convertTolerations(tolerations []*corev1.Toleration) []corev1.Toleration {
	result := make([]corev1.Toleration, 0, len(tolerations))
	for _, toleration := range tolerations {
		if toleration != nil {
			result = append(result, *toleration)
		}
	}
	return result
}

// getIstioProxyEnvVars returns the required environment variables for Istio proxy
func getIstioProxyEnvVars(gatewayName, namespace string) []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name:  "PILOT_CERT_PROVIDER",
			Value: "istiod",
		},
		{
			Name:  "CA_ADDR",
			Value: "istiod.istio-system.svc:15012",
		},
		{
			Name: "POD_NAME",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					APIVersion: "v1",
					FieldPath:  "metadata.name",
				},
			},
		},
		{
			Name: "POD_NAMESPACE",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					APIVersion: "v1",
					FieldPath:  "metadata.namespace",
				},
			},
		},
		{
			Name: "INSTANCE_IP",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					APIVersion: "v1",
					FieldPath:  "status.podIP",
				},
			},
		},
		{
			Name: "SERVICE_ACCOUNT",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					APIVersion: "v1",
					FieldPath:  "spec.serviceAccountName",
				},
			},
		},
		{
			Name: "HOST_IP",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					APIVersion: "v1",
					FieldPath:  "status.hostIP",
				},
			},
		},
		{
			Name:  "PROXY_CONFIG",
			Value: "{}",
		},
		{
			Name:  "ISTIO_META_POD_PORTS",
			Value: `[{"containerPort":15090,"protocol":"TCP","name":"http-envoy-prom"},{"containerPort":15021,"protocol":"TCP","name":"status-port"}]`,
		},
		{
			Name:  "ISTIO_META_APP_CONTAINERS",
			Value: "istio-proxy",
		},
		{
			Name:  "ISTIO_META_CLUSTER_ID",
			Value: "Kubernetes",
		},
		{
			Name:  "ISTIO_META_INTERCEPTION_MODE",
			Value: "REDIRECT",
		},
		{
			Name:  "ISTIO_META_WORKLOAD_NAME",
			Value: gatewayName,
		},
		{
			Name:  "ISTIO_META_OWNER",
			Value: fmt.Sprintf("kubernetes://apis/apps/v1/namespaces/%s/deployments/%s", namespace, gatewayName),
		},
		{
			Name:  "ISTIO_META_MESH_ID",
			Value: "cluster.local",
		},
		{
			Name:  "TRUST_DOMAIN",
			Value: "cluster.local",
		},
	}
}
