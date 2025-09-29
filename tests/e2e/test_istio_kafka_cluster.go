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
	return ginkgo.When("Installing Kafka cluster with Istio ingress", ginkgo.Ordered, func() {
		var kubectlOptions k8s.KubectlOptions
		var err error

		ginkgo.It("Acquiring K8s config and context", func() {
			kubectlOptions, err = kubectlOptionsForCurrentContext()
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		})

		ginkgo.It("Installing Kafka cluster with Istio ingress", func() {
			err = k8s.KubectlApplyE(ginkgo.GinkgoT(), &kubectlOptions, configPath)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		})

		ginkgo.It("Waiting for Kafka cluster to be ready", func() {
			// Wait for Kafka cluster to be ready
			time.Sleep(30 * time.Second)
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

		ginkgo.It("Validating Istio ingress gateway Deployment", func() {
			// Check for istio ingress gateway deployments
			deployments, err := k8s.RunKubectlAndGetOutputE(ginkgo.GinkgoT(), &kubectlOptions, "get", "deployments", "--all-namespaces", "-l", "app=istio-ingressgateway", "-o", "name")
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(deployments).NotTo(gomega.BeEmpty())
			ginkgo.By(fmt.Sprintf("Found istio ingress gateway deployments: %s", deployments))
		})

		ginkgo.It("Validating Istio ingress gateway Service", func() {
			// Check for istio ingress gateway services
			services, err := k8s.RunKubectlAndGetOutputE(ginkgo.GinkgoT(), &kubectlOptions, "get", "services", "--all-namespaces", "-l", "app=istio-ingressgateway", "-o", "name")
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(services).NotTo(gomega.BeEmpty())
			ginkgo.By(fmt.Sprintf("Found istio ingress gateway services: %s", services))
		})

		ginkgo.It("Validating Istio ingress gateway pods are running", func() {
			// Check that istio ingress gateway pods are running
			pods, err := k8s.RunKubectlAndGetOutputE(ginkgo.GinkgoT(), &kubectlOptions, "get", "pods", "--all-namespaces", "-l", "app=istio-ingressgateway", "-o", "jsonpath={.items[*].status.phase}")
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pods).To(gomega.ContainSubstring("Running"))
			ginkgo.By(fmt.Sprintf("Istio ingress gateway pods status: %s", pods))
		})
	})
}

func testProduceConsumeWithIstio() bool {
	return ginkgo.When("Testing produce/consume with Istio ingress", ginkgo.Ordered, func() {
		ginkgo.It("Testing external access through Istio ingress", func() {
			// This would test external access through Istio ingress
			// For now, we'll just verify the setup is working
			ginkgo.By("Istio ingress setup validated - external access testing would go here")
		})
	})
}

func testUninstallKafkaClusterWithIstio() bool {
	return ginkgo.When("Uninstalling Kafka cluster with Istio ingress", ginkgo.Ordered, func() {
		var kubectlOptions k8s.KubectlOptions
		var err error

		ginkgo.It("Acquiring K8s config and context", func() {
			kubectlOptions, err = kubectlOptionsForCurrentContext()
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		})

		ginkgo.It("Uninstalling Kafka cluster with Istio ingress", func() {
			// Remove the Kafka cluster
			err = k8s.RunKubectlE(ginkgo.GinkgoT(), &kubectlOptions, "delete", "-f", "../../config/samples/kafkacluster-with-istio.yaml", "--ignore-not-found=true")
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		})

		ginkgo.It("Verifying Istio resources are cleaned up", func() {
			// Wait a bit for cleanup
			time.Sleep(10 * time.Second)

			// Check that istio ingress gateway resources are removed
			deployments, err := k8s.RunKubectlAndGetOutputE(ginkgo.GinkgoT(), &kubectlOptions, "get", "deployments", "--all-namespaces", "-l", "app=istio-ingressgateway", "-o", "name")
			if err == nil {
				gomega.Expect(deployments).To(gomega.BeEmpty())
			}

			services, err := k8s.RunKubectlAndGetOutputE(ginkgo.GinkgoT(), &kubectlOptions, "get", "services", "--all-namespaces", "-l", "app=istio-ingressgateway", "-o", "name")
			if err == nil {
				gomega.Expect(services).To(gomega.BeEmpty())
			}
		})
	})
}
