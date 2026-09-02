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

//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/gruntwork-io/terratest/modules/k8s"
	ginkgo "github.com/onsi/ginkgo/v2"
	gomega "github.com/onsi/gomega"

	kafkautils "github.com/banzaicloud/koperator/pkg/util/kafka"
)

const (
	multidiskRemovalTimeout      = 1800 * time.Second // this test can take long: rebalance must finish before removal starts
	multidiskRemovalPollInterval = 5 * time.Second
	brokerConfigTemplateFormat   = "%s-config-%d"
)

var (
	// removedMountPaths are the storageConfigs mountPaths dropped from the broker config group.
	removedMountPaths = []string{"/kafka-logs2", "/kafka-logs4"}
	// removedLogDirPath are the corresponding broker log.dirs entries (mountPath + "/kafka")
	// that must disappear from the broker ConfigMaps once the disks are removed.
	removedLogDirPath = []string{"/kafka-logs2/kafka", "/kafka-logs4/kafka"}
)

// testMultiDiskRemoval patches the KafkaCluster's default broker config group to drop two disks,
// waits for Cruise Control and PVC cleanup, then asserts broker ConfigMaps' log.dirs no longer
// contain the removed paths and brokers stay healthy.
func testMultiDiskRemoval() bool {
	return ginkgo.When("Multi-disk removal: remove multiple disks and assert log.dirs is updated", func() {
		var kubectlOptions k8s.KubectlOptions
		var err error

		ginkgo.It("Acquiring K8s config and context", func() {
			kubectlOptions, err = kubectlOptionsForCurrentContext()
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			kubectlOptions.Namespace = koperatorLocalHelmDescriptor.Namespace
		})

		ginkgo.It("Waiting for Cruise Control to be ready and idle before triggering removal", func() {
			// A freshly installed cluster runs an initial CruiseControl rebalance to spread replicas
			// across the brokers, and/or the operator may still be mid-rollout of the CC Deployment.
			// Triggering remove_disks while either is still in flight races the operation: Cruise
			// Control can reject the request (e.g. "Invalid log dirs provided for broker N") because
			// its monitoring state for the racing broker is stale or was just reset by a pod restart.
			// The controller re-validates a stalled retry against Cruise Control's current state and
			// skips resubmission until it agrees again (see staleRemoveDisksRetry in
			// cruisecontroloperation_controller.go), so this now self-heals within a retry cycle
			// instead of stalling indefinitely - but gating here avoids the race, and the wasted
			// retry, outright. See testBatchedBrokerRemoval for the same race on the broker-removal path.
			// Gate on a running cluster with no in-flight CC operation and a settled CC Deployment so
			// removal starts from a quiescent Cruise Control.
			ginkgo.By("Ensuring the KafkaCluster is running")
			err := waitForKafkaClusterWithPodStatusCheck(kubectlOptions, kafkaClusterName, kafkaClusterResourceReadinessTimeout)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Waiting until no Cruise Control operation is in flight (initial rebalance finished)")
			gomega.Eventually(context.Background(), func() (bool, error) {
				return hasNoInFlightCruiseControlOperation(kubectlOptions)
			}, multidiskRemovalTimeout, multidiskRemovalPollInterval).Should(gomega.BeTrue())

			ginkgo.By("Waiting until the Cruise Control Deployment is fully rolled out (single Ready replica)")
			gomega.Eventually(context.Background(), func() (bool, error) {
				return isCruiseControlDeploymentRolledOut(kubectlOptions)
			}, multidiskRemovalTimeout, multidiskRemovalPollInterval).Should(gomega.BeTrue())
		})

		ginkgo.It("Patching the KafkaCluster to trigger disk removal", func() {
			ginkgo.By("Patching spec.brokerConfigGroups.default.storageConfigs to drop two disks")
			// Patch only storageConfigs instead of re-applying a full manifest: a whole-manifest
			// apply can silently carry unrelated drift, whereas this changes exactly the field under test.
			err := removeKafkaClusterStorageConfigs(kubectlOptions, kafkaClusterName, "default", removedMountPaths...)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		})

		ginkgo.It("Waiting for disk removal and PVC cleanup", func() {
			ginkgo.By("Waiting until broker ConfigMaps' log.dirs no longer contain the removed path")
			gomega.Eventually(context.Background(), func() (bool, error) {
				return brokerConfigMapsLogDirsExcludePath(kubectlOptions, kafkaClusterName, removedLogDirPath)
			}, multidiskRemovalTimeout, multidiskRemovalPollInterval).Should(gomega.BeTrue())
		})

		ginkgo.It("Asserting broker ConfigMaps log.dirs do not contain removed path", func() {
			exclude, err := brokerConfigMapsLogDirsExcludePath(kubectlOptions, kafkaClusterName, removedLogDirPath)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(exclude).To(gomega.BeTrue(), "broker log.dirs must not contain removed path %s", removedLogDirPath)
		})

		ginkgo.It("Waiting for the Cruise Control disk rebalance to complete", func() {
			// The log.dirs ConfigMap update above only proves the operator dropped the removed path from
			// broker config, not that Cruise Control finished draining data off those disks. Gate on a
			// quiescent Cruise Control so the disk rebalance genuinely completes here rather than lingering
			// in flight (which would otherwise leave an in-progress operation for the next scenario).
			ginkgo.By("Waiting until no Cruise Control operation is in flight (disk rebalance finished)")
			gomega.Eventually(context.Background(), func() (bool, error) {
				return hasNoInFlightCruiseControlOperation(kubectlOptions)
			}, multidiskRemovalTimeout, multidiskRemovalPollInterval).Should(gomega.BeTrue())
		})

		ginkgo.It("Asserting Kafka brokers remain healthy", func() {
			// Disk removal rolls the broker pods, so their names change underneath us. Gate on
			// the operator's own ClusterRunning state and re-resolve the live pod set each poll
			// instead of `kubectl wait`-ing on a snapshot of pod names that get deleted mid-roll.
			// Use the scenario's own (longer) timeout budget, not the generic readiness one: a
			// rolling restart across all brokers with fresh PVC provisioning per broker can
			// legitimately take longer than kafkaClusterResourceReadinessTimeout under CI load.
			err := waitForKafkaClusterWithPodStatusCheck(kubectlOptions, kafkaClusterName, multidiskRemovalTimeout)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		})
	})
}

// brokerConfigMapsLogDirsIncludePath returns true if all broker ConfigMaps have log.dirs that contain the given path.
func brokerConfigMapsLogDirsIncludePath(kubectlOptions k8s.KubectlOptions, clusterName string, path string) (bool, error) {
	for _, brokerID := range []int{0, 1, 2} {
		configMapName := fmt.Sprintf(brokerConfigTemplateFormat, clusterName, brokerID)
		logDirs, err := getBrokerConfigMapLogDirs(kubectlOptions, configMapName, kubectlOptions.Namespace)
		if err != nil {
			return false, err
		}
		found := false
		for _, d := range logDirs {
			if d == path {
				found = true
				break
			}
		}
		if !found {
			return false, nil
		}
	}
	return true, nil
}

// brokerConfigMapsLogDirsExcludePath returns true if all broker ConfigMaps (for the given cluster)
// have log.dirs that do not contain the given path. Returns error if any required ConfigMap is missing
// or broker-config data cannot be read.
func brokerConfigMapsLogDirsExcludePath(kubectlOptions k8s.KubectlOptions, clusterName string, path []string) (bool, error) {
	// Brokers 0, 1, 2 from default sample
	for _, brokerID := range []int{0, 1, 2} {
		configMapName := fmt.Sprintf(brokerConfigTemplateFormat, clusterName, brokerID)
		logDirs, err := getBrokerConfigMapLogDirs(kubectlOptions, configMapName, kubectlOptions.Namespace)
		if err != nil {
			return false, err
		}
		for _, d := range logDirs {
			if slices.Contains(path, d) {
				return false, nil
			}
		}
	}
	return true, nil
}

// getBrokerConfigMapLogDirs returns the log.dirs value from the broker ConfigMap's broker-config data,
// parsed as a slice of paths (comma-separated in the config).
func getBrokerConfigMapLogDirs(kubectlOptions k8s.KubectlOptions, configMapName string, namespace string) ([]string, error) {
	args := []string{
		"get", "configmap", configMapName,
		"-n", namespace,
		"-o", fmt.Sprintf("jsonpath={.data.%s}", kafkautils.ConfigPropertyName),
	}
	// Fetch broker-config directly without terratest's logging: the ConfigMap holds the
	// entire broker configuration (a multi-line properties blob) and this runs on every
	// poll iteration for each broker, so logging the full value would flood the output.
	// We only need log.dirs, which we parse out of the properties content below.
	output, err := runKubectlSilent(kubectlOptions, args...)
	if err != nil {
		return nil, fmt.Errorf("getting configmap %s: %w", configMapName, err)
	}
	// Parse properties-style content for log.dirs=path1,path2 (broker-config is multi-line)
	prefix := "log.dirs="
	lines := strings.Split(output, "\n")
	for i := range lines {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, prefix) {
			value := strings.TrimPrefix(line, prefix)
			value = strings.TrimSpace(value)
			if value == "" {
				return []string{}, nil
			}
			var paths []string
			for _, p := range strings.Split(value, ",") {
				if q := strings.TrimSpace(p); q != "" {
					paths = append(paths, q)
				}
			}
			// Log only the extracted log.dirs (not the whole broker config).
			ginkgo.By(fmt.Sprintf("configmap %s log.dirs: %v", configMapName, paths))
			return paths, nil
		}
	}
	return nil, fmt.Errorf("log.dirs not found in configmap %s", configMapName)
}
