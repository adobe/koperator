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
	"strings"
	"time"

	"github.com/gruntwork-io/terratest/modules/k8s"
	ginkgo "github.com/onsi/ginkgo/v2"
	gomega "github.com/onsi/gomega"

	"github.com/banzaicloud/koperator/api/v1beta1"
	kafkautils "github.com/banzaicloud/koperator/pkg/util/kafka"
)

const (
	multidiskRemovalTimeout      = 900 * time.Second // CC disk removal can take long
	multidiskRemovalPollInterval = 15 * time.Second
	removedLogDirPath            = "/kafka-logs-extra/kafka"
	brokerConfigTemplateFormat   = "%s-config-%d"
	// Paths for add-then-remove-middle and disk-swap scenarios (2-disk: /kafka-logs, /kafka-logs2; 3-disk adds /kafka-logs3)
	middleDiskLogDirPath = "/kafka-logs2/kafka"
	thirdDiskLogDirPath  = "/kafka-logs3/kafka"
)

// testMultiDiskRemoval applies the single-disk manifest to remove the second disk from the cluster,
// waits for Cruise Control and PVC cleanup, then asserts broker ConfigMaps' log.dirs no longer
// contain the removed path and brokers stay healthy.
func testMultiDiskRemoval() bool {
	return ginkgo.When("Multi-disk removal: remove one disk and assert log.dirs is updated", func() {
		var kubectlOptions k8s.KubectlOptions
		var err error

		ginkgo.It("Acquiring K8s config and context", func() {
			kubectlOptions, err = kubectlOptionsForCurrentContext()
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			kubectlOptions.Namespace = koperatorLocalHelmDescriptor.Namespace
		})

		ginkgo.It("Applying single-disk manifest to trigger disk removal", func() {
			ginkgo.By("Patching KafkaCluster to remove second disk (storageConfigs -> single entry)")
			applyK8sResourceManifest(kubectlOptions, "../../config/samples/simplekafkacluster.yaml")
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

// testAddThenRemoveMiddleDisk: create cluster with 2 disks, add 3rd disk, then remove the middle one.
// Asserts final log.dirs contains only disk1 and disk3; middle path is dropped after removal completes.
func testAddThenRemoveMiddleDisk() bool {
	return ginkgo.When("Add then remove middle disk: 2 disks -> add 3rd -> remove middle", func() {
		var kubectlOptions k8s.KubectlOptions
		var err error

		ginkgo.It("Acquiring K8s config and context", func() {
			kubectlOptions, err = kubectlOptionsForCurrentContext()
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			kubectlOptions.Namespace = koperatorLocalHelmDescriptor.Namespace
		})

		ginkgo.It("Applying 3-disk manifest to add the third disk", func() {
			ginkgo.By("Patching KafkaCluster to add third disk (storageConfigs: kafka-logs, kafka-logs2, kafka-logs3)")
			applyK8sResourceManifest(kubectlOptions, "../../config/samples/simplekafkacluster_threedisk.yaml")
		})

		ginkgo.It("Applying manifest without middle disk to remove the second disk", func() {
			ginkgo.By("Patching KafkaCluster to remove middle disk (storageConfigs: kafka-logs, kafka-logs3)")
			applyK8sResourceManifest(kubectlOptions, "../../config/samples/simplekafkacluster_twodisk_no_middle.yaml")
		})

		ginkgo.It("Waiting for disk removal and log.dirs update", func() {
			ginkgo.By("Waiting until broker ConfigMaps' log.dirs no longer contain the removed middle path")
			gomega.Eventually(context.Background(), func() (bool, error) {
				return brokerConfigMapsLogDirsExcludePath(kubectlOptions, kafkaClusterName, middleDiskLogDirPath)
			}, multidiskRemovalTimeout, multidiskRemovalPollInterval).Should(gomega.BeTrue())
		})

		ginkgo.It("Asserting broker ConfigMaps log.dirs do not contain removed middle path", func() {
			exclude, err := brokerConfigMapsLogDirsExcludePath(kubectlOptions, kafkaClusterName, middleDiskLogDirPath)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(exclude).To(gomega.BeTrue(), "broker log.dirs must not contain removed path %s", middleDiskLogDirPath)
		})

		ginkgo.It("Asserting Kafka brokers remain healthy", func() {
			err := waitK8sResourceCondition(kubectlOptions, "pod", "condition=Ready", defaultPodReadinessWaitTime,
				v1beta1.KafkaCRLabelKey+"="+kafkaClusterName+",app=kafka", "")
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		})
	})
}

// testDiskSwap: create cluster with 2 disks, then in one patch add 3rd disk and remove 2nd (replace disk2 with disk3).
// Asserts final log.dirs contains disk1 and disk3; disk2 path is absent once removal completes.
func testDiskSwap() bool {
	return ginkgo.When("Disk swap: replace disk2 with disk3 in a single manifest patch", func() {
		var kubectlOptions k8s.KubectlOptions
		var err error

		ginkgo.It("Acquiring K8s config and context", func() {
			kubectlOptions, err = kubectlOptionsForCurrentContext()
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			kubectlOptions.Namespace = koperatorLocalHelmDescriptor.Namespace
		})

		ginkgo.It("Applying disk-swap manifest (add disk3, remove disk2)", func() {
			ginkgo.By("Patching KafkaCluster to replace second disk with third (storageConfigs: kafka-logs, kafka-logs3)")
			applyK8sResourceManifest(kubectlOptions, "../../config/samples/simplekafkacluster_twodisk_no_middle.yaml")
		})

		ginkgo.It("Waiting for disk removal and log.dirs update", func() {
			ginkgo.By("Waiting until broker ConfigMaps' log.dirs no longer contain the removed path and include disk3")
			gomega.Eventually(context.Background(), func() (bool, error) {
				excludeMiddle, err := brokerConfigMapsLogDirsExcludePath(kubectlOptions, kafkaClusterName, middleDiskLogDirPath)
				if err != nil || !excludeMiddle {
					return false, err
				}
				return brokerConfigMapsLogDirsIncludePath(kubectlOptions, kafkaClusterName, thirdDiskLogDirPath)
			}, multidiskRemovalTimeout, multidiskRemovalPollInterval).Should(gomega.BeTrue())
		})

		ginkgo.It("Asserting broker ConfigMaps log.dirs match expected (disk1 and disk3, no disk2)", func() {
			exclude, err := brokerConfigMapsLogDirsExcludePath(kubectlOptions, kafkaClusterName, middleDiskLogDirPath)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(exclude).To(gomega.BeTrue(), "broker log.dirs must not contain removed path %s", middleDiskLogDirPath)
			include, err := brokerConfigMapsLogDirsIncludePath(kubectlOptions, kafkaClusterName, thirdDiskLogDirPath)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(include).To(gomega.BeTrue(), "broker log.dirs must contain path %s", thirdDiskLogDirPath)
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
func brokerConfigMapsLogDirsExcludePath(kubectlOptions k8s.KubectlOptions, clusterName string, path string) (bool, error) {
	// Brokers 0, 1, 2 from default sample
	for _, brokerID := range []int{0, 1, 2} {
		configMapName := fmt.Sprintf(brokerConfigTemplateFormat, clusterName, brokerID)
		logDirs, err := getBrokerConfigMapLogDirs(kubectlOptions, configMapName, kubectlOptions.Namespace)
		if err != nil {
			return false, err
		}
		for _, d := range logDirs {
			if d == path {
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
	output, err := k8s.RunKubectlAndGetOutputE(ginkgo.GinkgoT(), &kubectlOptions, args...)
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
			return paths, nil
		}
	}
	return nil, fmt.Errorf("log.dirs not found in configmap %s", configMapName)
}
