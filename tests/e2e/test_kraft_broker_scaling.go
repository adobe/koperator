// Copyright 2026 Adobe. All rights reserved.
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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"

	"github.com/gruntwork-io/terratest/modules/k8s"
	ginkgo "github.com/onsi/ginkgo/v2"
	gomega "github.com/onsi/gomega"

	"github.com/banzaicloud/koperator/api/v1alpha1"
	"github.com/banzaicloud/koperator/api/v1beta1"
)

// testKRaftBrokerScaling upscales a KRaft cluster from 3 broker-only nodes to 4 (add broker 103) and then
// downscales it back to 3, asserting Cruise Control drives each direction with exactly one
// add_broker / remove_broker operation, the broker-only node set tracks the spec by exact id, and the 3
// controller-only nodes (ids 0,1,2) are never touched (controller-only nodes are not CC brokers). The
// assertions check exact broker/controller ids - not just pod counts - so a manifest that silently swaps
// ids (rather than adding a single broker) would fail instead of passing green.
//
// This is the regression test for #301, and it deliberately exercises BOTH directions:
//   - UPSCALE is the case a "stop hashing capacity.json" fix would have silently broken: the capacity roll
//     is what loads the new broker's exact capacity into CC, so the fix must keep rolling on add and
//     sequence the add_broker op after the roll settles.
//   - DOWNSCALE is the case that stalled on master: the pre-downscale capacity roll kept CC un-ready so the
//     remove_broker op was never created.
func testKRaftBrokerScaling() bool {
	return ginkgo.When("KRaft broker scaling: upscale then downscale, controllers untouched", func() {
		var kubectlOptions k8s.KubectlOptions
		var err error

		ginkgo.It("Acquiring K8s config and context", func() {
			kubectlOptions, err = kubectlOptionsForCurrentContext()
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			kubectlOptions.Namespace = koperatorLocalHelmDescriptor.Namespace
		})

		ginkgo.It("Waiting for Cruise Control to be ready and settled before upscale", func() {
			ginkgo.By("Ensuring the KafkaCluster is running")
			gomega.Expect(waitForKafkaClusterWithPodStatusCheck(kubectlOptions, kafkaClusterName, kafkaClusterResourceReadinessTimeout)).NotTo(gomega.HaveOccurred())
			ginkgo.By("Waiting until no Cruise Control operation is in flight (initial rebalance finished)")
			gomega.Eventually(context.Background(), func() (bool, error) {
				return hasNoInFlightCruiseControlOperation(kubectlOptions)
			}, batchedBrokerRemovalTimeout, batchedBrokerRemovalPollInterval).Should(gomega.BeTrue())
			ginkgo.By("Waiting until the Cruise Control Deployment is fully rolled out (single Ready replica)")
			gomega.Eventually(context.Background(), func() (bool, error) {
				return isCruiseControlDeploymentRolledOut(kubectlOptions)
			}, batchedBrokerRemovalTimeout, batchedBrokerRemovalPollInterval).Should(gomega.BeTrue())
		})

		ginkgo.It("Asserting the cluster starts with broker-only nodes 100,101,102 and controllers 0,1,2", func() {
			ids, err := brokerPodIDs(kubectlOptions)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(ids).To(gomega.ConsistOf("100", "101", "102"), "unexpected broker-only ids before scaling")
			ids, err = controllerPodIDs(kubectlOptions)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(ids).To(gomega.ConsistOf("0", "1", "2"), "unexpected controller-only ids before scaling")
		})

		// --- Upscale: add broker 103 ---

		ginkgo.It("Applying the 4-broker KRaft manifest to add broker 103", func() {
			applyK8sResourceManifest(kubectlOptions, "../../config/samples/kraft/simplekafkacluster_kraft_4broker.yaml")
		})

		ginkgo.It("Waiting for exactly one add_broker CruiseControlOperation", func() {
			gomega.Eventually(context.Background(), func() (bool, error) {
				return hasExactlyNBrokerOperations(kubectlOptions, v1alpha1.OperationAddBroker, 1)
			}, batchedBrokerRemovalTimeout, batchedBrokerRemovalPollInterval).Should(gomega.BeTrue())
		})

		ginkgo.It("Waiting for broker 103 to join (broker-only nodes 100,101,102,103) and the cluster to be healthy", func() {
			gomega.Eventually(context.Background(), func() ([]string, error) {
				return brokerPodIDs(kubectlOptions)
			}, batchedBrokerRemovalTimeout, batchedBrokerRemovalPollInterval).Should(gomega.ConsistOf("100", "101", "102", "103"))
			gomega.Expect(waitForKafkaClusterWithPodStatusCheck(kubectlOptions, kafkaClusterName, kafkaClusterResourceReadinessTimeout)).NotTo(gomega.HaveOccurred())
		})

		ginkgo.It("Asserting controllers are untouched after upscale (still exactly controllers 0,1,2)", func() {
			ids, err := controllerPodIDs(kubectlOptions)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(ids).To(gomega.ConsistOf("0", "1", "2"), "controller-only nodes must not be affected by an upscale")
		})

		ginkgo.It("Asserting Cruise Control's capacity.json gained a real entry for the added broker 103", func() {
			// add_broker runs with AllowCapacityEstimation=true, so a healthy add alone does not prove the
			// operator wrote broker 103's capacity - CC could have estimated it. Assert the per-broker entry
			// is actually present so a regression that stops generating/rolling capacity.json is caught.
			gomega.Eventually(context.Background(), func() ([]string, error) {
				return cruiseControlCapacityBrokerIDs(kubectlOptions)
			}, batchedBrokerRemovalTimeout, batchedBrokerRemovalPollInterval).Should(gomega.ContainElement("103"),
				"broker 103's capacity must be written to capacity.json, not left to CC estimation")
		})

		ginkgo.It("Asserting the running CC pod's template carries the current capacity.json hash", func() {
			// The per-broker entry existing in the ConfigMap is not enough: the capacity must also have been
			// hashed into the pod template and rolled out. Assert the hash on the *running* CC pod (not just
			// the Deployment template) matches the current capacity.json - so a Ready CC pod started from that
			// capacity. It does not by itself prove the CC process re-read the file, but combined with a Ready
			// pod it is strong evidence the roll landed rather than add_broker relying on capacity estimation.
			gomega.Eventually(context.Background(), func() (bool, error) {
				return runningCruiseControlPodHasCurrentCapacityHash(kubectlOptions)
			}, batchedBrokerRemovalTimeout, batchedBrokerRemovalPollInterval).Should(gomega.BeTrue(),
				"the running CC pod's template must carry the hash of the current capacity.json")
		})

		// --- Downscale: remove broker 103 ---

		ginkgo.It("Waiting for Cruise Control to be settled again before downscale", func() {
			gomega.Eventually(context.Background(), func() (bool, error) {
				return hasNoInFlightCruiseControlOperation(kubectlOptions)
			}, batchedBrokerRemovalTimeout, batchedBrokerRemovalPollInterval).Should(gomega.BeTrue())
			gomega.Eventually(context.Background(), func() (bool, error) {
				return isCruiseControlDeploymentRolledOut(kubectlOptions)
			}, batchedBrokerRemovalTimeout, batchedBrokerRemovalPollInterval).Should(gomega.BeTrue())
		})

		ginkgo.It("Applying the 3-broker KRaft manifest to remove broker 103", func() {
			applyK8sResourceManifest(kubectlOptions, "../../config/samples/kraft/simplekafkacluster_kraft.yaml")
		})

		ginkgo.It("Waiting for exactly one remove_broker CruiseControlOperation", func() {
			gomega.Eventually(context.Background(), func() (bool, error) {
				return hasExactlyNBrokerOperations(kubectlOptions, v1alpha1.OperationRemoveBroker, 1)
			}, batchedBrokerRemovalTimeout, batchedBrokerRemovalPollInterval).Should(gomega.BeTrue())
		})

		ginkgo.It("Waiting for broker 103 to be removed (broker-only nodes back to 100,101,102) and the cluster to be healthy", func() {
			gomega.Eventually(context.Background(), func() ([]string, error) {
				return brokerPodIDs(kubectlOptions)
			}, batchedBrokerRemovalTimeout, batchedBrokerRemovalPollInterval).Should(gomega.ConsistOf("100", "101", "102"))
			gomega.Expect(waitForKafkaClusterWithPodStatusCheck(kubectlOptions, kafkaClusterName, kafkaClusterResourceReadinessTimeout)).NotTo(gomega.HaveOccurred())
		})

		ginkgo.It("Asserting controllers are untouched after downscale (still exactly controllers 0,1,2)", func() {
			ids, err := controllerPodIDs(kubectlOptions)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(ids).To(gomega.ConsistOf("0", "1", "2"), "controller-only nodes must not be affected by a downscale")
		})

		ginkgo.It("Asserting Cruise Control's capacity.json dropped the removed broker 103", func() {
			// Once broker 103's pod is gone and it leaves the status, the merge stops preserving its entry
			// and capacity.json shrinks back - the departing broker's capacity must not linger indefinitely.
			gomega.Eventually(context.Background(), func() ([]string, error) {
				return cruiseControlCapacityBrokerIDs(kubectlOptions)
			}, batchedBrokerRemovalTimeout, batchedBrokerRemovalPollInterval).ShouldNot(gomega.ContainElement("103"),
				"broker 103's capacity must be removed from capacity.json once it is fully downscaled")
		})
	})
}

// hasExactlyNBrokerOperations returns true when exactly n CruiseControlOperations whose current task is the
// given operation type exist in the namespace.
func hasExactlyNBrokerOperations(kubectlOptions k8s.KubectlOptions, operation v1alpha1.CruiseControlTaskOperation, n int) (bool, error) {
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
		if op == string(operation) {
			count++
		}
	}
	return count == n, nil
}

// brokerPodIDs returns the sorted broker ids (brokerId label) of the Running broker-only pods
// (isControllerNode=false) in the namespace.
func brokerPodIDs(kubectlOptions k8s.KubectlOptions) ([]string, error) {
	return podBrokerIDs(kubectlOptions, kafkaLabelSelectorBrokers)
}

// controllerPodIDs returns the sorted broker ids (brokerId label) of the Running controller-only pods
// (isControllerNode=true) in the namespace.
func controllerPodIDs(kubectlOptions k8s.KubectlOptions) ([]string, error) {
	return podBrokerIDs(kubectlOptions, kafkaLabelSelectorControllers)
}

// podBrokerIDs returns the sorted brokerId label values of the Running pods matching roleSelector. Asserting
// on the exact id set (rather than just a pod count) is what lets the scaling test catch a manifest that
// swaps ids instead of adding/removing a single node.
func podBrokerIDs(kubectlOptions k8s.KubectlOptions, roleSelector string) ([]string, error) {
	ids, err := getK8sResources(kubectlOptions,
		[]string{podsResource},
		v1beta1.KafkaCRLabelKey+"="+kafkaClusterName+","+roleSelector,
		"",
		"--field-selector=status.phase=Running",
		"-o", "jsonpath={range .items[*]}{.metadata.labels.brokerId}{'\\n'}{end}",
	)
	if err != nil {
		return nil, err
	}
	sort.Strings(ids)
	return ids, nil
}

// cruiseControlCapacityBrokerIDs returns the sorted broker ids present in Cruise Control's capacity.json
// (read from the CC ConfigMap). Asserting a scaled broker appears/disappears here proves the operator
// actually generated and rolled its per-broker capacity, independently of add_broker succeeding via Cruise
// Control's capacity estimation (AllowCapacityEstimation) - so a regression that stops writing per-broker
// capacity is caught rather than masked by estimation.
func cruiseControlCapacityBrokerIDs(kubectlOptions k8s.KubectlOptions) ([]string, error) {
	lines, err := getK8sResources(kubectlOptions,
		[]string{configMapsResource},
		v1beta1.KafkaCRLabelKey+"="+kafkaClusterName+",app=cruisecontrol",
		"",
		"-o", "jsonpath={range .items[*]}{.data.capacity\\.json}{end}",
	)
	if err != nil {
		return nil, err
	}
	raw := strings.TrimSpace(strings.Join(lines, "\n"))
	if raw == "" {
		return nil, nil
	}
	var parsed struct {
		BrokerCapacities []struct {
			BrokerID string `json:"brokerId"`
		} `json:"brokerCapacities"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(parsed.BrokerCapacities))
	for _, bc := range parsed.BrokerCapacities {
		ids = append(ids, bc.BrokerID)
	}
	sort.Strings(ids)
	return ids, nil
}

// runningCruiseControlPodHasCurrentCapacityHash reports whether the Running CC pod's template carries the hash
// of the CC ConfigMap's current capacity.json (the annotation koperator stamps in GeneratePodAnnotations).
// Reading the pod (not the Deployment template) shows what the currently-running CC actually started from, so
// with a Ready pod it is evidence the capacity roll landed rather than add_broker relying on capacity
// estimation. The hash is computed the same way the operator does (hex(sha256(capacity.json))). During a
// rollout there may briefly be two CC pods with different hashes; requiring an exact single-value match makes
// this true only once the roll has settled to the current-capacity pod.
func runningCruiseControlPodHasCurrentCapacityHash(kubectlOptions k8s.KubectlOptions) (bool, error) {
	ccSelector := v1beta1.KafkaCRLabelKey + "=" + kafkaClusterName + ",app=cruisecontrol"

	cmLines, err := getK8sResources(kubectlOptions,
		[]string{configMapsResource},
		ccSelector,
		"",
		"-o", "jsonpath={range .items[*]}{.data.capacity\\.json}{end}",
	)
	if err != nil {
		return false, err
	}
	// Do not trim the capacity.json - it is hashed byte-for-byte by the operator.
	capacityJSON := strings.Join(cmLines, "\n")
	if capacityJSON == "" {
		return false, nil
	}

	podLines, err := getK8sResources(kubectlOptions,
		[]string{podsResource},
		ccSelector,
		"",
		"--field-selector=status.phase=Running",
		"-o", "jsonpath={range .items[*]}{.metadata.annotations.cruiseControlCapacity\\.json}{'\\n'}{end}",
	)
	if err != nil {
		return false, err
	}
	runningHash := strings.TrimSpace(strings.Join(podLines, "\n"))
	sum := sha256.Sum256([]byte(capacityJSON))
	return runningHash != "" && runningHash == hex.EncodeToString(sum[:]), nil
}
