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

package kafka

import (
	"encoding/base64"
	"fmt"
	"reflect"
	"sort"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"

	"github.com/banzaicloud/koperator/api/v1beta1"
)

// generateQuorumVoters generates the quorum voters in the format of brokerID@nodeAddress:listenerPort
// The generated quorum voters are guaranteed in ascending order by broker IDs to ensure same quorum voters configurations are returned
// regardless of the order of brokers and controllerListenerStatuses are passed in - this is needed to avoid triggering
// unnecessary rolling upgrade operations
func generateQuorumVoters(kafkaCluster *v1beta1.KafkaCluster, controllerListenerStatuses map[string]v1beta1.ListenerStatusList) ([]string, error) {
	var (
		quorumVoters []string
		brokerIDs    []int32
	)
	idToListenerAddrMap := make(map[int32]string)

	// find the controller nodes and their corresponding listener addresses
	for _, b := range kafkaCluster.Spec.Brokers {
		brokerConfig, err := b.GetBrokerConfig(kafkaCluster.Spec)
		if err != nil {
			return nil, err
		}

		if brokerConfig.IsControllerNode() {
			for _, controllerListenerStatus := range controllerListenerStatuses {
				for _, status := range controllerListenerStatus {
					if status.Name == fmt.Sprintf("broker-%d", b.Id) {
						idToListenerAddrMap[b.Id] = status.Address
						brokerIDs = append(brokerIDs, b.Id)
						break
					}
				}
			}
		}
	}

	sort.Slice(brokerIDs, func(i, j int) bool {
		return brokerIDs[i] < brokerIDs[j]
	})

	for _, brokerId := range brokerIDs {
		quorumVoters = append(quorumVoters, fmt.Sprintf("%d@%s", brokerId, idToListenerAddrMap[brokerId]))
	}

	return quorumVoters, nil
}

// generateRandomClusterID() generates a based64-encoded random UUID with 16 bytes as the cluster ID
// it uses URL based64 encoding since that's what Kafka expects
func generateRandomClusterID() string {
	randomUUID := uuid.New()
	return base64.URLEncoding.EncodeToString(randomUUID[:])
}

// syncResourceRequests overwrites CPU and memory requests in desiredPod's containers
// with the values from currentPod so that request-only changes do not trigger a pod restart.
func syncResourceRequests(desiredPod, currentPod *corev1.Pod) {
	syncContainerResourceRequests(desiredPod.Spec.Containers, currentPod.Spec.Containers)
	syncContainerResourceRequests(desiredPod.Spec.InitContainers, currentPod.Spec.InitContainers)
}

func syncContainerResourceRequests(desired, current []corev1.Container) {
	index := make(map[string]corev1.ResourceList, len(current))
	for _, c := range current {
		index[c.Name] = c.Resources.Requests
	}
	for i := range desired {
		c := &desired[i]
		reqs, ok := index[c.Name]
		if !ok {
			continue
		}
		if c.Resources.Requests == nil {
			c.Resources.Requests = make(corev1.ResourceList)
		}
		for _, res := range []corev1.ResourceName{corev1.ResourceCPU, corev1.ResourceMemory} {
			if val, exists := reqs[res]; exists {
				c.Resources.Requests[res] = val
			} else {
				delete(c.Resources.Requests, res)
			}
		}
	}
}

// syncScaleOpsAffinities syncs all scale ops related affinities from the current pod to the desired pod.
// This includes pod affinities with "scaleops.sh/managed-unevictable" label selector
// and node affinities with "scaleops.sh/node-packing=true" selector.
func syncScaleOpsAffinities(desiredPod, currentPod *corev1.Pod) {
	syncScaleOpsPodAffinities(desiredPod, currentPod)
	syncScaleOpsNodeAffinities(desiredPod, currentPod)
}

// syncScaleOpsPodAffinities syncs preferred pod affinities with "scaleops.sh/managed-unevictable"
// label selector from current pod to desired pod.
func syncScaleOpsPodAffinities(desiredPod, currentPod *corev1.Pod) {
	if currentPod.Spec.Affinity == nil || currentPod.Spec.Affinity.PodAffinity == nil {
		return
	}

	currentPodAffinity := currentPod.Spec.Affinity.PodAffinity

	// Filter preferred pod affinities with "scaleops.sh/managed-unevictable" label selector
	var scaleOpsPreferredAffinities []corev1.WeightedPodAffinityTerm
	if currentPodAffinity.PreferredDuringSchedulingIgnoredDuringExecution != nil {
		for _, term := range currentPodAffinity.PreferredDuringSchedulingIgnoredDuringExecution {
			if term.PodAffinityTerm.LabelSelector != nil {
				hasScaleOpsLabel := false

				// Check MatchExpressions
				for _, requirement := range term.PodAffinityTerm.LabelSelector.MatchExpressions {
					if requirement.Key == "scaleops.sh/managed-unevictable" {
						hasScaleOpsLabel = true
						break
					}
				}

				// Check MatchLabels if not found in MatchExpressions
				if !hasScaleOpsLabel {
					if _, exists := term.PodAffinityTerm.LabelSelector.MatchLabels["scaleops.sh/managed-unevictable"]; exists {
						hasScaleOpsLabel = true
					}
				}

				if hasScaleOpsLabel {
					scaleOpsPreferredAffinities = append(scaleOpsPreferredAffinities, term)
				}
			}
		}
	}

	// If we found any scale ops preferred affinities, add them to the desired pod
	if len(scaleOpsPreferredAffinities) > 0 {
		if desiredPod.Spec.Affinity == nil {
			desiredPod.Spec.Affinity = &corev1.Affinity{}
		}
		if desiredPod.Spec.Affinity.PodAffinity == nil {
			desiredPod.Spec.Affinity.PodAffinity = &corev1.PodAffinity{}
		}

		// Merge scale ops preferred affinities, avoiding duplicates
		existingTerms := desiredPod.Spec.Affinity.PodAffinity.PreferredDuringSchedulingIgnoredDuringExecution
		for _, newTerm := range scaleOpsPreferredAffinities {
			// Check if this term already exists
			found := false
			for _, existing := range existingTerms {
				if reflect.DeepEqual(existing.PodAffinityTerm, newTerm.PodAffinityTerm) && existing.Weight == newTerm.Weight {
					found = true
					break
				}
			}
			if !found {
				existingTerms = append(existingTerms, newTerm)
			}
		}
		desiredPod.Spec.Affinity.PodAffinity.PreferredDuringSchedulingIgnoredDuringExecution = existingTerms
	}
}

// syncScaleOpsNodeAffinities syncs preferred node affinities with "scaleops.sh/node-packing=true"
// selector from current pod to desired pod.
func syncScaleOpsNodeAffinities(desiredPod, currentPod *corev1.Pod) {
	if currentPod.Spec.Affinity == nil || currentPod.Spec.Affinity.NodeAffinity == nil {
		return
	}

	currentNodeAffinity := currentPod.Spec.Affinity.NodeAffinity

	// Filter preferred node affinities with "scaleops.sh/node-packing=true" selector
	var scaleOpsPreferredTerms []corev1.PreferredSchedulingTerm
	if currentNodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution != nil {
		for _, term := range currentNodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution {
			hasScaleOpsNodePacking := false

			// Check MatchExpressions
			for _, requirement := range term.Preference.MatchExpressions {
				if requirement.Key == "scaleops.sh/node-packing" {
					hasScaleOpsNodePacking = true
				}
				if hasScaleOpsNodePacking {
					break
				}
			}

			// Check MatchFields if not found in MatchExpressions
			if !hasScaleOpsNodePacking {
				for _, requirement := range term.Preference.MatchFields {
					if requirement.Key == "scaleops.sh/node-packing" {
						hasScaleOpsNodePacking = true
					}
					if hasScaleOpsNodePacking {
						break
					}
				}
			}

			if hasScaleOpsNodePacking {
				scaleOpsPreferredTerms = append(scaleOpsPreferredTerms, term)
			}
		}
	}

	// If we found any scale ops node affinities, add them to the desired pod
	if len(scaleOpsPreferredTerms) > 0 {
		if desiredPod.Spec.Affinity == nil {
			desiredPod.Spec.Affinity = &corev1.Affinity{}
		}
		if desiredPod.Spec.Affinity.NodeAffinity == nil {
			desiredPod.Spec.Affinity.NodeAffinity = &corev1.NodeAffinity{}
		}

		// Merge scale ops node affinities, avoiding duplicates
		existingTerms := desiredPod.Spec.Affinity.NodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution
		for _, newTerm := range scaleOpsPreferredTerms {
			// Check if this term already exists
			found := false
			for _, existing := range existingTerms {
				if reflect.DeepEqual(existing.Preference, newTerm.Preference) && existing.Weight == newTerm.Weight {
					found = true
					break
				}
			}
			if !found {
				existingTerms = append(existingTerms, newTerm)
			}
		}
		desiredPod.Spec.Affinity.NodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution = existingTerms
	}
}
