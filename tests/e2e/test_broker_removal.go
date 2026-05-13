// Copyright © 2025 Cisco Systems, Inc. and/or its affiliates
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

//go:build e2e

package e2e

import (
	"context"
	"time"

	"github.com/gruntwork-io/terratest/modules/k8s"
	ginkgo "github.com/onsi/ginkgo/v2"
	gomega "github.com/onsi/gomega"

	"github.com/banzaicloud/koperator/api/v1beta1"
)

const (
	batchedBrokerRemovalTimeout      = 1200 * time.Second
	batchedBrokerRemovalPollInterval = 15 * time.Second
)

// testBatchedBrokerRemoval applies the 3-broker manifest over the running 5-broker cluster,
// waits for CruiseControl to complete removal, then asserts exactly one remove_broker
// CruiseControlOperation was created and only 3 broker pods remain Ready.
func testBatchedBrokerRemoval() bool {
	return ginkgo.When("Batched broker removal: remove two brokers and assert single CC operation", func() {
		var kubectlOptions k8s.KubectlOptions
		var err error

		ginkgo.It("Acquiring K8s config and context", func() {
			kubectlOptions, err = kubectlOptionsForCurrentContext()
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			kubectlOptions.Namespace = koperatorLocalHelmDescriptor.Namespace
		})

		ginkgo.It("Applying 3-broker manifest to trigger removal of brokers 3 and 4", func() {
			ginkgo.By("Patching KafkaCluster to remove brokers 3 and 4")
			applyK8sResourceManifest(kubectlOptions, "../../config/samples/simplekafkacluster.yaml")
		})

		ginkgo.It("Waiting for exactly one remove_broker CruiseControlOperation to be created", func() {
			ginkgo.By("Polling until exactly one remove_broker CruiseControlOperation exists")
			gomega.Eventually(context.Background(), func() (bool, error) {
				return hasExactlyOneRemoveBrokerOperation(kubectlOptions)
			}, batchedBrokerRemovalTimeout, batchedBrokerRemovalPollInterval).Should(gomega.BeTrue())
		})

		ginkgo.It("Asserting exactly one remove_broker CruiseControlOperation was created", func() {
			ok, err := hasExactlyOneRemoveBrokerOperation(kubectlOptions)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(ok).To(gomega.BeTrue(), "expected exactly one remove_broker CruiseControlOperation")
		})

		ginkgo.It("Waiting for brokers 3 and 4 to be removed (only 3 pods remain)", func() {
			ginkgo.By("Waiting until only 3 kafka broker pods are Ready")
			gomega.Eventually(context.Background(), func() (bool, error) {
				return hasExactlyNBrokerPods(kubectlOptions, 3)
			}, batchedBrokerRemovalTimeout, batchedBrokerRemovalPollInterval).Should(gomega.BeTrue())
		})

		ginkgo.It("Asserting remaining Kafka brokers are healthy", func() {
			err := waitK8sResourceCondition(kubectlOptions, "pod", "condition=Ready", defaultPodReadinessWaitTime,
				v1beta1.KafkaCRLabelKey+"="+kafkaClusterName+","+kafkaLabelSelectorBrokers, "")
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		})
	})
}

// hasExactlyOneRemoveBrokerOperation returns true if there is exactly one CruiseControlOperation
// of type remove_broker in the namespace.
func hasExactlyOneRemoveBrokerOperation(kubectlOptions k8s.KubectlOptions) (bool, error) {
	ops, err := getK8sResources(kubectlOptions,
		[]string{"cruisecontroloperation"},
		"",
		"",
		"-o", "jsonpath={range .items[*]}{.status.currentTask.operation}{'\\n'}{end}",
	)
	if err != nil {
		return false, err
	}

	count := 0
	for _, op := range ops {
		if op == "removeBroker" {
			count++
		}
	}
	return count == 1, nil
}

// hasExactlyNBrokerPods returns true when exactly n broker pods exist in the namespace.
func hasExactlyNBrokerPods(kubectlOptions k8s.KubectlOptions, n int) (bool, error) {
	pods, err := getK8sResources(kubectlOptions,
		[]string{"pod"},
		v1beta1.KafkaCRLabelKey+"="+kafkaClusterName+","+kafkaLabelSelectorBrokers,
		"",
		"--field-selector=status.phase=Running",
	)
	if err != nil {
		return false, err
	}
	// subtract 1 for the header line
	actual := len(pods) - 1
	if actual < 0 {
		actual = 0
	}
	return actual == n, nil
}
