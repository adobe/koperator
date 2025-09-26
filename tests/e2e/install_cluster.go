// Copyright © 2023 Cisco Systems, Inc. and/or its affiliates
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

package e2e

import (
	"context"
	"fmt"
	"time"

	"github.com/gruntwork-io/terratest/modules/k8s"

	ginkgo "github.com/onsi/ginkgo/v2"
	gomega "github.com/onsi/gomega"

	"github.com/banzaicloud/koperator/api/v1beta1"
)

// requireCreatingKafkaCluster creates a KafkaCluster and
// checks the success of that operation.
func requireCreatingKafkaCluster(kubectlOptions k8s.KubectlOptions, manifestPath string) {
	ginkgo.It("Deploying a KafkaCluster", func() {
		ginkgo.By("Checking existing KafkaClusters")
		found := isExistingK8SResource(kubectlOptions, kafkaKind, kafkaClusterName)
		if found {
			ginkgo.By(fmt.Sprintf("KafkaCluster %s already exists\n", kafkaClusterName))
		} else {
			ginkgo.By("Deploying a KafkaCluster")
			applyK8sResourceManifest(kubectlOptions, manifestPath)
		}

		ginkgo.By("Verifying the KafkaCluster state")
		err := waitForKafkaClusterWithPodStatusCheck(kubectlOptions, kafkaClusterName, kafkaClusterCreateTimeout)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		ginkgo.By("Verifying the CruiseControl pod")
		gomega.Eventually(context.Background(), func() error {
			return waitK8sResourceCondition(kubectlOptions, "pod", "condition=Ready", cruiseControlPodReadinessTimeout, v1beta1.KafkaCRLabelKey+"="+kafkaClusterName+",app=cruisecontrol", "")
		}, kafkaClusterResourceReadinessTimeout, 3*time.Second).ShouldNot(gomega.HaveOccurred())

		ginkgo.By("Verifying all Kafka pods")
		err = waitK8sResourceCondition(kubectlOptions, "pod", "condition=Ready", defaultPodReadinessWaitTime, v1beta1.KafkaCRLabelKey+"="+kafkaClusterName+",app=kafka", "")
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	})
}

// requireCreatingZookeeperCluster creates a ZookeeperCluster and
// checks the success of that operation.
func requireCreatingZookeeperCluster(kubectlOptions k8s.KubectlOptions) {
	ginkgo.It("Deploying a ZookeeperCluster", func() {
		ginkgo.By("Checking existing ZookeeperClusters")
		found := isExistingK8SResource(kubectlOptions, zookeeperKind, zookeeperClusterName)
		if found {
			ginkgo.By(fmt.Sprintf("ZookeeperCluster %s already exists\n", zookeeperClusterName))
		} else {
			ginkgo.By("Deploying the sample ZookeeperCluster")
			err := applyK8sResourceFromTemplate(kubectlOptions,
				zookeeperClusterTemplate,
				map[string]interface{}{
					"Name":      zookeeperClusterName,
					"Namespace": kubectlOptions.Namespace,
					"Replicas":  zookeeperClusterReplicaCount,
				},
			)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		}

		ginkgo.By("Verifying the ZookeeperCluster resource")
		err := waitK8sResourceCondition(kubectlOptions, zookeeperKind, "jsonpath={.status.readyReplicas}=1", zookeeperClusterCreateTimeout, "", zookeeperClusterName)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		ginkgo.By("Verifying the ZookeeperCluster's pods")
		err = waitK8sResourceCondition(kubectlOptions, "pod", "condition=Ready", defaultPodReadinessWaitTime, "app="+zookeeperClusterName, "")
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	})
}
