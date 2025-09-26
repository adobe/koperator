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

	"github.com/gruntwork-io/terratest/modules/k8s"
	ginkgo "github.com/onsi/ginkgo/v2"
	gomega "github.com/onsi/gomega"
)

// requireUninstallingKoperator uninstall koperator Helm chart and removes Koperator's CRDs.
func requireUninstallingKoperator(kubectlOptions k8s.KubectlOptions) {
	ginkgo.When("Uninstalling Koperator", func() {
		requireUninstallingKoperatorHelmChart(kubectlOptions)
		requireRemoveKoperatorCRDs(kubectlOptions)
		requireRemoveNamespace(kubectlOptions, koperatorLocalHelmDescriptor.Namespace)
	})
}

// requireUninstallingKoperatorHelmChart uninstall Koperator Helm chart
// and checks the success of that operation.
func requireUninstallingKoperatorHelmChart(kubectlOptions k8s.KubectlOptions) {
	ginkgo.It("Uninstalling Koperator Helm chart", func() {
		err := koperatorLocalHelmDescriptor.uninstallHelmChart(kubectlOptions, true)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		ginkgo.By("Verifying Koperator helm chart resources cleanup")
		k8sResourceKinds, err := listK8sResourceKinds(kubectlOptions, "")
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

		koperatorAvailableResourceKinds := stringSlicesInstersect(koperatorCRDs(), k8sResourceKinds)
		koperatorAvailableResourceKinds = append(koperatorAvailableResourceKinds, basicK8sResourceKinds()...)

		remainedResources, err := getK8sResources(kubectlOptions,
			koperatorAvailableResourceKinds,
			fmt.Sprintf(managedByHelmLabelTemplate, koperatorLocalHelmDescriptor.ReleaseName),
			"",
			kubectlArgGoTemplateKindNameNamespace,
			"--all-namespaces")

		gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
		gomega.Expect(remainedResources).Should(gomega.BeEmpty())
	})
}

// requireRemoveKoperatorCRDs deletes the koperator CRDs
func requireRemoveKoperatorCRDs(kubectlOptions k8s.KubectlOptions) {
	ginkgo.It("Removing koperator CRDs", func() {
		for _, crd := range koperatorCRDs() {
			err := deleteK8sResourceNoErrNotFound(kubectlOptions, defaultDeletionTimeout, crdKind, crd)
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
		}
	})
}

// requireUninstallingZookeeperOperator uninstall Zookeeper-operator Helm chart
// and remove CRDs.
func requireUninstallingZookeeperOperator(kubectlOptions k8s.KubectlOptions) {
	ginkgo.When("Uninstalling zookeeper-operator", func() {
		requireUninstallingZookeeperOperatorHelmChart(kubectlOptions)
		requireRemoveZookeeperOperatorCRDs(kubectlOptions)
		requireRemoveNamespace(kubectlOptions, zookeeperOperatorHelmDescriptor.Namespace)
	})
}

// requireUninstallingZookeeperOperatorHelmChart uninstall Zookeeper-operator Helm chart
// and checks the success of that operation.
func requireUninstallingZookeeperOperatorHelmChart(kubectlOptions k8s.KubectlOptions) {
	ginkgo.It("Uninstalling zookeeper-operator Helm chart", func() {
		err := zookeeperOperatorHelmDescriptor.uninstallHelmChart(kubectlOptions, true)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		ginkgo.By("Verifying Zookeeper-operator helm chart resources cleanup")

		k8sResourceKinds, err := listK8sResourceKinds(kubectlOptions, "")
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

		zookeeperAvailableResourceKinds := stringSlicesInstersect(dependencyCRDs.Zookeeper(), k8sResourceKinds)
		zookeeperAvailableResourceKinds = append(zookeeperAvailableResourceKinds, basicK8sResourceKinds()...)

		remainedResources, err := getK8sResources(kubectlOptions,
			zookeeperAvailableResourceKinds,
			fmt.Sprintf(managedByHelmLabelTemplate, zookeeperOperatorHelmDescriptor.ReleaseName),
			"",
			kubectlArgGoTemplateKindNameNamespace,
			"--all-namespaces")
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

		gomega.Expect(remainedResources).Should(gomega.BeEmpty())
	})
}

// requireRemoveZookeeperOperatorCRDs deletes the zookeeper-operator CRDs
func requireRemoveZookeeperOperatorCRDs(kubectlOptions k8s.KubectlOptions) {
	ginkgo.It("Removing zookeeper-operator CRDs", func() {
		for _, crd := range dependencyCRDs.Zookeeper() {
			err := deleteK8sResourceNoErrNotFound(kubectlOptions, defaultDeletionTimeout, crdKind, crd)
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
		}
	})
}

// requireUninstallingPrometheusOperator uninstall prometheus-operator Helm chart and
// remove CRDs.
func requireUninstallingPrometheusOperator(kubectlOptions k8s.KubectlOptions) {
	ginkgo.When("Uninstalling prometheus-operator", func() {
		requireUninstallingPrometheusOperatorHelmChart(kubectlOptions)
		requireRemovePrometheusOperatorCRDs(kubectlOptions)
		requireRemoveNamespace(kubectlOptions, prometheusOperatorHelmDescriptor.Namespace)
	})
}

// requireUninstallingPrometheusOperatorHelmChart uninstall prometheus-operator Helm chart
// and checks the success of that operation.
func requireUninstallingPrometheusOperatorHelmChart(kubectlOptions k8s.KubectlOptions) {
	ginkgo.It("Uninstalling Prometheus-operator Helm chart", func() {
		err := prometheusOperatorHelmDescriptor.uninstallHelmChart(kubectlOptions, true)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		ginkgo.By("Verifying Prometheus-operator helm chart resources cleanup")

		k8sResourceKinds, err := listK8sResourceKinds(kubectlOptions, "")
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

		prometheusAvailableResourceKinds := stringSlicesInstersect(dependencyCRDs.Prometheus(), k8sResourceKinds)
		prometheusAvailableResourceKinds = append(prometheusAvailableResourceKinds, basicK8sResourceKinds()...)

		remainedResources, err := getK8sResources(kubectlOptions,
			prometheusAvailableResourceKinds,
			fmt.Sprintf(managedByHelmLabelTemplate, prometheusOperatorHelmDescriptor.ReleaseName),
			"",
			kubectlArgGoTemplateKindNameNamespace,
			"--all-namespaces")
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

		gomega.Expect(remainedResources).Should(gomega.BeEmpty())
	})
}

// requireRemovePrometheusOperatorCRDs deletes the Prometheus-operator CRDs
func requireRemovePrometheusOperatorCRDs(kubectlOptions k8s.KubectlOptions) {
	ginkgo.It("Removing prometheus-operator CRDs", func() {
		for _, crd := range dependencyCRDs.Prometheus() {
			err := deleteK8sResourceNoErrNotFound(kubectlOptions, defaultDeletionTimeout, crdKind, crd)
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
		}
	})
}

// requireUninstallingCertManager uninstall Cert-manager Helm chart and
// remove CRDs.
func requireUninstallingCertManager(kubectlOptions k8s.KubectlOptions) {
	ginkgo.When("Uninstalling cert-manager", func() {
		requireUninstallingCertManagerHelmChart(kubectlOptions)
		requireRemoveCertManagerCRDs(kubectlOptions)
		requireRemoveNamespace(kubectlOptions, certManagerHelmDescriptor.Namespace)
	})
}

// requireUninstallingCertManagerHelmChart uninstalls cert-manager helm chart
// and checks the success of that operation.
func requireUninstallingCertManagerHelmChart(kubectlOptions k8s.KubectlOptions) {
	ginkgo.It("Uninstalling Cert-manager Helm chart", func() {
		err := certManagerHelmDescriptor.uninstallHelmChart(kubectlOptions, true)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		ginkgo.By("Verifying Cert-manager helm chart resources cleanup")

		k8sResourceKinds, err := listK8sResourceKinds(kubectlOptions, "")
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

		certManagerAvailableResourceKinds := stringSlicesInstersect(dependencyCRDs.CertManager(), k8sResourceKinds)
		certManagerAvailableResourceKinds = append(certManagerAvailableResourceKinds, basicK8sResourceKinds()...)

		remainedResources, err := getK8sResources(kubectlOptions,
			certManagerAvailableResourceKinds,
			fmt.Sprintf(managedByHelmLabelTemplate, certManagerHelmDescriptor.ReleaseName),
			"",
			kubectlArgGoTemplateKindNameNamespace,
			"--all-namespaces")
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

		gomega.Expect(remainedResources).Should(gomega.BeEmpty())
	})
}

// requireRemoveCertManagerCRDs deletes the cert-manager CRDs
func requireRemoveCertManagerCRDs(kubectlOptions k8s.KubectlOptions) {
	ginkgo.It("Removing cert-manager CRDs", func() {
		// First, try to remove CRDs detected by the dependencyCRDs system
		for _, crd := range dependencyCRDs.CertManager() {
			err := deleteK8sResourceNoErrNotFound(kubectlOptions, defaultDeletionTimeout, crdKind, crd)
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
		}

		// Additionally, explicitly remove known cert-manager CRDs to ensure complete cleanup
		knownCertManagerCRDs := []string{
			"certificaterequests.cert-manager.io",
			"certificates.cert-manager.io",
			"challenges.acme.cert-manager.io",
			"clusterissuers.cert-manager.io",
			"issuers.cert-manager.io",
			"orders.acme.cert-manager.io",
		}

		for _, crd := range knownCertManagerCRDs {
			err := deleteK8sResourceNoErrNotFound(kubectlOptions, defaultDeletionTimeout, crdKind, crd)
			if err != nil && !isKubectlNotFoundError(err) {
				ginkgo.By(fmt.Sprintf("Warning: Failed to delete CRD %s: %v", crd, err))
			}
		}

		// Verify that cert-manager CRDs are actually removed
		ginkgo.By("Verifying cert-manager CRDs cleanup")
		remainingCRDs, err := getK8sResources(kubectlOptions, []string{crdKind}, "", "", "--all-namespaces", kubectlArgGoTemplateKindNameNamespace)
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

		// Check if any cert-manager CRDs are still present
		for _, crd := range remainingCRDs {
			if strings.Contains(crd, "cert-manager.io") {
				ginkgo.By(fmt.Sprintf("Warning: cert-manager CRD still present: %s", crd))
			}
		}
	})
}
func requireUninstallingContour(kubectlOptions k8s.KubectlOptions) {
	ginkgo.When("Uninstalling zookeeper-operator", func() {
		requireUninstallingContourHelmChart(kubectlOptions)
		requireRemoveContourCRDs(kubectlOptions)
		requireRemoveNamespace(kubectlOptions, contourIngressControllerHelmDescriptor.Namespace)
	})
}

// requireUninstallingCertManagerHelmChart uninstalls cert-manager helm chart
// and checks the success of that operation.
func requireUninstallingContourHelmChart(kubectlOptions k8s.KubectlOptions) {
	ginkgo.It("Uninstalling Project Contour Helm chart", func() {
		err := contourIngressControllerHelmDescriptor.uninstallHelmChart(kubectlOptions, true)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		ginkgo.By("Verifying Project Contour helm chart resources cleanup")

		k8sResourceKinds, err := listK8sResourceKinds(kubectlOptions, "")
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

		contourAvailableResourceKinds := stringSlicesInstersect(dependencyCRDs.Contour(), k8sResourceKinds)
		contourAvailableResourceKinds = append(contourAvailableResourceKinds, basicK8sResourceKinds()...)

		remainedResources, err := getK8sResources(kubectlOptions,
			contourAvailableResourceKinds,
			fmt.Sprintf(managedByHelmLabelTemplate, contourIngressControllerHelmDescriptor.ReleaseName),
			"",
			kubectlArgGoTemplateKindNameNamespace,
			"--all-namespaces")
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

		gomega.Expect(remainedResources).Should(gomega.BeEmpty())
	})
}

func requireRemoveContourCRDs(kubectlOptions k8s.KubectlOptions) {
	ginkgo.It("Removing Contour Ingress Controller CRDs", func() {
		for _, crd := range dependencyCRDs.Contour() {
			err := deleteK8sResourceNoErrNotFound(kubectlOptions, defaultDeletionTimeout, crdKind, crd)
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
		}
	})
}

// requireRemoveNamespace deletes the indicated namespace object
func requireRemoveNamespace(kubectlOptions k8s.KubectlOptions, namespace string) {
	ginkgo.It(fmt.Sprintf("Removing namespace %s", namespace), func() {
		err := deleteK8sResourceNoErrNotFound(kubectlOptions, defaultDeletionTimeout, "namespace", namespace, "--wait")
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
	})
}
