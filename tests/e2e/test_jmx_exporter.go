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
	"fmt"
	"strings"

	coreV1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/gruntwork-io/terratest/modules/k8s"
	ginkgo "github.com/onsi/ginkgo/v2"
	gomega "github.com/onsi/gomega"
)

func testJmxExporter() bool { //nolint:unparam // Note: respecting Ginkgo testing interface by returning bool.
	return ginkgo.When("Deploying JMX Exporter rules", ginkgo.Ordered, func() {
		var kubectlOptions k8s.KubectlOptions
		var err error

		ginkgo.It("Acquiring K8s config and context", func() {
			kubectlOptions, err = kubectlOptionsForCurrentContext()
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		})

		kubectlOptions.Namespace = koperatorLocalHelmDescriptor.Namespace
		requireJmxMetrics(kubectlOptions)
	})
}

func requireJmxMetrics(kubectlOptions k8s.KubectlOptions) {
	var kRaftEnabled bool
	var err error

	ginkgo.It("Acquiring kRaftEnabled", func() {
		kRaftEnabled, err = isKRaftEnabledForKafkaCluster(kubectlOptions, kafkaClusterName)
		gomega.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to determine if KRaft mode is enabled")
	})

	ginkgo.It("All brokers should have kafka_server_ metrics available", func() {
		checkMetricExistsForBrokers(kubectlOptions, kafkaLabelSelectorAll, "kafka_server_", true)
	})

	ginkgo.It("When kraft mode is enabled, brokers/controllers should have kafka_server_raft_metrics_current_state_ metric available", func() {
		if kRaftEnabled {
			checkMetricExistsForBrokers(kubectlOptions, kafkaLabelSelectorBrokers, "kafka_server_raft_metrics_current_state_", true)
			checkMetricExistsForBrokers(kubectlOptions, kafkaLabelSelectorControllers, "kafka_server_raft_metrics_current_state_", true)
		} else {
			checkMetricExistsForBrokers(kubectlOptions, kafkaLabelSelectorBrokers, "kafka_server_raft_metrics_current_state_", false)
		}
	})
}

func checkMetricExistsForBrokers(kubectlOptions k8s.KubectlOptions, kafkaBrokerLabelSelector string, metricPrefix string, expectedMetricExists bool) {
	listOptions := metav1.ListOptions{
		LabelSelector: kafkaBrokerLabelSelector,
	}

	pods, err := k8s.ListPodsE(ginkgo.GinkgoT(), &kubectlOptions, listOptions)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to list pods")

	gomega.Expect(
		len(pods)).To(gomega.BeNumerically(">", 0),
		fmt.Sprintf("No Kafka pods found with the specified label selector: %s", kafkaBrokerLabelSelector),
	)

	for _, pod := range pods {
		actualMetricExists, err := metricExistsInPod(pod, kubectlOptions, metricPrefix)
		gomega.Expect(err).NotTo(gomega.HaveOccurred(), fmt.Sprintf("Failed to execute command inside pod %s", pod.Name))

		gomega.Expect(actualMetricExists).To(gomega.Equal(expectedMetricExists))
	}
}

func metricExistsInPod(pod coreV1.Pod, kubectlOptions k8s.KubectlOptions, metricPrefix string) (bool, error) {
	baseCommand := fmt.Sprintf("exec %s --container kafka -- sh -c", pod.Name)
	curlCommand := fmt.Sprintf("curl -s http://localhost:%s/metrics|grep ^%s|head -n 1", jmxExporterPort, metricPrefix)
	output, err := k8s.RunKubectlAndGetOutputE(ginkgo.GinkgoT(),
		&kubectlOptions,
		append(strings.Split(baseCommand, " "), curlCommand)...)

	if err != nil {
		return false, fmt.Errorf("failed to exec into pod '%s': %w", pod.Name, err)
	}
	fmt.Printf("Metric %s* exists?: '%t'\n", metricPrefix, strings.Contains(output, metricPrefix))
	return strings.Contains(output, metricPrefix), nil
}

func isKRaftEnabledForKafkaCluster(kubectlOptions k8s.KubectlOptions, kafkaClusterName string) (bool, error) {
	command := fmt.Sprintf("get %s %s -o jsonpath={.spec.kRaft}", kafkaKind, kafkaClusterName)
	kraftModeValue, err := k8s.RunKubectlAndGetOutputE(
		ginkgo.GinkgoT(),
		&kubectlOptions,
		strings.Split(command, " ")...)

	if err != nil {
		return false, fmt.Errorf("failed to get KafkaCluster '%s': %w", kafkaClusterName, err)
	}
	fmt.Printf("Is KRaft enabled?: %s\n", kraftModeValue)
	return strings.ToLower(kraftModeValue) == "true", nil
}
