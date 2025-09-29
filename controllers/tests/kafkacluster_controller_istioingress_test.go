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

package tests

import (
	"context"
	"fmt"
	"sync/atomic"

	istioclientv1beta1 "github.com/banzaicloud/istio-client-go/pkg/networking/v1beta1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/banzaicloud/koperator/api/v1beta1"
	"github.com/banzaicloud/koperator/pkg/util"
	"github.com/banzaicloud/koperator/pkg/util/istioingress"
)

var _ = Describe("KafkaClusterIstioIngressController", func() {
	var (
		count              uint64 = 0
		namespace          string
		namespaceObj       *corev1.Namespace
		kafkaClusterCRName string
		kafkaCluster       *v1beta1.KafkaCluster
	)

	ExpectIstioIngressLabels := func(labels map[string]string, eListenerName, crName string) {
		Expect(labels).To(HaveKeyWithValue(v1beta1.AppLabelKey, "istioingress"))
		Expect(labels).To(HaveKeyWithValue("eListenerName", eListenerName))
		Expect(labels).To(HaveKeyWithValue(v1beta1.KafkaCRLabelKey, crName))
	}

	BeforeEach(func() {
		atomic.AddUint64(&count, 1)

		namespace = fmt.Sprintf("kafka-istioingress-%v", count)
		namespaceObj = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}

		kafkaClusterCRName = fmt.Sprintf("kafkacluster-%v", count)
		kafkaCluster = createMinimalKafkaClusterCR(kafkaClusterCRName, namespace)

		kafkaCluster.Spec.IngressController = istioingress.IngressControllerName
		kafkaCluster.Spec.IstioControlPlane = &v1beta1.IstioControlPlaneReference{Name: "istiod", Namespace: "istio-system"}
		kafkaCluster.Spec.ListenersConfig.ExternalListeners = []v1beta1.ExternalListenerConfig{
			{
				CommonListenerSpec: v1beta1.CommonListenerSpec{
					Type:          "plaintext",
					Name:          "external",
					ContainerPort: 9094,
				},
				ExternalStartingPort: 19090,
			},
		}
	})

	JustBeforeEach(func(ctx SpecContext) {
		By("creating namespace " + namespace)
		err := k8sClient.Create(ctx, namespaceObj)
		Expect(err).NotTo(HaveOccurred())

		By("creating Kafka cluster object " + kafkaCluster.Name + " in namespace " + namespace)
		err = k8sClient.Create(ctx, kafkaCluster)
		Expect(err).NotTo(HaveOccurred())

		svcName := fmt.Sprintf("meshgateway-external-%s", kafkaCluster.Name)
		svcFromMeshGateway := corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      svcName,
				Namespace: namespace,
			},
			Spec: corev1.ServiceSpec{
				Type: corev1.ServiceTypeLoadBalancer,
				Ports: []corev1.ServicePort{
					// other ports omitted
					{
						Name:     "tcp-all-broker",
						Port:     29092, // from MeshGateway (guarded by the tests)
						Protocol: corev1.ProtocolTCP,
					},
				},
			},
		}
		err = k8sClient.Create(ctx, &svcFromMeshGateway)
		Expect(err).NotTo(HaveOccurred())
		svcFromMeshGateway.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{{Hostname: "ingress.test.host.com"}}
		err = k8sClient.Status().Update(ctx, &svcFromMeshGateway)
		Expect(err).NotTo(HaveOccurred())

		waitForClusterRunningState(ctx, kafkaCluster, namespace)
	})

	JustAfterEach(func(ctx SpecContext) {
		By("deleting Kafka cluster object " + kafkaCluster.Name + " in namespace " + namespace)
		err := k8sClient.Delete(ctx, kafkaCluster)
		Expect(err).NotTo(HaveOccurred())
		kafkaCluster = nil
	})

	When("Istio ingress controller is configured", func() {
		BeforeEach(func() {
			kafkaCluster.Spec.IngressController = istioingress.IngressControllerName
		})

		It("creates Istio ingress related objects", func(ctx SpecContext) {
			// Test that the mesh gateway deployment is created
			var deployment appsv1.Deployment
			meshGatewayName := fmt.Sprintf("meshgateway-external-%s", kafkaCluster.Name)
			Eventually(ctx, func() error {
				err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: namespace, Name: meshGatewayName}, &deployment)
				return err
			}).Should(Succeed())

			Expect(deployment.Spec.Replicas).To(Equal(util.Int32Pointer(1)))
			Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(1))
			Expect(deployment.Spec.Template.Spec.Containers[0].Name).To(Equal("istio-proxy"))

			// Test that the mesh gateway service is created
			var service corev1.Service
			Eventually(ctx, func() error {
				err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: namespace, Name: meshGatewayName}, &service)
				return err
			}).Should(Succeed())

			Expect(service.Spec.Type).To(Equal(corev1.ServiceTypeLoadBalancer))
			Expect(service.Spec.Ports).To(HaveLen(4))

			// For LoadBalancer services, NodePort is automatically assigned by Kubernetes
			// So we check individual fields instead of comparing the entire struct
			Expect(service.Spec.Ports[0].Name).To(Equal("tcp-broker-0"))
			Expect(service.Spec.Ports[0].Protocol).To(Equal(corev1.ProtocolTCP))
			Expect(service.Spec.Ports[0].Port).To(Equal(int32(19090)))
			Expect(service.Spec.Ports[0].TargetPort).To(Equal(intstr.FromInt(19090)))

			Expect(service.Spec.Ports[1].Name).To(Equal("tcp-broker-1"))
			Expect(service.Spec.Ports[1].Protocol).To(Equal(corev1.ProtocolTCP))
			Expect(service.Spec.Ports[1].Port).To(Equal(int32(19091)))
			Expect(service.Spec.Ports[1].TargetPort).To(Equal(intstr.FromInt(19091)))

			Expect(service.Spec.Ports[2].Name).To(Equal("tcp-broker-2"))
			Expect(service.Spec.Ports[2].Protocol).To(Equal(corev1.ProtocolTCP))
			Expect(service.Spec.Ports[2].Port).To(Equal(int32(19092)))
			Expect(service.Spec.Ports[2].TargetPort).To(Equal(intstr.FromInt(19092)))

			Expect(service.Spec.Ports[3].Name).To(Equal("tcp-all-broker"))
			Expect(service.Spec.Ports[3].Protocol).To(Equal(corev1.ProtocolTCP))
			Expect(service.Spec.Ports[3].Port).To(Equal(int32(29092)))
			Expect(service.Spec.Ports[3].TargetPort).To(Equal(intstr.FromInt(29092)))

			var gateway istioclientv1beta1.Gateway
			gatewayName := fmt.Sprintf("%s-external-gateway", kafkaCluster.Name)
			Eventually(ctx, func() error {
				err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: gatewayName}, &gateway)
				return err
			}).Should(Succeed())

			ExpectIstioIngressLabels(gateway.Labels, "external", kafkaClusterCRName)
			ExpectIstioIngressLabels(gateway.Spec.Selector, "external", kafkaClusterCRName)
			Expect(gateway.Spec.Servers).To(ConsistOf(
				istioclientv1beta1.Server{
					Port: &istioclientv1beta1.Port{
						Number:   19090,
						Protocol: "TCP",
						Name:     "tcp-broker-0"},
					Hosts: []string{"*"},
				},
				istioclientv1beta1.Server{
					Port: &istioclientv1beta1.Port{
						Number:   19091,
						Protocol: "TCP",
						Name:     "tcp-broker-1"},
					Hosts: []string{"*"},
				},
				istioclientv1beta1.Server{
					Port: &istioclientv1beta1.Port{
						Number:   19092,
						Protocol: "TCP",
						Name:     "tcp-broker-2"},
					Hosts: []string{"*"},
				},
				istioclientv1beta1.Server{
					Port: &istioclientv1beta1.Port{
						Number:   29092,
						Protocol: "TCP",
						Name:     "tcp-all-broker",
					},
					Hosts: []string{"*"},
				}))

			var virtualService istioclientv1beta1.VirtualService
			virtualServiceName := fmt.Sprintf("%s-external-virtualservice", kafkaCluster.Name)
			Eventually(ctx, func() error {
				err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: virtualServiceName}, &virtualService)
				return err
			}).Should(Succeed())

			ExpectIstioIngressLabels(virtualService.Labels, "external", kafkaClusterCRName)
			Expect(virtualService.Spec).To(Equal(istioclientv1beta1.VirtualServiceSpec{
				Hosts:    []string{"*"},
				Gateways: []string{fmt.Sprintf("%s-external-gateway", kafkaClusterCRName)},
				TCP: []istioclientv1beta1.TCPRoute{
					{
						Match: []istioclientv1beta1.L4MatchAttributes{{Port: util.IntPointer(19090)}},
						Route: []*istioclientv1beta1.RouteDestination{{
							Destination: &istioclientv1beta1.Destination{
								Host: "kafkacluster-1-0",
								Port: &istioclientv1beta1.PortSelector{Number: 9094},
							},
						}},
					},
					{
						Match: []istioclientv1beta1.L4MatchAttributes{{Port: util.IntPointer(19091)}},
						Route: []*istioclientv1beta1.RouteDestination{{
							Destination: &istioclientv1beta1.Destination{
								Host: "kafkacluster-1-1",
								Port: &istioclientv1beta1.PortSelector{Number: 9094},
							},
						}},
					},
					{
						Match: []istioclientv1beta1.L4MatchAttributes{{Port: util.IntPointer(19092)}},
						Route: []*istioclientv1beta1.RouteDestination{{
							Destination: &istioclientv1beta1.Destination{
								Host: "kafkacluster-1-2",
								Port: &istioclientv1beta1.PortSelector{Number: 9094},
							},
						}},
					},
					{
						Match: []istioclientv1beta1.L4MatchAttributes{{Port: util.IntPointer(29092)}},
						Route: []*istioclientv1beta1.RouteDestination{{
							Destination: &istioclientv1beta1.Destination{
								Host: "kafkacluster-1-all-broker",
								Port: &istioclientv1beta1.PortSelector{Number: 9094},
							},
						}},
					},
				},
			}))

			// expect kafkaCluster listener status
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      kafkaCluster.Name,
				Namespace: kafkaCluster.Namespace,
			}, kafkaCluster)
			Expect(err).NotTo(HaveOccurred())

			Expect(kafkaCluster.Status.ListenerStatuses).To(Equal(v1beta1.ListenerStatuses{
				InternalListeners: map[string]v1beta1.ListenerStatusList{
					"internal": {
						{
							Name:    "any-broker",
							Address: fmt.Sprintf("%s-all-broker.kafka-istioingress-%d.svc.cluster.local:29092", kafkaCluster.Name, count),
						},
						{
							Name:    "broker-0",
							Address: fmt.Sprintf("%s-0.kafka-istioingress-%d.svc.cluster.local:29092", kafkaCluster.Name, count),
						},
						{
							Name:    "broker-1",
							Address: fmt.Sprintf("%s-1.kafka-istioingress-%d.svc.cluster.local:29092", kafkaCluster.Name, count),
						},
						{
							Name:    "broker-2",
							Address: fmt.Sprintf("%s-2.kafka-istioingress-%d.svc.cluster.local:29092", kafkaCluster.Name, count),
						},
					},
				},
				ExternalListeners: map[string]v1beta1.ListenerStatusList{
					"external": {
						{
							Name:    "any-broker",
							Address: "ingress.test.host.com:29092",
						},
						{
							Name:    "broker-0",
							Address: "ingress.test.host.com:19090",
						},
						{
							Name:    "broker-1",
							Address: "ingress.test.host.com:19091",
						},
						{
							Name:    "broker-2",
							Address: "ingress.test.host.com:19092",
						},
					},
				},
			}))
		})
	})

	When("Headless mode is turned on", func() {
		BeforeEach(func() {
			kafkaCluster.Spec.HeadlessServiceEnabled = true
		})

		It("does not add the all-broker service to the listener status", func(ctx SpecContext) {
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      kafkaCluster.Name,
				Namespace: kafkaCluster.Namespace,
			}, kafkaCluster)
			Expect(err).NotTo(HaveOccurred())

			Expect(kafkaCluster.Status.ListenerStatuses).To(Equal(v1beta1.ListenerStatuses{
				InternalListeners: map[string]v1beta1.ListenerStatusList{
					"internal": {
						{
							Name:    "headless",
							Address: fmt.Sprintf("%s-headless.kafka-istioingress-%d.svc.cluster.local:29092", kafkaCluster.Name, count),
						},
						{
							Name:    "broker-0",
							Address: fmt.Sprintf("%s-0.%s-headless.kafka-istioingress-%d.svc.cluster.local:29092", kafkaCluster.Name, kafkaCluster.Name, count),
						},
						{
							Name:    "broker-1",
							Address: fmt.Sprintf("%s-1.%s-headless.kafka-istioingress-%d.svc.cluster.local:29092", kafkaCluster.Name, kafkaCluster.Name, count),
						},
						{
							Name:    "broker-2",
							Address: fmt.Sprintf("%s-2.%s-headless.kafka-istioingress-%d.svc.cluster.local:29092", kafkaCluster.Name, kafkaCluster.Name, count),
						},
					},
				},
				ExternalListeners: map[string]v1beta1.ListenerStatusList{
					"external": {
						{
							Name:    "any-broker",
							Address: "ingress.test.host.com:29092",
						},
						{
							Name:    "broker-0",
							Address: "ingress.test.host.com:19090",
						},
						{
							Name:    "broker-1",
							Address: "ingress.test.host.com:19091",
						},
						{
							Name:    "broker-2",
							Address: "ingress.test.host.com:19092",
						},
					},
				},
			}))
		})
	})
})

var _ = Describe("KafkaClusterIstioIngressControllerWithBrokerIdBindings", func() {
	var (
		count              uint64 = 0
		namespace          string
		namespaceObj       *corev1.Namespace
		kafkaClusterCRName string
		kafkaCluster       *v1beta1.KafkaCluster
	)

	ExpectIstioIngressLabels := func(labels map[string]string, eListenerName, crName string) {
		Expect(labels).To(HaveKeyWithValue(v1beta1.AppLabelKey, "istioingress"))
		Expect(labels).To(HaveKeyWithValue("eListenerName", eListenerName))
		Expect(labels).To(HaveKeyWithValue(v1beta1.KafkaCRLabelKey, crName))
	}

	BeforeEach(func() {
		atomic.AddUint64(&count, 1)

		namespace = fmt.Sprintf("kafka-istioingress-with-bindings-%v", count)
		namespaceObj = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}

		kafkaClusterCRName = fmt.Sprintf("kafkacluster-%v", count)
		kafkaCluster = createMinimalKafkaClusterCR(kafkaClusterCRName, namespace)

		kafkaCluster.Spec.IngressController = istioingress.IngressControllerName
		kafkaCluster.Spec.IstioControlPlane = &v1beta1.IstioControlPlaneReference{Name: "istiod", Namespace: "istio-system"}
		kafkaCluster.Spec.ListenersConfig.ExternalListeners = []v1beta1.ExternalListenerConfig{
			{
				CommonListenerSpec: v1beta1.CommonListenerSpec{
					Type:          "plaintext",
					Name:          "external",
					ContainerPort: 9094,
				},
				ExternalStartingPort: 19090,
				Config: &v1beta1.Config{
					DefaultIngressConfig: "az1",
					IngressConfig: map[string]v1beta1.IngressConfig{
						"az1": {IstioIngressConfig: &v1beta1.IstioIngressConfig{
							Annotations: map[string]string{"zone": "az1"},
						},
						},
						"az2": {IstioIngressConfig: &v1beta1.IstioIngressConfig{
							Annotations: map[string]string{"zone": "az2"},
							TLSOptions: &istioclientv1beta1.TLSOptions{
								Mode:           istioclientv1beta1.TLSModeSimple,
								CredentialName: util.StringPointer("foobar"),
							},
						},
						},
					},
				},
			},
		}
		kafkaCluster.Spec.Brokers[0].BrokerConfig = &v1beta1.BrokerConfig{BrokerIngressMapping: []string{"az1"}}
		kafkaCluster.Spec.Brokers[1].BrokerConfig = &v1beta1.BrokerConfig{BrokerIngressMapping: []string{"az2"}}
	})

	JustBeforeEach(func(ctx SpecContext) {
		By("creating namespace " + namespace)
		err := k8sClient.Create(ctx, namespaceObj)
		Expect(err).NotTo(HaveOccurred())

		By("creating Kafka cluster object " + kafkaCluster.Name + " in namespace " + namespace)
		err = k8sClient.Create(ctx, kafkaCluster)
		Expect(err).NotTo(HaveOccurred())

		createMeshGatewayService(ctx, "external.az1.host.com",
			fmt.Sprintf("meshgateway-external-az1-%s", kafkaCluster.Name), namespace)
		createMeshGatewayService(ctx, "external.az2.host.com",
			fmt.Sprintf("meshgateway-external-az2-%s", kafkaCluster.Name), namespace)

		waitForClusterRunningState(ctx, kafkaCluster, namespace)
	})

	JustAfterEach(func(ctx SpecContext) {
		By("deleting Kafka cluster object " + kafkaCluster.Name + " in namespace " + namespace)
		err := k8sClient.Delete(ctx, kafkaCluster)
		Expect(err).NotTo(HaveOccurred())
		kafkaCluster = nil
	})

	When("Istio ingress controller is configured", func() {

		It("creates Istio ingress related objects", func(ctx SpecContext) {
			// Test that the mesh gateway deployment is created
			var deployment appsv1.Deployment
			meshGatewayAz1Name := fmt.Sprintf("meshgateway-external-az1-%s", kafkaCluster.Name)
			Eventually(ctx, func() error {
				err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: meshGatewayAz1Name}, &deployment)
				return err
			}).Should(Succeed())

			Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(1))
			Expect(deployment.Spec.Template.Spec.Containers[0].Name).To(Equal("istio-proxy"))

			// Test that the mesh gateway service is created
			var service corev1.Service
			Eventually(ctx, func() error {
				err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: meshGatewayAz1Name}, &service)
				return err
			}).Should(Succeed())

			Expect(service.Spec.Ports).To(HaveLen(3))

			// For LoadBalancer services, NodePort is automatically assigned by Kubernetes
			// So we check individual fields instead of comparing the entire struct
			Expect(service.Spec.Ports[0].Name).To(Equal("tcp-broker-0"))
			Expect(service.Spec.Ports[0].Protocol).To(Equal(corev1.ProtocolTCP))
			Expect(service.Spec.Ports[0].Port).To(Equal(int32(19090)))
			Expect(service.Spec.Ports[0].TargetPort).To(Equal(intstr.FromInt(19090)))

			Expect(service.Spec.Ports[1].Name).To(Equal("tcp-broker-2"))
			Expect(service.Spec.Ports[1].Protocol).To(Equal(corev1.ProtocolTCP))
			Expect(service.Spec.Ports[1].Port).To(Equal(int32(19092)))
			Expect(service.Spec.Ports[1].TargetPort).To(Equal(intstr.FromInt(19092)))

			Expect(service.Spec.Ports[2].Name).To(Equal("tcp-all-broker"))
			Expect(service.Spec.Ports[2].Protocol).To(Equal(corev1.ProtocolTCP))
			Expect(service.Spec.Ports[2].Port).To(Equal(int32(29092)))
			Expect(service.Spec.Ports[2].TargetPort).To(Equal(intstr.FromInt(29092)))

			var gateway istioclientv1beta1.Gateway
			gatewayName := fmt.Sprintf("%s-external-az1-gateway", kafkaCluster.Name)
			Eventually(ctx, func() error {
				err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: gatewayName}, &gateway)
				return err
			}).Should(Succeed())

			ExpectIstioIngressLabels(gateway.Labels, "external-az1", kafkaClusterCRName)
			ExpectIstioIngressLabels(gateway.Spec.Selector, "external-az1", kafkaClusterCRName)
			Expect(gateway.Spec.Servers).To(ConsistOf(
				istioclientv1beta1.Server{
					Port: &istioclientv1beta1.Port{
						Number:   19090,
						Protocol: "TCP",
						Name:     "tcp-broker-0"},
					Hosts: []string{"*"},
				},
				istioclientv1beta1.Server{
					Port: &istioclientv1beta1.Port{
						Number:   19092,
						Protocol: "TCP",
						Name:     "tcp-broker-2"},
					Hosts: []string{"*"},
				},
				istioclientv1beta1.Server{
					Port: &istioclientv1beta1.Port{
						Number:   29092,
						Protocol: "TCP",
						Name:     "tcp-all-broker",
					},
					Hosts: []string{"*"},
				}))

			var virtualService istioclientv1beta1.VirtualService
			virtualServiceName := fmt.Sprintf("%s-external-az1-virtualservice", kafkaCluster.Name)
			Eventually(ctx, func() error {
				err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: virtualServiceName}, &virtualService)
				return err
			}).Should(Succeed())

			ExpectIstioIngressLabels(virtualService.Labels, "external-az1", kafkaClusterCRName)
			Expect(virtualService.Spec).To(Equal(istioclientv1beta1.VirtualServiceSpec{
				Hosts:    []string{"*"},
				Gateways: []string{gatewayName},
				TCP: []istioclientv1beta1.TCPRoute{
					{
						Match: []istioclientv1beta1.L4MatchAttributes{{Port: util.IntPointer(19090)}},
						Route: []*istioclientv1beta1.RouteDestination{{
							Destination: &istioclientv1beta1.Destination{
								Host: "kafkacluster-1-0",
								Port: &istioclientv1beta1.PortSelector{Number: 9094},
							},
						}},
					},
					{
						Match: []istioclientv1beta1.L4MatchAttributes{{Port: util.IntPointer(19092)}},
						Route: []*istioclientv1beta1.RouteDestination{{
							Destination: &istioclientv1beta1.Destination{
								Host: "kafkacluster-1-2",
								Port: &istioclientv1beta1.PortSelector{Number: 9094},
							},
						}},
					},
					{
						Match: []istioclientv1beta1.L4MatchAttributes{{Port: util.IntPointer(29092)}},
						Route: []*istioclientv1beta1.RouteDestination{{
							Destination: &istioclientv1beta1.Destination{
								Host: "kafkacluster-1-all-broker",
								Port: &istioclientv1beta1.PortSelector{Number: 9094},
							},
						}},
					},
				},
			}))
			// Test Istio Ingress Az2 related objects
			var deploymentAz2 appsv1.Deployment
			meshGatewayAz2Name := fmt.Sprintf("meshgateway-external-az2-%s", kafkaCluster.Name)
			Eventually(ctx, func() error {
				err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: meshGatewayAz2Name}, &deploymentAz2)
				return err
			}).Should(Succeed())

			Expect(deploymentAz2.Spec.Template.Spec.Containers).To(HaveLen(1))
			Expect(deploymentAz2.Spec.Template.Spec.Containers[0].Name).To(Equal("istio-proxy"))

			// Test that the mesh gateway service is created for az2
			var serviceAz2 corev1.Service
			Eventually(ctx, func() error {
				err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: meshGatewayAz2Name}, &serviceAz2)
				return err
			}).Should(Succeed())

			Expect(serviceAz2.Spec.Ports).To(HaveLen(2))

			// For LoadBalancer services, NodePort is automatically assigned by Kubernetes
			// So we check individual fields instead of comparing the entire struct
			Expect(serviceAz2.Spec.Ports[0].Name).To(Equal("tcp-broker-1"))
			Expect(serviceAz2.Spec.Ports[0].Protocol).To(Equal(corev1.ProtocolTCP))
			Expect(serviceAz2.Spec.Ports[0].Port).To(Equal(int32(19091)))
			Expect(serviceAz2.Spec.Ports[0].TargetPort).To(Equal(intstr.FromInt(19091)))

			Expect(serviceAz2.Spec.Ports[1].Name).To(Equal("tcp-all-broker"))
			Expect(serviceAz2.Spec.Ports[1].Protocol).To(Equal(corev1.ProtocolTCP))
			Expect(serviceAz2.Spec.Ports[1].Port).To(Equal(int32(29092)))
			Expect(serviceAz2.Spec.Ports[1].TargetPort).To(Equal(intstr.FromInt(29092)))

			gatewayName = fmt.Sprintf("%s-external-az2-gateway", kafkaCluster.Name)
			Eventually(ctx, func() error {
				err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: gatewayName}, &gateway)
				return err
			}).Should(Succeed())

			ExpectIstioIngressLabels(gateway.Labels, "external-az2", kafkaClusterCRName)
			ExpectIstioIngressLabels(gateway.Spec.Selector, "external-az2", kafkaClusterCRName)
			Expect(gateway.Spec.Servers).To(ConsistOf(
				istioclientv1beta1.Server{
					TLS: &istioclientv1beta1.TLSOptions{
						Mode:           istioclientv1beta1.TLSModeSimple,
						CredentialName: util.StringPointer("foobar"),
					},
					Port: &istioclientv1beta1.Port{
						Number:   19091,
						Protocol: "TLS",
						Name:     "tcp-broker-1"},
					Hosts: []string{"*"},
				},
				istioclientv1beta1.Server{
					TLS: &istioclientv1beta1.TLSOptions{
						Mode:           istioclientv1beta1.TLSModeSimple,
						CredentialName: util.StringPointer("foobar"),
					},
					Port: &istioclientv1beta1.Port{
						Number:   29092,
						Protocol: "TLS",
						Name:     "tcp-all-broker",
					},
					Hosts: []string{"*"},
				}))

			virtualServiceName = fmt.Sprintf("%s-external-az2-virtualservice", kafkaCluster.Name)
			Eventually(ctx, func() error {
				err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: virtualServiceName}, &virtualService)
				return err
			}).Should(Succeed())

			ExpectIstioIngressLabels(virtualService.Labels, "external-az2", kafkaClusterCRName)
			Expect(virtualService.Spec).To(Equal(istioclientv1beta1.VirtualServiceSpec{
				Hosts:    []string{"*"},
				Gateways: []string{gatewayName},
				TCP: []istioclientv1beta1.TCPRoute{
					{
						Match: []istioclientv1beta1.L4MatchAttributes{{Port: util.IntPointer(19091)}},
						Route: []*istioclientv1beta1.RouteDestination{{
							Destination: &istioclientv1beta1.Destination{
								Host: "kafkacluster-1-1",
								Port: &istioclientv1beta1.PortSelector{Number: 9094},
							},
						}},
					},
					{
						Match: []istioclientv1beta1.L4MatchAttributes{{Port: util.IntPointer(29092)}},
						Route: []*istioclientv1beta1.RouteDestination{{
							Destination: &istioclientv1beta1.Destination{
								Host: "kafkacluster-1-all-broker",
								Port: &istioclientv1beta1.PortSelector{Number: 9094},
							},
						}},
					},
				},
			}))

			// expect kafkaCluster listener status
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      kafkaCluster.Name,
				Namespace: kafkaCluster.Namespace,
			}, kafkaCluster)
			Expect(err).NotTo(HaveOccurred())

			Expect(kafkaCluster.Status.ListenerStatuses).To(Equal(v1beta1.ListenerStatuses{
				InternalListeners: map[string]v1beta1.ListenerStatusList{
					"internal": {
						{
							Name:    "any-broker",
							Address: fmt.Sprintf("%s-all-broker.kafka-istioingress-with-bindings-%d.svc.cluster.local:29092", kafkaCluster.Name, count),
						},
						{
							Name:    "broker-0",
							Address: fmt.Sprintf("%s-0.kafka-istioingress-with-bindings-%d.svc.cluster.local:29092", kafkaCluster.Name, count),
						},
						{
							Name:    "broker-1",
							Address: fmt.Sprintf("%s-1.kafka-istioingress-with-bindings-%d.svc.cluster.local:29092", kafkaCluster.Name, count),
						},
						{
							Name:    "broker-2",
							Address: fmt.Sprintf("%s-2.kafka-istioingress-with-bindings-%d.svc.cluster.local:29092", kafkaCluster.Name, count),
						},
					},
				},
				ExternalListeners: map[string]v1beta1.ListenerStatusList{
					"external": {
						{
							Name:    "any-broker-az1",
							Address: "external.az1.host.com:29092",
						},
						{
							Name:    "any-broker-az2",
							Address: "external.az2.host.com:29092",
						},
						{
							Name:    "broker-0",
							Address: "external.az1.host.com:19090",
						},
						{
							Name:    "broker-1",
							Address: "external.az2.host.com:19091",
						},
						{
							Name:    "broker-2",
							Address: "external.az1.host.com:19092",
						},
					},
				},
			}))
		})
	})
})

func createMeshGatewayService(ctx context.Context, extListenerName, extListenerServiceName, namespace string) {
	svcFromMeshGateway := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      extListenerServiceName,
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeLoadBalancer,
			Ports: []corev1.ServicePort{
				// other ports omitted
				{
					Name:     "tcp-all-broker",
					Port:     29092, // from MeshGateway (guarded by the tests)
					Protocol: corev1.ProtocolTCP,
				},
			},
		},
	}
	err := k8sClient.Create(ctx, &svcFromMeshGateway)
	Expect(err).NotTo(HaveOccurred())
	svcFromMeshGateway.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{{Hostname: extListenerName}}
	err = k8sClient.Status().Update(ctx, &svcFromMeshGateway)
	Expect(err).NotTo(HaveOccurred())
}
