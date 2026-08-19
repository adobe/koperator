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
	"strconv"
	"strings"
	"time"

	"github.com/gruntwork-io/terratest/modules/k8s"
	ginkgo "github.com/onsi/ginkgo/v2"
	gomega "github.com/onsi/gomega"

	"github.com/banzaicloud/koperator/api/v1alpha1"
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

		ginkgo.It("Waiting for Cruise Control to be ready and idle before triggering removal", func() {
			// A freshly installed cluster runs an initial CruiseControl rebalance to spread replicas
			// across the new brokers. Triggering remove_broker while that rebalance is still in flight
			// makes the two operations race and the removal can stall. Gate on a running cluster with no
			// in-flight CC operation so removal starts from a quiescent Cruise Control.
			ginkgo.By("Ensuring the KafkaCluster is running")
			err := waitForKafkaClusterWithPodStatusCheck(kubectlOptions, kafkaClusterName, kafkaClusterResourceReadinessTimeout)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Waiting until no Cruise Control operation is in flight (initial rebalance finished)")
			gomega.Eventually(context.Background(), func() (bool, error) {
				return hasNoInFlightCruiseControlOperation(kubectlOptions)
			}, batchedBrokerRemovalTimeout, batchedBrokerRemovalPollInterval).Should(gomega.BeTrue())

			// The idle-operation gate above only checks CruiseControl's task queue, not the CC
			// Deployment itself. If the operator is mid-rollout of a new CC revision, two CC pods
			// race and the new one may never become Ready, resetting CC's metric-sampling window and
			// stalling the remove_broker task. Gate on a settled CC Deployment so removal starts from
			// a single, fully rolled-out CruiseControl pod.
			ginkgo.By("Waiting until the Cruise Control Deployment is fully rolled out (single Ready replica)")
			gomega.Eventually(context.Background(), func() (bool, error) {
				return isCruiseControlDeploymentRolledOut(kubectlOptions)
			}, batchedBrokerRemovalTimeout, batchedBrokerRemovalPollInterval).Should(gomega.BeTrue())
		})

		ginkgo.It("Removing brokers 3 and 4 from the running KafkaCluster", func() {
			ginkgo.By("Fetching the running KafkaCluster and patching out brokers 3 and 4")
			err := removeKafkaClusterBrokers(kubectlOptions, kafkaClusterName, 3, 4)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
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
			// Broker removal reconciles the cluster; gate on the operator's own ClusterRunning
			// state and re-resolve the live pod set each poll rather than `kubectl wait`-ing on
			// a snapshot of pod names that can change while the cluster settles.
			err := waitForKafkaClusterWithPodStatusCheck(kubectlOptions, kafkaClusterName, kafkaClusterResourceReadinessTimeout)
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
		if op == string(v1alpha1.OperationRemoveBroker) {
			count++
		}
	}
	return count == 1, nil
}

// hasNoInFlightCruiseControlOperation returns true when no CruiseControlOperation in the namespace
// has a currently-running task (Active or InExecution). Completed / CompletedWithError tasks and
// operations without a currentTask count as idle. It is used to gate mutation tests (broker/disk
// removal) on a quiescent Cruise Control so a new operation does not race an in-flight one, such as
// the rebalance CruiseControl runs right after a fresh cluster install.
func hasNoInFlightCruiseControlOperation(kubectlOptions k8s.KubectlOptions) (bool, error) {
	states, err := getK8sResources(kubectlOptions,
		[]string{"cruisecontroloperation"},
		"",
		"",
		"-o", "jsonpath={range .items[*]}{.status.currentTask.state}{'\\n'}{end}",
	)
	if err != nil {
		return false, err
	}
	for _, state := range states {
		if state == string(v1beta1.CruiseControlTaskActive) || state == string(v1beta1.CruiseControlTaskInExecution) {
			return false, nil
		}
	}
	return true, nil
}

// hasExactlyNBrokerPods returns true when exactly n broker pods exist in the namespace.
func hasExactlyNBrokerPods(kubectlOptions k8s.KubectlOptions, n int) (bool, error) {
	pods, err := getK8sResources(kubectlOptions,
		[]string{"pod"},
		v1beta1.KafkaCRLabelKey+"="+kafkaClusterName+","+kafkaLabelSelectorBrokers,
		"",
		"--field-selector=status.phase=Running",
		"-o", "name",
	)
	if err != nil {
		return false, err
	}
	return len(pods) == n, nil
}

// isCruiseControlDeploymentRolledOut returns true when the Cruise Control Deployment has fully
// settled to a single Ready replica: the operator's latest spec has been observed
// (observedGeneration == generation) and the spec/total/updated/ready/available replica counts all
// agree, so no pod from a previous revision is lingering. Broker removal drives the operator to
// regenerate CruiseControl's capacity config; starting removal while CC is mid-rollout leaves two
// CC pods racing and resets CC's metric-sampling window, which can stall an in-flight remove_broker
// task. Gating on a settled CC Deployment avoids that race.
func isCruiseControlDeploymentRolledOut(kubectlOptions k8s.KubectlOptions) (bool, error) {
	// One line per CC Deployment: generation/observedGeneration/spec.replicas/status.replicas/
	// updatedReplicas/readyReplicas/availableReplicas. Absent status counts render as empty.
	lines, err := getK8sResources(kubectlOptions,
		[]string{"deployment"},
		v1beta1.KafkaCRLabelKey+"="+kafkaClusterName+",app=cruisecontrol",
		"",
		"-o", "jsonpath={range .items[*]}{.metadata.generation}/{.status.observedGeneration}/{.spec.replicas}/{.status.replicas}/{.status.updatedReplicas}/{.status.readyReplicas}/{.status.availableReplicas}{'\\n'}{end}",
	)
	if err != nil {
		return false, err
	}
	// Exactly one Cruise Control Deployment is expected; anything else is not a settled state.
	if len(lines) != 1 {
		return false, nil
	}

	fields := strings.Split(lines[0], "/")
	if len(fields) != 7 {
		return false, nil
	}
	nums := make([]int, len(fields))
	for i, f := range fields {
		if f == "" {
			// A missing status replica count (e.g. readyReplicas before any pod is Ready) is 0.
			nums[i] = 0
			continue
		}
		n, convErr := strconv.Atoi(f)
		if convErr != nil {
			return false, nil
		}
		nums[i] = n
	}

	generation, observed := nums[0], nums[1]
	specReplicas, statusReplicas, updated, ready, available := nums[2], nums[3], nums[4], nums[5], nums[6]

	return observed == generation &&
		specReplicas >= 1 &&
		statusReplicas == specReplicas &&
		updated == specReplicas &&
		ready == specReplicas &&
		available == specReplicas, nil
}
