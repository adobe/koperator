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
	"sync"

	"github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

func testInstall() bool {
	return ginkgo.When("Installing Koperator and dependencies", ginkgo.Ordered, func() {
		var kubectlOptions k8s.KubectlOptions
		var err error

		ginkgo.It("Acquiring K8s config and context", func() {
			kubectlOptions, err = kubectlOptionsForCurrentContext()
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		})

		ginkgo.It("Installing infrastructure components in parallel", func() {
			var wg sync.WaitGroup
			errChan := make(chan error, 3)

			// Install cert-manager, Contour, and Envoy Gateway in parallel
			wg.Add(3)

			go func() {
				defer wg.Done()
				ginkgo.By("Installing cert-manager Helm chart")
				if installErr := certManagerHelmDescriptor.installHelmChart(kubectlOptions); installErr != nil {
					errChan <- installErr
				}
			}()

			go func() {
				defer wg.Done()
				ginkgo.By("Installing Contour Helm chart")
				if installErr := contourIngressControllerHelmDescriptor.installHelmChart(kubectlOptions); installErr != nil {
					errChan <- installErr
				}
			}()

			go func() {
				defer wg.Done()
				ginkgo.By("Installing Envoy Gateway Helm chart")
				if installErr := envoyGatewayHelmDescriptor.installHelmChart(kubectlOptions); installErr != nil {
					errChan <- installErr
				}
			}()

			wg.Wait()
			close(errChan)

			// Check for errors
			for installErr := range errChan {
				gomega.Expect(installErr).NotTo(gomega.HaveOccurred())
			}
		})

		ginkgo.It("Creating Envoy Gateway GatewayClass", func() {
			gatewayClassManifest := `apiVersion: gateway.networking.k8s.io/v1
kind: GatewayClass
metadata:
  name: eg
spec:
  controllerName: gateway.envoyproxy.io/gatewayclass-controller`
			err = applyK8sResourceManifestFromString(kubectlOptions, gatewayClassManifest)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		})

		ginkgo.It("Installing dependency operators in parallel", func() {
			var wg sync.WaitGroup
			errChan := make(chan error, 2)

			// Install zookeeper-operator and prometheus-operator in parallel
			wg.Add(2)

			go func() {
				defer wg.Done()
				ginkgo.By("Installing zookeeper-operator Helm chart")
				if installErr := zookeeperOperatorHelmDescriptor.installHelmChart(kubectlOptions); installErr != nil {
					errChan <- installErr
				}
			}()

			go func() {
				defer wg.Done()
				ginkgo.By("Installing prometheus-operator Helm chart")
				if installErr := prometheusOperatorHelmDescriptor.installHelmChart(kubectlOptions); installErr != nil {
					errChan <- installErr
				}
			}()

			wg.Wait()
			close(errChan)

			// Check for errors
			for installErr := range errChan {
				gomega.Expect(installErr).NotTo(gomega.HaveOccurred())
			}
		})

		ginkgo.It("Installing Koperator Helm chart", func() {
			err = koperatorLocalHelmDescriptor.installHelmChart(kubectlOptions)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		})
	})
}
