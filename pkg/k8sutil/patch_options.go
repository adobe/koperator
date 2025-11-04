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

		// Remove fields that mutation webhooks commonly modify
		currentPod = cleanMutationWebhookFields(currentPod)
		modifiedPod = cleanMutationWebhookFields(modifiedPod)

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

func cleanMutationWebhookFields(pod *corev1.Pod) *corev1.Pod {
	// Create a copy to avoid modifying the original
	cleaned := pod.DeepCopy()

	// Remove mutation webhook annotations that should not trigger reconciliation
	if cleaned.Annotations != nil {
		delete(cleaned.Annotations, "gatekeeper.sh/mutation-id")
		delete(cleaned.Annotations, "gatekeeper.sh/mutations")
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
