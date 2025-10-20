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

package clusterregistry

import (
	"fmt"
	"sync/atomic"

	clusterregv1alpha1 "github.com/cisco-open/cluster-registry-controller/api/v1alpha1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/banzaicloud/koperator/api/v1beta1"
)

var _ = Describe("KafkaClusterController", func() {
	var (
		count            uint64 = 0
		namespace        string
		namespaceObj     *corev1.Namespace
		kafkaClusterName string
		kafkaCluster     *v1beta1.KafkaCluster
	)

	BeforeEach(func() {
		atomic.AddUint64(&count, 1)

		namespace = fmt.Sprintf("cruise-control-%v", count)
		namespaceObj = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}

		kafkaClusterName = fmt.Sprintf("kafkacluster-%d", count)
		kafkaCluster = createMinimalKafkaClusterCR(kafkaClusterName, namespace)
	})

	JustBeforeEach(func() {
		By("creating namespace " + namespace)
		err := k8sClient.Create(ctx, namespaceObj)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		kafkaClusterReconciler.Reset()
	})

	Describe("Controller should ignore events for KafkaCluster CRs", func() {
		It("with OwnershipAnnotation is set", func() {
			By("creating the KafkaCluster CR")
			expectedNumOfReconciles := 0
			kafkaCluster.Annotations[clusterregv1alpha1.OwnershipAnnotation] = "id"
			err := k8sClient.Create(ctx, kafkaCluster)
			Expect(err).ToNot(HaveOccurred())
			Eventually(func() int {
				reconciles := kafkaClusterReconciler.NumOfRequests()
				log.Info("check reconciles on create", "reconciles", reconciles)
				return reconciles
			}, 10).Should(BeNumerically("==", expectedNumOfReconciles))

			By("updating the KafkaCluster CR")
			kafkaCluster.Spec.Brokers = append(kafkaCluster.Spec.Brokers,
				v1beta1.Broker{Id: 3, BrokerConfigGroup: defaultBrokerConfigGroup})
			err = k8sClient.Update(ctx, kafkaCluster)
			Expect(err).ToNot(HaveOccurred())
			Eventually(func() int {
				reconciles := kafkaClusterReconciler.NumOfRequests()
				log.Info("check reconciles on update", "reconciles", reconciles)
				return reconciles
			}, 10).Should(BeNumerically("==", expectedNumOfReconciles))

			By("deleting the KafkaCluster CR")
			err = k8sClient.Delete(ctx, kafkaCluster)
			Expect(err).ToNot(HaveOccurred())
			Eventually(func() int {
				reconciles := kafkaClusterReconciler.NumOfRequests()
				log.Info("check reconciles on delete", "reconciles", reconciles)
				return reconciles
			}, 10).Should(BeNumerically("==", expectedNumOfReconciles))
		})
	})

	Describe("Controller should not ignore events for KafkaCluster CRs", func() {
		It("with OwnershipAnnotation not set", func() {
			By("creating the KafkaCluster CR")
			expectedNumOfReconciles := 1
			err := k8sClient.Create(ctx, kafkaCluster)
			Expect(err).ToNot(HaveOccurred())
			Eventually(func() int {
				reconciles := kafkaClusterReconciler.NumOfRequests()
				log.Info("check reconciles on create", "reconciles", reconciles)
				return reconciles
			}, 10).Should(BeNumerically("==", expectedNumOfReconciles))

			By("updating the KafkaCluster CR")
			expectedNumOfReconciles += 1
			kafkaCluster.Spec.Brokers = append(kafkaCluster.Spec.Brokers,
				v1beta1.Broker{Id: 3, BrokerConfigGroup: defaultBrokerConfigGroup})
			err = k8sClient.Update(ctx, kafkaCluster)
			Expect(err).ToNot(HaveOccurred())
			Eventually(func() int {
				reconciles := kafkaClusterReconciler.NumOfRequests()
				log.Info("check reconciles on update", "reconciles", reconciles)
				return reconciles
			}, 10).Should(BeNumerically("==", expectedNumOfReconciles))

			By("deleting the KafkaCluster CR")
			expectedNumOfReconciles += 1
			err = k8sClient.Delete(ctx, kafkaCluster)
			Expect(err).ToNot(HaveOccurred())
			Eventually(func() int {
				reconciles := kafkaClusterReconciler.NumOfRequests()
				log.Info("check reconciles on delete", "reconciles", reconciles)
				return reconciles
			}, 10).Should(BeNumerically("==", expectedNumOfReconciles))
		})
	})

	Context("Reconcile - Basic Object Lifecycle", func() {
		It("should successfully create a KafkaCluster CR", func() {
			By("creating the KafkaCluster CR")
			err := k8sClient.Create(ctx, kafkaCluster)
			Expect(err).NotTo(HaveOccurred())

			By("verifying the cluster exists")
			cluster := &v1beta1.KafkaCluster{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      kafkaClusterName,
				Namespace: namespace,
			}, cluster)
			Expect(err).NotTo(HaveOccurred())
			Expect(cluster.Name).To(Equal(kafkaClusterName))
		})

		It("should allow updating KafkaCluster spec", func() {
			By("creating the KafkaCluster CR")
			err := k8sClient.Create(ctx, kafkaCluster)
			Expect(err).NotTo(HaveOccurred())

			By("updating the cluster spec")
			cluster := &v1beta1.KafkaCluster{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      kafkaClusterName,
				Namespace: namespace,
			}, cluster)
			Expect(err).NotTo(HaveOccurred())

			if cluster.Annotations == nil {
				cluster.Annotations = make(map[string]string)
			}
			cluster.Annotations["test-key"] = "test-value"
			err = k8sClient.Update(ctx, cluster)
			Expect(err).NotTo(HaveOccurred())

			By("verifying the update was applied")
			updatedCluster := &v1beta1.KafkaCluster{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      kafkaClusterName,
				Namespace: namespace,
			}, updatedCluster)
			Expect(err).NotTo(HaveOccurred())
			Expect(updatedCluster.Annotations["test-key"]).To(Equal("test-value"))
		})

		It("should handle multiple clusters in same namespace", func() {
			By("creating first cluster")
			err := k8sClient.Create(ctx, kafkaCluster)
			Expect(err).NotTo(HaveOccurred())

			By("creating second cluster")
			cluster2 := createMinimalKafkaClusterCR(fmt.Sprintf("%s-2", kafkaClusterName), namespace)
			err = k8sClient.Create(ctx, cluster2)
			Expect(err).NotTo(HaveOccurred())

			By("verifying both clusters exist")
			list := &v1beta1.KafkaClusterList{}
			err = k8sClient.List(ctx, list, &client.ListOptions{Namespace: namespace})
			Expect(err).NotTo(HaveOccurred())
			Expect(len(list.Items)).To(BeNumerically(">=", 2))
		})
	})
})
