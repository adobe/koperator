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
	"time"

	"github.com/gruntwork-io/terratest/modules/k8s"
	ginkgo "github.com/onsi/ginkgo/v2"
	gomega "github.com/onsi/gomega"
)

func testInstallKafkaClusterWithIstio(configPath string) bool {
	return ginkgo.When("Installing Kafka cluster (KRaft mode, Istio ingress)", ginkgo.Ordered, func() {
		var kubectlOptions k8s.KubectlOptions
		var err error

		ginkgo.It("Acquiring K8s config and context", func() {
			kubectlOptions, err = kubectlOptionsForCurrentContext()
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			kubectlOptions.Namespace = koperatorLocalHelmDescriptor.Namespace
		})

		ginkgo.It("Deploying KafkaCluster with Istio ingress", func() {
			err = k8s.KubectlApplyE(ginkgo.GinkgoT(), &kubectlOptions, configPath)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		})

		ginkgo.It("Waiting for Kafka cluster to be ready", func() {
			// Wait for Kafka cluster to be ready using the same method as other tests
			err := waitForKafkaClusterWithPodStatusCheck(kubectlOptions, kafkaClusterName, kafkaClusterCreateTimeout)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		})
	})
}

func testValidateIstioResources() bool {
	return ginkgo.When("Validating Istio resources", ginkgo.Ordered, func() {
		var kubectlOptions k8s.KubectlOptions
		var err error

		ginkgo.It("Acquiring K8s config and context", func() {
			kubectlOptions, err = kubectlOptionsForCurrentContext()
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			kubectlOptions.Namespace = koperatorLocalHelmDescriptor.Namespace
		})

		ginkgo.It("Validating Istio Gateway resources", func() {
			// Check for Istio Gateway resources
			gateways, err := k8s.RunKubectlAndGetOutputE(ginkgo.GinkgoT(), &kubectlOptions, "get", "gateways.networking.istio.io", "--all-namespaces", "-o", "name")
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(gateways).NotTo(gomega.BeEmpty())
			ginkgo.By(fmt.Sprintf("Found Istio Gateways: %s", gateways))
		})

		ginkgo.It("Validating Istio VirtualService resources", func() {
			// Check for Istio VirtualService resources
			virtualServices, err := k8s.RunKubectlAndGetOutputE(ginkgo.GinkgoT(), &kubectlOptions, "get", "virtualservices.networking.istio.io", "--all-namespaces", "-o", "name")
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(virtualServices).NotTo(gomega.BeEmpty())
			ginkgo.By(fmt.Sprintf("Found Istio VirtualServices: %s", virtualServices))
		})

		ginkgo.It("Validating Istio Gateway selector", func() {
			// Check that the Gateway resources use the correct selector for vanilla Istio
			gatewayYaml, err := k8s.RunKubectlAndGetOutputE(ginkgo.GinkgoT(), &kubectlOptions, "get", "gateways.networking.istio.io", "--all-namespaces", "-o", "yaml")
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(gatewayYaml).To(gomega.ContainSubstring("istio: ingressgateway"))
			ginkgo.By("Verified Gateway resources use vanilla Istio selector")
		})

		ginkgo.It("Validating Gateway selector format", func() {
			// More specific check for the exact selector format
			gatewayJson, err := k8s.RunKubectlAndGetOutputE(ginkgo.GinkgoT(), &kubectlOptions, "get", "gateways.networking.istio.io", "--all-namespaces", "-o", "jsonpath={.items[*].spec.selector}")
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(gatewayJson).To(gomega.ContainSubstring("istio"))
			gomega.Expect(gatewayJson).To(gomega.ContainSubstring("ingressgateway"))
			ginkgo.By("Verified Gateway selector format is correct")
		})

		ginkgo.It("Validating standard Istio ingress gateway is available", func() {
			// Check for standard istio ingress gateway deployments (not custom ones)
			deployments, err := k8s.RunKubectlAndGetOutputE(ginkgo.GinkgoT(), &kubectlOptions, "get", "deployments", "--all-namespaces", "-l", "app=istio-ingressgateway", "-o", "name")
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(deployments).NotTo(gomega.BeEmpty())
			ginkgo.By(fmt.Sprintf("Found standard istio ingress gateway deployments: %s", deployments))
		})

		ginkgo.It("Validating standard Istio ingress gateway service", func() {
			// Check for standard istio ingress gateway services
			services, err := k8s.RunKubectlAndGetOutputE(ginkgo.GinkgoT(), &kubectlOptions, "get", "services", "--all-namespaces", "-l", "app=istio-ingressgateway", "-o", "name")
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(services).NotTo(gomega.BeEmpty())
			ginkgo.By(fmt.Sprintf("Found standard istio ingress gateway services: %s", services))
		})

		ginkgo.It("Validating standard Istio ingress gateway pods are running", func() {
			// Check that standard istio ingress gateway pods are running
			pods, err := k8s.RunKubectlAndGetOutputE(ginkgo.GinkgoT(), &kubectlOptions, "get", "pods", "--all-namespaces", "-l", "app=istio-ingressgateway", "-o", "jsonpath={.items[*].status.phase}")
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pods).To(gomega.ContainSubstring("Running"))
			ginkgo.By(fmt.Sprintf("Standard Istio ingress gateway pods status: %s", pods))
		})

		ginkgo.It("Validating external listener statuses are populated", func() {
			// Check that external listener statuses are populated in the KafkaCluster
			externalAddresses, err := getExternalListenerAddresses(kubectlOptions, "", kafkaClusterName)
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
			gomega.Expect(externalAddresses).ShouldNot(gomega.BeEmpty())
			ginkgo.By(fmt.Sprintf("Found external listener addresses: %v", externalAddresses))

			// Verify we have the expected number of external addresses (any-broker + individual brokers)
			// For a 3-broker cluster, we should have 4 addresses: any-broker + broker-0, broker-1, broker-2
			gomega.Expect(len(externalAddresses)).Should(gomega.BeNumerically(">=", 4))
		})
	})
}

func testProduceConsumeWithIstio() bool {
	return ginkgo.When("Testing produce/consume with Istio ingress", ginkgo.Ordered, func() {
		var kubectlOptions k8s.KubectlOptions
		var err error

		ginkgo.It("Acquiring K8s config and context", func() {
			kubectlOptions, err = kubectlOptionsForCurrentContext()
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			kubectlOptions.Namespace = koperatorLocalHelmDescriptor.Namespace
		})

		ginkgo.It("Deploying kcat pod for external testing", func() {
			requireDeployingKcatPod(kubectlOptions, kcatName, "")
		})

		ginkgo.It("Deploying Kafka topic for external testing", func() {
			requireDeployingKafkaTopic(kubectlOptions, testExternalTopicName)
		})

		ginkgo.It("Testing external produce/consume through Istio ingress", func() {
			requireExternalProducingConsumingMessageViaKcat(kubectlOptions, kcatName, testExternalTopicName, "")
		})

		ginkgo.It("Cleaning up external test resources", func() {
			requireDeleteKafkaTopic(kubectlOptions, testExternalTopicName)
			requireDeleteKcatPod(kubectlOptions, kcatName)
		})
	})
}

func testUninstallKafkaClusterWithIstio() bool {
	return ginkgo.When("Uninstalling Kafka cluster (KRaft mode, Istio ingress)", ginkgo.Ordered, func() {
		var kubectlOptions k8s.KubectlOptions
		var err error

		ginkgo.It("Acquiring K8s config and context", func() {
			kubectlOptions, err = kubectlOptionsForCurrentContext()
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			kubectlOptions.Namespace = koperatorLocalHelmDescriptor.Namespace
		})

		ginkgo.It("Uninstalling KafkaCluster with Istio ingress", func() {
			// Remove the Kafka cluster
			err = k8s.RunKubectlE(ginkgo.GinkgoT(), &kubectlOptions, "delete", "-f", "../../config/samples/kraft/kafkacluster-kraft-with-istio.yaml", "--ignore-not-found=true")
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		})

		ginkgo.It("Verifying Kafka-specific Istio resources are cleaned up", func() {
			// Wait a bit for cleanup
			time.Sleep(10 * time.Second)

			// Check that Kafka-specific Gateway resources are removed
			gateways, err := k8s.RunKubectlAndGetOutputE(ginkgo.GinkgoT(), &kubectlOptions, "get", "gateways.networking.istio.io", "--all-namespaces", "-l", "app=istioingress", "-o", "name")
			if err == nil {
				gomega.Expect(gateways).To(gomega.BeEmpty())
			}

			// Check that Kafka-specific VirtualService resources are removed
			virtualServices, err := k8s.RunKubectlAndGetOutputE(ginkgo.GinkgoT(), &kubectlOptions, "get", "virtualservices.networking.istio.io", "--all-namespaces", "-l", "app=istioingress", "-o", "name")
			if err == nil {
				gomega.Expect(virtualServices).To(gomega.BeEmpty())
			}

			// Note: Standard Istio ingress gateway should still be running
			// as it's not managed by the Kafka operator
			ginkgo.By("Kafka-specific Istio resources cleaned up successfully")
		})
	})
}
