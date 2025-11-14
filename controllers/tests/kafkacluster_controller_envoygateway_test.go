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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/banzaicloud/koperator/api/v1beta1"
)

var _ = Describe("KafkaClusterWithEnvoyGatewayIngressController", Label("envoygateway"), func() {
	var (
		count        uint64 = 0
		namespace    string
		namespaceObj *corev1.Namespace
		kafkaCluster *v1beta1.KafkaCluster
	)

	BeforeEach(func() {
		atomic.AddUint64(&count, 1)
		namespace = fmt.Sprintf("kafkaenvoygatewaytest-%v", count)
		namespaceObj = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}

		kafkaCluster = createMinimalKafkaClusterCR(fmt.Sprintf("kafkacluster-%d", count), namespace)
		kafkaCluster.Spec.IngressController = "envoygateway"
		kafkaCluster.Spec.EnvoyGatewayConfig = v1beta1.EnvoyGatewayIngressConfig{
			GatewayClassName:       "eg",
			BrokerHostnameTemplate: "broker-%id.kafka.cluster.local",
		}

		envoyGatewayListener := kafkaCluster.Spec.ListenersConfig.ExternalListeners[0]
		envoyGatewayListener.AccessMethod = corev1.ServiceTypeLoadBalancer
		envoyGatewayListener.ExternalStartingPort = 19090
		envoyGatewayListener.Type = "plaintext"
		envoyGatewayListener.Name = "listener1"

		kafkaCluster.Spec.ListenersConfig.ExternalListeners[0] = envoyGatewayListener
	})

	JustBeforeEach(func(ctx SpecContext) {
		By("creating namespace " + namespace)
		err := k8sClient.Create(ctx, namespaceObj)
		Expect(err).NotTo(HaveOccurred())

		By("creating kafka cluster object " + kafkaCluster.Name + " in namespace " + namespace)
		err = k8sClient.Create(ctx, kafkaCluster)
		Expect(err).NotTo(HaveOccurred())

		waitForClusterRunningState(ctx, kafkaCluster, namespace)
	})

	JustAfterEach(func(ctx SpecContext) {
		By("deleting Kafka cluster object " + kafkaCluster.Name + " in namespace " + namespace)
		err := k8sClient.Delete(ctx, kafkaCluster)
		Expect(err).NotTo(HaveOccurred())

		kafkaCluster = nil
	})

	When("configuring Envoy Gateway ingress with TCP routes", func() {
		It("should reconcile Gateway and TCPRoute objects properly", func(ctx SpecContext) {
			expectEnvoyGateway(ctx, kafkaCluster, "listener1")
			expectEnvoyGatewayTCPRoutes(ctx, kafkaCluster, "listener1")
		})
	})
})

func expectEnvoyGatewayLabels(labels map[string]string, eListenerName, crName string) {
	Expect(labels).To(HaveKeyWithValue(v1beta1.AppLabelKey, "envoygateway"))
	Expect(labels).To(HaveKeyWithValue("eListenerName", eListenerName))
	Expect(labels).To(HaveKeyWithValue(v1beta1.KafkaCRLabelKey, crName))
}

func expectEnvoyGateway(ctx context.Context, kafkaCluster *v1beta1.KafkaCluster, eListenerName string) {
	var gateway gatewayv1.Gateway
	gatewayName := fmt.Sprintf("kafka-gateway-%s", eListenerName)
	Eventually(ctx, func() error {
		err := k8sClient.Get(ctx, types.NamespacedName{Namespace: kafkaCluster.Namespace, Name: gatewayName}, &gateway)
		return err
	}).Should(Succeed())

	expectEnvoyGatewayLabels(gateway.Labels, eListenerName, kafkaCluster.Name)
	Expect(string(gateway.Spec.GatewayClassName)).To(Equal("eg"))

	// Check listeners
	if kafkaCluster.Spec.KRaftMode {
		// 2 brokers + 1 anycast = 3 listeners
		Expect(gateway.Spec.Listeners).To(HaveLen(3))
	} else {
		// 3 brokers + 1 anycast = 4 listeners
		Expect(gateway.Spec.Listeners).To(HaveLen(4))
	}

	// Verify broker listeners
	brokerCount := len(kafkaCluster.Spec.Brokers)
	for i := 0; i < brokerCount; i++ {
		listener := gateway.Spec.Listeners[i]
		Expect(string(listener.Name)).To(Equal(fmt.Sprintf("broker-%d", kafkaCluster.Spec.Brokers[i].Id)))
		Expect(listener.Port).To(BeEquivalentTo(19090 + kafkaCluster.Spec.Brokers[i].Id))
		Expect(listener.Protocol).To(Equal(gatewayv1.TCPProtocolType))
	}

	// Verify anycast listener
	anycastListener := gateway.Spec.Listeners[brokerCount]
	Expect(string(anycastListener.Name)).To(Equal("anycast"))
	// Anycast listener should use the default anycast port (29092), not ExternalStartingPort
	Expect(anycastListener.Port).To(BeEquivalentTo(29092))
	Expect(anycastListener.Protocol).To(Equal(gatewayv1.TCPProtocolType))
}

func expectEnvoyGatewayTCPRoutes(ctx context.Context, kafkaCluster *v1beta1.KafkaCluster, eListenerName string) {
	brokerCount := len(kafkaCluster.Spec.Brokers)

	// Check TCPRoute for each broker
	for i := 0; i < brokerCount; i++ {
		var tcpRoute gatewayv1alpha2.TCPRoute
		tcpRouteName := fmt.Sprintf("kafka-tcproute-%s-%d", eListenerName, kafkaCluster.Spec.Brokers[i].Id)
		Eventually(ctx, func() error {
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: kafkaCluster.Namespace, Name: tcpRouteName}, &tcpRoute)
			return err
		}).Should(Succeed())

		expectEnvoyGatewayLabels(tcpRoute.Labels, eListenerName, kafkaCluster.Name)

		// Verify parent reference
		Expect(tcpRoute.Spec.ParentRefs).To(HaveLen(1))
		Expect(string(tcpRoute.Spec.ParentRefs[0].Name)).To(Equal(fmt.Sprintf("kafka-gateway-%s", eListenerName)))
		Expect(string(*tcpRoute.Spec.ParentRefs[0].SectionName)).To(Equal(fmt.Sprintf("broker-%d", kafkaCluster.Spec.Brokers[i].Id)))

		// Verify backend reference
		Expect(tcpRoute.Spec.Rules).To(HaveLen(1))
		Expect(tcpRoute.Spec.Rules[0].BackendRefs).To(HaveLen(1))
		Expect(string(tcpRoute.Spec.Rules[0].BackendRefs[0].Name)).To(Equal(fmt.Sprintf("%s-all-broker", kafkaCluster.Name)))
	}

	// Check anycast TCPRoute
	var anycastTCPRoute gatewayv1alpha2.TCPRoute
	anycastTCPRouteName := fmt.Sprintf("kafka-tcproute-%s-anycast", eListenerName)
	Eventually(ctx, func() error {
		err := k8sClient.Get(ctx, types.NamespacedName{Namespace: kafkaCluster.Namespace, Name: anycastTCPRouteName}, &anycastTCPRoute)
		return err
	}).Should(Succeed())

	expectEnvoyGatewayLabels(anycastTCPRoute.Labels, eListenerName, kafkaCluster.Name)
	Expect(anycastTCPRoute.Spec.ParentRefs).To(HaveLen(1))
	Expect(string(*anycastTCPRoute.Spec.ParentRefs[0].SectionName)).To(Equal("anycast"))
}
