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
	"fmt"
	"strings"
	"time"

	"github.com/gruntwork-io/terratest/modules/k8s"
	ginkgo "github.com/onsi/ginkgo/v2"
	gomega "github.com/onsi/gomega"

	kafkautils "github.com/banzaicloud/koperator/pkg/util/kafka"
)

const (
	// This scenario waits for both a disk removal (Cruise Control rebalance + removal) and a
	// rolling restart, so it can take long — reuse the multidisk timeout budget.
	configChangeDiskRemovalTimeout      = 1800 * time.Second
	configChangeDiskRemovalPollInterval = 5 * time.Second

	// A read-only broker property changed together with the disk removal. Read-only properties
	// require a rolling restart, so its presence in the broker ConfigMap plus the cluster
	// returning to ClusterRunning proves the config change was reconciled (not just written).
	// Non-default value (Kafka default for log.retention.hours is 168).
	configChangeProperty = "log.retention.hours=170"
)

// configChangeRemovedMountPath is the storageConfigs mount path dropped by this test (keeping only
// /kafka-logs1). This test chains after testMultiDiskRemoval, which leaves the cluster with
// /kafka-logs1 and /kafka-logs3.
const configChangeRemovedMountPath = "/kafka-logs3"

// configChangeRemovedLogDirPath is the broker log.dirs entry (mount path + "/kafka") that must
// disappear once configChangeRemovedMountPath is removed.
var configChangeRemovedLogDirPath = []string{"/kafka-logs3/kafka"}

// testConfigChangeWithDiskRemoval makes a single targeted patch that BOTH changes a read-only broker
// config property (forcing a rolling restart) AND removes a disk, so the operator observes both in the
// same reconcile. This is the scenario that previously deadlocked: reconcileKafkaPvc blocked the whole
// reconcile on the pending disk removal, so reconcileKafkaPod never ran to restart the brokers for the
// config change, and the cluster was stuck in ClusterRollingUpgrading indefinitely. The test asserts
// the cluster reconciles correctly: the config change propagates, the disk is removed, Cruise Control
// goes quiescent, and the cluster returns to ClusterRunning within the timeout (a deadlock would make
// the final wait time out).
func testConfigChangeWithDiskRemoval() bool {
	return ginkgo.When("Config change + disk removal in one patch: cluster reconciles correctly", func() {
		var kubectlOptions k8s.KubectlOptions
		var err error

		ginkgo.It("Acquiring K8s config and context", func() {
			kubectlOptions, err = kubectlOptionsForCurrentContext()
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			kubectlOptions.Namespace = koperatorLocalHelmDescriptor.Namespace
		})

		ginkgo.It("Patching the KafkaCluster to change broker config AND remove a disk", func() {
			ginkgo.By(fmt.Sprintf("Patching spec.readOnlyConfig (%s) and dropping storageConfig %s in one merge patch",
				configChangeProperty, configChangeRemovedMountPath))
			// Change both fields in a single targeted patch (not a whole-manifest re-apply, which can
			// silently carry unrelated drift and re-trigger a Cruise Control rollout) so the operator
			// sees the restart-forcing config change and the disk removal in the same reconcile.
			err := patchKafkaClusterReadOnlyConfigAndRemoveDisks(kubectlOptions, kafkaClusterName, "default", configChangeProperty, configChangeRemovedMountPath)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		})

		ginkgo.It("Waiting for the broker config change to propagate to broker ConfigMaps", func() {
			ginkgo.By(fmt.Sprintf("Waiting until all broker ConfigMaps contain %q", configChangeProperty))
			gomega.Eventually(context.Background(), func() (bool, error) {
				return brokerConfigMapsContainProperty(kubectlOptions, kafkaClusterName, configChangeProperty)
			}, configChangeDiskRemovalTimeout, configChangeDiskRemovalPollInterval).Should(gomega.BeTrue())
		})

		ginkgo.It("Waiting for disk removal and PVC cleanup (log.dirs excludes removed path)", func() {
			ginkgo.By("Waiting until broker ConfigMaps' log.dirs no longer contain the removed path")
			gomega.Eventually(context.Background(), func() (bool, error) {
				return brokerConfigMapsLogDirsExcludePath(kubectlOptions, kafkaClusterName, configChangeRemovedLogDirPath)
			}, configChangeDiskRemovalTimeout, configChangeDiskRemovalPollInterval).Should(gomega.BeTrue())
		})

		ginkgo.It("Waiting for the Cruise Control disk removal to complete", func() {
			ginkgo.By("Waiting until no Cruise Control operation is in flight")
			gomega.Eventually(context.Background(), func() (bool, error) {
				return hasNoInFlightCruiseControlOperation(kubectlOptions)
			}, configChangeDiskRemovalTimeout, configChangeDiskRemovalPollInterval).Should(gomega.BeTrue())
		})

		ginkgo.It("Asserting the cluster reconciles to ClusterRunning (no deadlock)", func() {
			// The core regression assertion: a config change while a disk removal is pending must not
			// wedge the reconcile. If the deadlock regressed, the cluster would stay in
			// ClusterRollingUpgrading and this wait would time out.
			// Use the scenario's own (longer) timeout budget, not the generic readiness one: the
			// preceding steps already use configChangeDiskRemovalTimeout because this scenario can
			// take long, and the final rolling restart this gates on is no faster.
			err := waitForKafkaClusterWithPodStatusCheck(kubectlOptions, kafkaClusterName, configChangeDiskRemovalTimeout)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		})

		ginkgo.It("Asserting broker ConfigMaps still carry the config change and not the removed disk", func() {
			hasProperty, err := brokerConfigMapsContainProperty(kubectlOptions, kafkaClusterName, configChangeProperty)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(hasProperty).To(gomega.BeTrue(), "broker config must contain %q after reconcile", configChangeProperty)

			exclude, err := brokerConfigMapsLogDirsExcludePath(kubectlOptions, kafkaClusterName, configChangeRemovedLogDirPath)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(exclude).To(gomega.BeTrue(), "broker log.dirs must not contain removed path %s", configChangeRemovedLogDirPath)
		})
	})
}

// brokerConfigMapsContainProperty returns true if every broker ConfigMap's broker-config contains
// the given "key=value" property line.
func brokerConfigMapsContainProperty(kubectlOptions k8s.KubectlOptions, clusterName string, property string) (bool, error) {
	for _, brokerID := range []int{0, 1, 2} {
		configMapName := fmt.Sprintf(brokerConfigTemplateFormat, clusterName, brokerID)
		contains, err := brokerConfigMapContainsProperty(kubectlOptions, configMapName, kubectlOptions.Namespace, property)
		if err != nil {
			return false, err
		}
		if !contains {
			return false, nil
		}
	}
	return true, nil
}

// brokerConfigMapContainsProperty returns true if the broker ConfigMap's broker-config data contains
// the given "key=value" property line (whitespace-trimmed, exact match).
func brokerConfigMapContainsProperty(kubectlOptions k8s.KubectlOptions, configMapName string, namespace string, property string) (bool, error) {
	args := []string{
		"get", "configmap", configMapName,
		"-n", namespace,
		"-o", fmt.Sprintf("jsonpath={.data.%s}", kafkautils.ConfigPropertyName),
	}
	// Fetch broker-config silently: it is the entire multi-line broker configuration and this runs on
	// every poll iteration for each broker, so logging the full value would flood the output.
	output, err := runKubectlSilent(kubectlOptions, args...)
	if err != nil {
		return false, fmt.Errorf("getting configmap %s: %w", configMapName, err)
	}
	want := strings.TrimSpace(property)
	for _, line := range strings.Split(output, "\n") {
		if strings.TrimSpace(line) == want {
			return true, nil
		}
	}
	return false, nil
}
