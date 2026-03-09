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
	"fmt"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/banzaicloud/koperator/api/v1beta1"
	"github.com/banzaicloud/koperator/pkg/util"
)

var _ = Describe("KafkaClusterWithIngressControllerOverride", Label("contour"), func() {
	var (
		count        uint64 = 0
		namespace    string
		namespaceObj *corev1.Namespace
		kafkaCluster *v1beta1.KafkaCluster
	)

	BeforeEach(func() {
		atomic.AddUint64(&count, 1)
		namespace = fmt.Sprintf("kafkacontouroverridetest-%v", count)
		namespaceObj = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}

		kafkaCluster = createMinimalKafkaClusterCR(fmt.Sprintf("kafkacluster-%d", count), namespace)

		// Cluster default is envoy; listener overrides to contour.
		kafkaCluster.Spec.IngressController = "envoy"

		kafkaCluster.Spec.ListenersConfig.ExternalListeners = []v1beta1.ExternalListenerConfig{
			{
				CommonListenerSpec: v1beta1.CommonListenerSpec{
					Name:          "ingress1",
					Type:          v1beta1.SecurityProtocolPlaintext,
					ContainerPort: 29098,
				},
				IngressController:    "contour",
				AccessMethod:         corev1.ServiceTypeClusterIP,
				ExternalStartingPort: -1,
				AnyCastPort:          util.Int32Pointer(8443),
				Config: &v1beta1.Config{
					DefaultIngressConfig: "",
					IngressConfig: map[string]v1beta1.IngressConfig{
						"ingress1": {
							IngressServiceSettings: v1beta1.IngressServiceSettings{
								HostnameOverride: "kafka.cluster.local",
							},
							ContourIngressConfig: &v1beta1.ContourIngressConfig{
								TLSSecretName:      "test-tls-secret",
								BrokerFQDNTemplate: "broker-%id.kafka.cluster.local",
							},
						},
					},
				},
			},
		}

		kafkaCluster.Spec.Brokers[0].BrokerConfig = &v1beta1.BrokerConfig{BrokerIngressMapping: []string{"ingress1"}}
		kafkaCluster.Spec.Brokers[1].BrokerConfig = &v1beta1.BrokerConfig{BrokerIngressMapping: []string{"ingress1"}}
		kafkaCluster.Spec.Brokers[2].BrokerConfig = &v1beta1.BrokerConfig{BrokerIngressMapping: []string{"ingress1"}}
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

	It("creates contour resources when a listener overrides the cluster ingress controller", func(ctx SpecContext) {
		expectContour(ctx, kafkaCluster)
	})
})
