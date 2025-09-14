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
	"github.com/gruntwork-io/terratest/modules/k8s"
	ginkgo "github.com/onsi/ginkgo/v2"
	gomega "github.com/onsi/gomega"
)

func testInstall() bool {
	return ginkgo.When("Installing Koperator and dependencies", ginkgo.Ordered, func() {
		var kubectlOptions k8s.KubectlOptions
		var err error

		ginkgo.It("Acquiring K8s config and context", func() {
			kubectlOptions, err = kubectlOptionsForCurrentContext()
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		})

		ginkgo.When("Installing cert-manager", func() {
			ginkgo.It("Installing cert-manager Helm chart", func() {
				err = certManagerHelmDescriptor.installHelmChart(kubectlOptions)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
			})
		})

		ginkgo.When("Installing contour ingress controller", func() {
			ginkgo.It("Installing contour Helm chart", func() {
				err = contourIngressControllerHelmDescriptor.installHelmChart(kubectlOptions)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
			})
		})

		ginkgo.When("Installing zookeeper-operator", func() {
			ginkgo.It("Installing zookeeper-operator Helm chart", func() {
				err = zookeeperOperatorHelmDescriptor.installHelmChart(kubectlOptions)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
			})
		})

		ginkgo.When("Installing prometheus-operator", func() {
			ginkgo.It("Installing prometheus-operator Helm chart", func() {
				err = prometheusOperatorHelmDescriptor.installHelmChart(kubectlOptions)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
			})
		})

		ginkgo.When("Installing Koperator", func() {
			ginkgo.It("Installing Koperator Helm chart", func() {
				err = koperatorLocalHelmDescriptor.installHelmChart(kubectlOptions)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
			})
		})
	})
}
