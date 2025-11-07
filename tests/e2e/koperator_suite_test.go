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
	"testing"

	"github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestKoperator(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail) // Note: Ginkgo - Gomega connector.
	ginkgo.RunSpecs(t, "Koperator end to end test suite")
}

var _ = ginkgo.BeforeSuite(func() {
	// Setup reduced logging for terratest operations
	setupReducedLogging()

	ginkgo.By("Acquiring K8s cluster")
	var kubeconfigPath string
	var kubecontextName string

	ginkgo.By("Acquiring K8s config and context", func() {
		var err error
		kubeconfigPath, kubecontextName, err = currentEnvK8sContext()
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	})

	ginkgo.By("Listing kube-system pods", func() {
		pods := k8s.ListPods(
			ginkgo.GinkgoT(),
			k8s.NewKubectlOptions(kubecontextName, kubeconfigPath, "kube-system"),
			v1.ListOptions{},
		)

		gomega.Expect(len(pods)).To(gomega.Not(gomega.BeZero()))
	})
})

var _ = ginkgo.When("Testing e2e test altogether", ginkgo.Ordered, func() {
	var snapshottedInfo = &clusterSnapshot{}
	snapshotCluster(snapshottedInfo)
	testInstall()
	testInstallZookeeperCluster()
	testInstallNoIngressKafkaCluster("Installing Kafka cluster (Zookeeper-based, plaintext, no ingress)", "../../config/samples/simplekafkacluster.yaml")
	testProduceConsumeInternal()
	testJmxExporter()
	testUninstallKafkaCluster()
	testInstallNoIngressKafkaCluster("Installing Kafka cluster (Zookeeper-based, SSL enabled, no ingress)", "../../config/samples/simplekafkacluster_ssl.yaml")
	testProduceConsumeInternalSSL(defaultTLSSecretName)
	testJmxExporter()
	testUninstallKafkaCluster()
	testUninstallZookeeperCluster()
	testInstallNoIngressKafkaCluster("Installing Kafka cluster (KRaft mode, plaintext, no ingress)", "../../config/samples/kraft/simplekafkacluster_kraft.yaml")
	testProduceConsumeInternal()
	testJmxExporter()
	testUninstallKafkaCluster()
	testInstallZookeeperCluster()
	testInstallEnvoyKafkaCluster("Installing Kafka cluster (Zookeeper-based, Envoy ingress)", "../../config/samples/simplekafkacluster_with_envoy.yaml")
	testProduceConsumeInternal()
	testJmxExporter()
	testUninstallKafkaCluster()
	testUninstallZookeeperCluster()
	testInstallEnvoyKafkaCluster("Installing Kafka cluster (KRaft mode, Envoy ingress)", "../../config/samples/kraft/simplekafkacluster_kraft_with_envoy.yaml")
	testProduceConsumeInternal()
	testJmxExporter()
	testUninstallKafkaCluster()
	testInstallZookeeperCluster()
	testInstallEnvoyGatewayKafkaCluster("Installing Kafka cluster (Zookeeper-based, Envoy Gateway ingress)", "../../config/samples/simplekafkacluster_with_envoygateway.yaml")
	testProduceConsumeInternal()
	testJmxExporter()
	testUninstallEnvoyGatewayKafkaCluster("../../config/samples/simplekafkacluster_with_envoygateway.yaml")
	testUninstallZookeeperCluster()
	testInstallEnvoyGatewayKafkaCluster("Installing Kafka cluster (KRaft mode, Envoy Gateway ingress)", "../../config/samples/kraft/simplekafkacluster_kraft_with_envoygateway.yaml")
	testProduceConsumeInternal()
	testJmxExporter()
	testUninstallEnvoyGatewayKafkaCluster("../../config/samples/kraft/simplekafkacluster_kraft_with_envoygateway.yaml")
	testUninstall()
	snapshotClusterAndCompare(snapshottedInfo)
})
