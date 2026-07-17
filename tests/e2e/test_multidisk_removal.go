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

	"github.com/banzaicloud/koperator/api/v1beta1"
	kafkautils "github.com/banzaicloud/koperator/pkg/util/kafka"
)

const (
	multidiskRemovalTimeout      = 1800 * time.Second // this test can take long: rebalance must finish before removal starts
	multidiskRemovalPollInterval = 5 * time.Second
	brokerConfigTemplateFormat   = "%s-config-%d"
)

var (
	removedLogDirPath = []string{"/kafka-logs2/kafka", "/kafka-logs4/kafka"}
)

// testMultiDiskRemoval applies the single-disk manifest to remove the second disk from the cluster,
// waits for Cruise Control and PVC cleanup, then asserts broker ConfigMaps' log.dirs no longer
// contain the removed path and brokers stay healthy.
func testMultiDiskRemoval() bool {
	return ginkgo.When("Multi-disk removal: remove multiple disks and assert log.dirs is updated", func() {
		var kubectlOptions k8s.KubectlOptions
		var err error

		ginkgo.It("Acquiring K8s config and context", func() {
			kubectlOptions, err = kubectlOptionsForCurrentContext()
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			kubectlOptions.Namespace = koperatorLocalHelmDescriptor.Namespace
		})

		ginkgo.It("Applying two-disk manifest to trigger disk removal", func() {
			ginkgo.By("Patching KafkaCluster to remove two disks (storageConfigs -> two entries)")
			applyK8sResourceManifest(kubectlOptions, "../../config/samples/simplekafkacluster_2disk.yaml")
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

		ginkgo.It("Asserting Kafka brokers remain healthy", func() {
			err := waitK8sResourceCondition(kubectlOptions, "pod", "condition=Ready", defaultPodReadinessWaitTime,
				v1beta1.KafkaCRLabelKey+"="+kafkaClusterName+",app=kafka", "")
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
