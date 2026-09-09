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
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type clusterSnapshot struct {
	resources []metav1.PartialObjectMetadata
}

func (s *clusterSnapshot) Resources() []metav1.PartialObjectMetadata {
	return s.resources
}

// ResourcesAsComparisonType returns a slice of a helper type that makes comparisons easier
func (s *clusterSnapshot) ResourcesAsComparisonType() []localComparisonPartialObjectMetadataType {
	var localList []localComparisonPartialObjectMetadataType
	for _, r := range s.resources {
		// Filter out cert-manager and envoy-gateway related resources to avoid comparison failures
		// when these components are not fully cleaned up during uninstall
		resourceName := r.GetName()
		if strings.Contains(resourceName, "cert-manager") || strings.Contains(resourceName, "acme.cert-manager") {
			continue
		}
		// Filter out Envoy Gateway and Gateway API resources (CRDs, RBAC, APIServices)
		if strings.Contains(resourceName, "gateway.envoyproxy.io") ||
			strings.Contains(resourceName, "gateway.networking.x-k8s.io") ||
			strings.Contains(resourceName, "gateway.networking.k8s.io") ||
			strings.Contains(resourceName, "eg-gateway-helm-certgen") {
			continue
		}
		// Filter out Kind cluster infrastructure resources
		if resourceName == "cloud-provider-kind" {
			continue
		}

		localList = append(localList, localComparisonPartialObjectMetadataType{
			GVK:       r.GroupVersionKind(),
			Namespace: r.GetNamespace(),
			Name:      r.GetName(),
		})
	}
	return localList
}

// localComparisonPartialObjectMetadataType holds a version of the minimal information required
// to compare k8s.io/apimachinery/pkg/apis/meta/v1.PartialObjectMetadata instances
type localComparisonPartialObjectMetadataType struct {
	GVK       schema.GroupVersionKind
	Namespace string
	Name      string
}

// snapshotCluster takes a clusterSnapshot of a K8s cluster and
// stores it into the snapshotCluster instance referenced as input
func snapshotCluster(snapshottedInfo *clusterSnapshot) bool { //nolint:unparam // Note: respecting Ginkgo testing interface by returning bool.
	return ginkgo.When("Get cluster resources state", ginkgo.Ordered, func() {
		var kubectlOptions k8s.KubectlOptions
		var err error

		ginkgo.BeforeAll(func() {
			ginkgo.By("Acquiring K8s config and context")
			kubectlOptions, err = kubectlOptionsForCurrentContext()
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		})

		var clusterResourceNames []string
		var namespacedResourceNames []string

		ginkgo.When("Get api-resources names", func() {
			ginkgo.It("Get cluster-scoped api-resources names", func() {
				clusterResourceNames, err = listK8sResourceKinds(kubectlOptions, "", "--namespaced=false")
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(clusterResourceNames).NotTo(gomega.BeNil())
				clusterResourceNames = pruneUnnecessaryClusterResourceNames(clusterResourceNames)
			})
			ginkgo.It("Get namespaced api-resources names", func() {
				namespacedResourceNames, err = listK8sResourceKinds(kubectlOptions, "", "--namespaced=true")
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(namespacedResourceNames).NotTo(gomega.BeNil())
				namespacedResourceNames = pruneUnnecessaryNamespacedResourceNames(namespacedResourceNames)
			})
		})

		var resources []metav1.PartialObjectMetadata

		var namespacesForNamespacedResources = []string{"default"}

		ginkgo.When("Snapshotting objects", func() {
			ginkgo.It("Recording cluster-scoped resource objects", func() {
				ginkgo.By(fmt.Sprintf("Getting cluster-scoped resources %v as json", clusterResourceNames))
				output, err := getK8sResourcesQuiet(kubectlOptions, clusterResourceNames, "", "", "--output=json")
				gomega.Expect(err).NotTo(gomega.HaveOccurred())

				ginkgo.By(fmt.Sprintf("Unmarshalling cluster-scoped resources %v from json", clusterResourceNames))
				var resourceList metav1.PartialObjectMetadataList
				err = json.Unmarshal([]byte(strings.Join(output, "\n")), &resourceList)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())

				// Log only the resource names instead of full content
				resourceNames := make([]string, len(resourceList.Items))
				for i, resource := range resourceList.Items {
					resourceNames[i] = fmt.Sprintf("%s/%s", resource.GroupVersionKind().Kind, resource.GetName())
				}
				ginkgo.By(fmt.Sprintf("Recorded %d cluster-scoped resources: %v", len(resourceList.Items), resourceNames))

				resources = append(resources, resourceList.Items...)
			})
			ginkgo.It("Recording namespaced resource objects", func() {
				initialNS := kubectlOptions.Namespace
				for _, ns := range namespacesForNamespacedResources {
					kubectlOptions.Namespace = ns

					ginkgo.By(fmt.Sprintf("Getting namespaced resources %v as json for namespace %s", namespacedResourceNames, ns))
					output, err := getK8sResourcesQuiet(kubectlOptions, namespacedResourceNames, "", "", "--output=json")
					gomega.Expect(err).NotTo(gomega.HaveOccurred())

					ginkgo.By(fmt.Sprintf("Unmarshalling namespaced resources %v from json for namespace %s", namespacedResourceNames, ns))
					var resourceList metav1.PartialObjectMetadataList
					err = json.Unmarshal([]byte(strings.Join(output, "\n")), &resourceList)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())

					// Log only the resource names instead of full content
					resourceNames := make([]string, len(resourceList.Items))
					for i, resource := range resourceList.Items {
						resourceNames[i] = fmt.Sprintf("%s/%s", resource.GroupVersionKind().Kind, resource.GetName())
					}
					ginkgo.By(fmt.Sprintf("Recorded %d namespaced resources in %s: %v", len(resourceList.Items), ns, resourceNames))

					resources = append(resources, resourceList.Items...)
				}
				kubectlOptions.Namespace = initialNS
			})
		})

		ginkgo.AfterAll(func() {
			ginkgo.By("Storing recorded objects into the input snapshot object")
			snapshottedInfo.resources = resources
		})
	})
}

// snapshotClusterAndCompare takes a current snapshot of the K8s cluster and
// compares it against a snapshot provided as input
func snapshotClusterAndCompare(snapshottedInitialInfo *clusterSnapshot) bool {
	return ginkgo.When("Verifying cluster resources state", ginkgo.Ordered, func() {
		var snapshottedCurrentInfo = &clusterSnapshot{}
		snapshotCluster(snapshottedCurrentInfo)

		ginkgo.It("Checking resources list", func() {
			current := snapshottedCurrentInfo.ResourcesAsComparisonType()
			initial := snapshottedInitialInfo.ResourcesAsComparisonType()

			// Calculate differences for better error reporting
			var extra []localComparisonPartialObjectMetadataType
			var missing []localComparisonPartialObjectMetadataType

			for _, c := range current {
				found := false
				for _, i := range initial {
					if c.GVK == i.GVK && c.Namespace == i.Namespace && c.Name == i.Name {
						found = true
						break
					}
				}
				if !found {
					extra = append(extra, c)
				}
			}

			for _, i := range initial {
				found := false
				for _, c := range current {
					if c.GVK == i.GVK && c.Namespace == i.Namespace && c.Name == i.Name {
						found = true
						break
					}
				}
				if !found {
					missing = append(missing, i)
				}
			}

			// If there are differences, print them clearly and fail with a simple message
			if len(extra) > 0 || len(missing) > 0 {
				if len(extra) > 0 {
					ginkgo.GinkgoWriter.Printf("\n=== EXTRA RESOURCES (present now but not in initial snapshot) ===\n")
					for _, r := range extra {
						ginkgo.GinkgoWriter.Printf("  %s/%s %s (namespace: %q)\n", r.GVK.Group, r.GVK.Kind, r.Name, r.Namespace)
					}
				}

				if len(missing) > 0 {
					ginkgo.GinkgoWriter.Printf("\n=== MISSING RESOURCES (present in initial snapshot but not now) ===\n")
					for _, r := range missing {
						ginkgo.GinkgoWriter.Printf("  %s/%s %s (namespace: %q)\n", r.GVK.Group, r.GVK.Kind, r.Name, r.Namespace)
					}
				}

				ginkgo.Fail(fmt.Sprintf("Cluster resources mismatch: %d extra, %d missing (see details above)", len(extra), len(missing)))
			}
		})
	})
}

func pruneUnnecessaryClusterResourceNames(resourceNameList []string) []string {
	var updatedList []string
	for _, name := range resourceNameList {
		// Avoid failing because the number of K8s workers changed during the test. (e.g. PKE)
		if name == "nodes" {
			continue
		}
		// When the number of nodes changes we also get CSRs for signers kubernetes.io/kubelet-serving and kubernetes.io/kube-apiserver-client-kubelet
		// TODO: in time, we want to be able to compare CSRs, too, or be able to ignore particular CSR list differences.
		if name == "certificatesigningrequests.certificates.k8s.io" {
			continue
		}
		// Ignore CSI elements from storage.k8s.io
		// Additionally, these resources don't mesh well with computing differences for clusters with a variable number of workers.
		if name == "csidrivers.storage.k8s.io" || name == "csinodes.storage.k8s.io" || name == "csistoragecapacities.storage.k8s.io" {
			continue
		}
		// We never need to snapshot Cilium-related resources (namespaced or not).
		if strings.HasPrefix(name, "cilium") {
			continue
		}
		// We never need to snapshot cert-manager-related resources (namespaced or not).
		// These resources may not be fully cleaned up during uninstall and can cause snapshot comparison failures.
		if strings.Contains(name, "cert-manager") || strings.Contains(name, "acme.cert-manager") {
			continue
		}
		// ComponentStatus is deprecated in Kubernetes v1.19+ and causes warnings
		if name == "componentstatuses" {
			continue
		}
		updatedList = append(updatedList, name)
	}
	return updatedList
}

func pruneUnnecessaryNamespacedResourceNames(resourceNameList []string) []string {
	var updatedList []string
	for _, name := range resourceNameList {
		// The list of K8s Events is rarely unchanged over time. It is not fit for comparison.
		// Additionally, at the very least, KafkaCluster installs create PVs which generate events by themselves.
		if name == "events" || name == "events.events.k8s.io" {
			continue
		}
		// We never need to snapshot Cilium-related resources (namespaced or not).
		if strings.HasPrefix(name, "cilium") {
			continue
		}
		// We never need to snapshot cert-manager-related resources (namespaced or not).
		// These resources may not be fully cleaned up during uninstall and can cause snapshot comparison failures.
		if strings.Contains(name, "cert-manager") || strings.Contains(name, "acme.cert-manager") {
			continue
		}
		updatedList = append(updatedList, name)
	}
	return updatedList
}
