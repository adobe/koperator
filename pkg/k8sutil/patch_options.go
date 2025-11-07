// Copyright © 2019 Cisco Systems, Inc. and/or its affiliates
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

package k8sutil

import (
	"emperror.dev/errors"
	json "github.com/json-iterator/go"
	corev1 "k8s.io/api/core/v1"

	"github.com/banzaicloud/k8s-objectmatcher/patch"
)

// IgnoreMutationWebhookFields creates a CalculateOption that ignores fields commonly
// modified by mutation webhooks like Gatekeeper, OPA, and Pod Security Policies
func IgnoreMutationWebhookFields() patch.CalculateOption {
	return func(current, modified []byte) ([]byte, []byte, error) {
		currentPod := &corev1.Pod{}
		if err := json.Unmarshal(current, currentPod); err != nil {
			// Not a pod, return unchanged
			return current, modified, nil
		}

		modifiedPod := &corev1.Pod{}
		if err := json.Unmarshal(modified, modifiedPod); err != nil {
			return current, modified, nil
		}

		// Check if ScaleOps is managing resources in EITHER pod
		isScaleOpsManaged := isScaleOpsManagedPod(currentPod) || isScaleOpsManagedPod(modifiedPod)

		// Remove fields that mutation webhooks commonly modify
		currentPod = cleanMutationWebhookFields(currentPod, isScaleOpsManaged)
		modifiedPod = cleanMutationWebhookFields(modifiedPod, isScaleOpsManaged)

		currentBytes, err := json.Marshal(currentPod)
		if err != nil {
			return []byte{}, []byte{}, errors.Wrap(err, "could not marshal cleaned current pod")
		}

		modifiedBytes, err := json.Marshal(modifiedPod)
		if err != nil {
			return []byte{}, []byte{}, errors.Wrap(err, "could not marshal cleaned modified pod")
		}

		return currentBytes, modifiedBytes, nil
	}
}

// isScaleOpsManagedPod checks if a pod is managed by ScaleOps
func isScaleOpsManagedPod(pod *corev1.Pod) bool {
	return pod.Annotations != nil && (pod.Annotations["scaleops.sh/managed-containers"] != "" ||
		pod.Annotations["scaleops.sh/pod-owner-grouping"] != "")
}

func cleanMutationWebhookFields(pod *corev1.Pod, isScaleOpsManaged bool) *corev1.Pod {
	// Create a copy to avoid modifying the original
	cleaned := pod.DeepCopy()

	// Remove mutation webhook annotations that should not trigger reconciliation
	if cleaned.Annotations != nil {
		// Gatekeeper annotations
		delete(cleaned.Annotations, "gatekeeper.sh/mutation-id")
		delete(cleaned.Annotations, "gatekeeper.sh/mutations")

		// ScaleOps annotations
		delete(cleaned.Annotations, "scaleops.sh/admission")
		delete(cleaned.Annotations, "scaleops.sh/applied-policy")
		delete(cleaned.Annotations, "scaleops.sh/last-applied-resources")
		delete(cleaned.Annotations, "scaleops.sh/managed-containers")
		delete(cleaned.Annotations, "scaleops.sh/managed-keep-limit-cpu")
		delete(cleaned.Annotations, "scaleops.sh/managed-keep-limit-memory")
		delete(cleaned.Annotations, "scaleops.sh/origin-resources")
		delete(cleaned.Annotations, "scaleops.sh/pod-owner-grouping")
		delete(cleaned.Annotations, "scaleops.sh/pod-owner-identifier")

		// Remove the last-applied annotation that may contain ScaleOps fields
		// Note: This is regenerated on updates by the k8s-objectmatcher library
		delete(cleaned.Annotations, "banzaicloud.com/last-applied")

		// If annotations map is empty, set to nil to normalize comparison
		if len(cleaned.Annotations) == 0 {
			cleaned.Annotations = nil
		}
	}

	// Remove ScaleOps labels
	if cleaned.Labels != nil {
		delete(cleaned.Labels, "scaleops.sh/applied-recommendation")
		delete(cleaned.Labels, "scaleops.sh/managed")
		delete(cleaned.Labels, "scaleops.sh/managed-unevictable")
		delete(cleaned.Labels, "scaleops.sh/pod-owner-grouping")
		delete(cleaned.Labels, "scaleops.sh/pod-owner-identifier")

		// If labels map is empty, set to nil to normalize comparison
		if len(cleaned.Labels) == 0 {
			cleaned.Labels = nil
		}
	}

	// Remove ScaleOps-added affinity rules (preferred scheduling only)
	if cleaned.Spec.Affinity != nil {
		if cleaned.Spec.Affinity.NodeAffinity != nil {
			// Remove preferred node affinity added by ScaleOps (node-packing)
			if cleaned.Spec.Affinity.NodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution != nil {
				var filtered []corev1.PreferredSchedulingTerm
				for _, term := range cleaned.Spec.Affinity.NodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution {
					// Keep only terms that are NOT ScaleOps node-packing preferences
					isScaleOpsTerm := false
					for _, expr := range term.Preference.MatchExpressions {
						if expr.Key == "scaleops.sh/node-packing" {
							isScaleOpsTerm = true
							break
						}
					}
					if !isScaleOpsTerm {
						filtered = append(filtered, term)
					}
				}
				if len(filtered) == 0 {
					cleaned.Spec.Affinity.NodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution = nil
				} else {
					cleaned.Spec.Affinity.NodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution = filtered
				}
			}
			// Clean up empty NodeAffinity
			if cleaned.Spec.Affinity.NodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution == nil &&
				cleaned.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
				cleaned.Spec.Affinity.NodeAffinity = nil
			}
		}

		if cleaned.Spec.Affinity.PodAffinity != nil {
			// Remove preferred pod affinity added by ScaleOps (managed-unevictable)
			if cleaned.Spec.Affinity.PodAffinity.PreferredDuringSchedulingIgnoredDuringExecution != nil {
				var filtered []corev1.WeightedPodAffinityTerm
				for _, term := range cleaned.Spec.Affinity.PodAffinity.PreferredDuringSchedulingIgnoredDuringExecution {
					// Keep only terms that are NOT ScaleOps managed-unevictable preferences
					isScaleOpsTerm := false
					if term.PodAffinityTerm.LabelSelector != nil {
						for _, expr := range term.PodAffinityTerm.LabelSelector.MatchExpressions {
							if expr.Key == "scaleops.sh/managed-unevictable" {
								isScaleOpsTerm = true
								break
							}
						}
					}
					if !isScaleOpsTerm {
						filtered = append(filtered, term)
					}
				}
				if len(filtered) == 0 {
					cleaned.Spec.Affinity.PodAffinity.PreferredDuringSchedulingIgnoredDuringExecution = nil
				} else {
					cleaned.Spec.Affinity.PodAffinity.PreferredDuringSchedulingIgnoredDuringExecution = filtered
				}
			}
			// Clean up empty PodAffinity
			if cleaned.Spec.Affinity.PodAffinity.PreferredDuringSchedulingIgnoredDuringExecution == nil &&
				cleaned.Spec.Affinity.PodAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
				cleaned.Spec.Affinity.PodAffinity = nil
			}
		}

		// Clean up empty Affinity
		if cleaned.Spec.Affinity.NodeAffinity == nil &&
			cleaned.Spec.Affinity.PodAffinity == nil &&
			cleaned.Spec.Affinity.PodAntiAffinity == nil {
			cleaned.Spec.Affinity = nil
		}
	}

	// Clean resources if ScaleOps is managing them
	if isScaleOpsManaged {
		for i := range cleaned.Spec.InitContainers {
			cleaned.Spec.InitContainers[i].Resources = corev1.ResourceRequirements{}
		}
		for i := range cleaned.Spec.Containers {
			cleaned.Spec.Containers[i].Resources = corev1.ResourceRequirements{}
		}
	}

	// Clean security context fields commonly set by PSPs/Gatekeeper
	for i := range cleaned.Spec.InitContainers {
		cleanSecurityContext(&cleaned.Spec.InitContainers[i])
	}
	for i := range cleaned.Spec.Containers {
		cleanSecurityContext(&cleaned.Spec.Containers[i])
	}

	return cleaned
}

func cleanSecurityContext(container *corev1.Container) {
	if container.SecurityContext == nil {
		return
	}

	// Note: We intentionally do NOT clean security context fields here by default
	// because those are typically important security controls that should be reconciled.
	// If you need to ignore specific security context fields, uncomment the relevant lines below:

	// AllowPrivilegeEscalation is often set by PSPs
	// container.SecurityContext.AllowPrivilegeEscalation = nil

	// ReadOnlyRootFilesystem is often set by PSPs
	// container.SecurityContext.ReadOnlyRootFilesystem = nil

	// Capabilities are often modified by PSPs
	// container.SecurityContext.Capabilities = nil
}

// IgnorePodResourcesIfAnnotated creates a CalculateOption that ignores pod resource
// requests/limits if the pod has specific annotations indicating it's managed by
// an external system (e.g., ScaleOps, VPA)
func IgnorePodResourcesIfAnnotated() patch.CalculateOption {
	return func(current, modified []byte) ([]byte, []byte, error) {
		currentMap := map[string]interface{}{}
		if err := json.Unmarshal(current, &currentMap); err != nil {
			return current, modified, nil
		}

		// Check if this pod should ignore resource diffs (e.g., via annotation)
		if shouldIgnoreResources(currentMap) {
			// Remove resources from comparison
			current = removeResourcesFromPod(current)
			modified = removeResourcesFromPod(modified)
		}

		return current, modified, nil
	}
}

func shouldIgnoreResources(podMap map[string]interface{}) bool {
	metadata, ok := podMap["metadata"].(map[string]interface{})
	if !ok {
		return false
	}

	annotations, ok := metadata["annotations"].(map[string]interface{})
	if !ok {
		return false
	}

	// Check for annotations that indicate external resource management
	annotationsToCheck := []string{
		"scaleops.sh/pod-owner-grouping",
		"vpa.k8s.io/updateMode",
		"cluster-autoscaler.kubernetes.io/safe-to-evict-local-volumes",
	}

	for _, ann := range annotationsToCheck {
		if _, exists := annotations[ann]; exists {
			return true
		}
	}

	return false
}

func removeResourcesFromPod(podBytes []byte) []byte {
	podMap := map[string]interface{}{}
	if err := json.Unmarshal(podBytes, &podMap); err != nil {
		return podBytes
	}

	if spec, ok := podMap["spec"].(map[string]interface{}); ok {
		// Remove resources from all containers
		if containers, ok := spec["containers"].([]interface{}); ok {
			for _, c := range containers {
				if container, ok := c.(map[string]interface{}); ok {
					delete(container, "resources")
				}
			}
		}
		if initContainers, ok := spec["initContainers"].([]interface{}); ok {
			for _, c := range initContainers {
				if container, ok := c.(map[string]interface{}); ok {
					delete(container, "resources")
				}
			}
		}
	}

	result, err := json.Marshal(podMap)
	if err != nil {
		return podBytes
	}
	return result
}
