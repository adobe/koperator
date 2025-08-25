// Copyright © 2023 Cisco Systems, Inc. and/or its affiliates
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
	"encoding/json"
	"fmt"
	"strings"

	"github.com/banzaicloud/koperator/api/v1beta1"
	coreV1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/gruntwork-io/terratest/modules/k8s"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func testJmxExporter() bool { //nolint:unparam // Note: respecting Ginkgo testing interface by returning bool.
	return When("Deploying JMX Exporter rules", Ordered, func() {
		var kubectlOptions k8s.KubectlOptions
		var err error

		It("Acquiring K8s config and context", func() {
			kubectlOptions, err = kubectlOptionsForCurrentContext()
			Expect(err).NotTo(HaveOccurred())
		})

		kubectlOptions.Namespace = koperatorLocalHelmDescriptor.Namespace
		It("Checking JMX Exporter metrics", func() {
			requireJmxMetrics(kubectlOptions)
		})

	})
}

func requireJmxMetrics(kubectlOptions k8s.KubectlOptions) {


	kRaftEnabled, err := isKRaftEnabled(kubectlOptions, kafkaClusterName)
	Expect(err).NotTo(HaveOccurred(), "Failed to determine if KRaft mode is enabled")

	// should always have kafka_server_ metrics available for zk/kraft based clusters
	checkMetricExistsForBrokers(kubectlOptions, kafkaLabelSelectorAll, "kafka_server_", true)

	// should only have kafka_server_raft_metrics_current_state_ available for kraft based cluster
	checkMetricExistsForBrokers(kubectlOptions, kafkaLabelSelectorBrokers, "kafka_server_raft_metrics_current_state_", kRaftEnabled)

	if kRaftEnabled {
		// only check controller pods if KRaft is enabled
		checkMetricExistsForBrokers(kubectlOptions, kafkaLabelSelectorControllers, "kafka_server_raft_metrics_current_state_", true)
	}
}

func checkMetricExistsForBrokers(kubectlOptions k8s.KubectlOptions, kafkaBrokerLabelSelector string, metricPrefix string, expectMetricExists bool) {
	listOptions := metav1.ListOptions{
		LabelSelector: kafkaBrokerLabelSelector,
	}

	pods, err := k8s.ListPodsE(GinkgoT(), &kubectlOptions, listOptions)
	Expect(err).NotTo(HaveOccurred(), "Failed to list pods")

	Expect(
		len(pods)).To(BeNumerically(">", 0),
		fmt.Sprintf("No Kafka pods found with the specified label selector: %s", kafkaBrokerLabelSelector),
	)

	for _, pod := range pods {
		output, err := podExecJMXExporterMetrics(pod, kubectlOptions)
		Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to execute command inside pod %s", pod.Name))

		if expectMetricExists {
			Expect(strings.Contains(output, metricPrefix)).To(BeTrue())
		} else {
			Expect(strings.Contains(output, metricPrefix)).To(BeFalse())
		}
	}
}

func podExecJMXExporterMetrics(pod coreV1.Pod, kubectlOptions k8s.KubectlOptions) (string, error) {
	return k8s.RunKubectlAndGetOutputE(GinkgoT(),
		&kubectlOptions,
		"exec",
		pod.Name,
		"--container", "kafka",
		"--",
		"curl",
		fmt.Sprintf("http://localhost:%s/metrics", jmxExporterPort))
}

func isKRaftEnabled(kubectlOptions k8s.KubectlOptions, kafkaClusterName string) (bool, error) {
	jsonOutput, err := k8s.RunKubectlAndGetOutputE(GinkgoT(),
		&kubectlOptions,
		"get",
		kafkaKind,
		kafkaClusterName,
		"-o",
		"json")

	if err != nil {
		return false, fmt.Errorf("failed to get KafkaCluster '%s': %w", kafkaClusterName, err)
	}

	var kafkaCluster v1beta1.KafkaCluster
	if err := json.Unmarshal([]byte(jsonOutput), &kafkaCluster); err != nil {
		return false, fmt.Errorf("failed to unmarshal JSON output into KafkaCluster object: %w", err)
	}

	return kafkaCluster.Spec.KRaftMode, nil
}
