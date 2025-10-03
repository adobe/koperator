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
	"strings"

	"github.com/gruntwork-io/terratest/modules/k8s"
	ginkgo "github.com/onsi/ginkgo/v2"
	gomega "github.com/onsi/gomega"
)

func testInstallZookeeperCluster() bool {
	return ginkgo.When("Installing Zookeeper cluster (required for Zookeeper-based Kafka)", func() {
		var kubectlOptions k8s.KubectlOptions
		var err error

		ginkgo.It("Acquiring K8s config and context", func() {
			kubectlOptions, err = kubectlOptionsForCurrentContext()
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		})

		kubectlOptions.Namespace = zookeeperOperatorHelmDescriptor.Namespace
		requireCreatingZookeeperCluster(kubectlOptions)
	})
}

func testInstallKafkaCluster(kafkaClusterManifestPath string) bool { //nolint:unparam // Note: respecting Ginkgo testing interface by returning bool.
	// Determine cluster type based on manifest path for more descriptive test names
	var clusterDescription string
	switch {
	case strings.Contains(kafkaClusterManifestPath, "simplekafkacluster_ssl.yaml"):
		clusterDescription = "Installing Kafka cluster (Zookeeper-based, SSL enabled)"
	case strings.Contains(kafkaClusterManifestPath, "simplekafkacluster.yaml"):
		clusterDescription = "Installing Kafka cluster (Zookeeper-based, plaintext)"
	case strings.Contains(kafkaClusterManifestPath, "kraft/simplekafkacluster_kraft.yaml"):
		clusterDescription = "Installing Kafka cluster (KRaft mode, plaintext)"
	default:
		clusterDescription = "Installing Kafka cluster"
	}

	return ginkgo.When(clusterDescription, func() {
		var kubectlOptions k8s.KubectlOptions
		var err error

		ginkgo.It("Acquiring K8s config and context", func() {
			kubectlOptions, err = kubectlOptionsForCurrentContext()
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		})

		kubectlOptions.Namespace = koperatorLocalHelmDescriptor.Namespace
		requireCreatingKafkaCluster(kubectlOptions, kafkaClusterManifestPath)
	})
}
