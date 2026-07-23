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
	"encoding/json"
	"fmt"
	"reflect"
	"sort"

	"github.com/banzaicloud/k8s-objectmatcher/patch"
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

// getOriginalPod returns the Pod that Koperator itself last declared as desired for currentPod,
// reconstructed from k8s-objectmatcher's own "last-applied" annotation (the same annotation
// patch.DefaultPatchMaker.Calculate uses as the "original" side of its three-way merge). It
// returns nil if no such annotation is present yet - for example before
// ExternalResourceManagementEnabled was turned on, or on a pod created before this feature
// existed - in which case callers should treat every field as "changed" and fall back to the
// freshly-computed desired value rather than trusting currentPod's live drift.
func getOriginalPod(currentPod *corev1.Pod) (*corev1.Pod, error) {
	raw, err := patch.DefaultAnnotator.GetOriginalConfiguration(currentPod)
	if err != nil || len(raw) == 0 {
		return nil, err
	}
	original := &corev1.Pod{}
	if err := json.Unmarshal(raw, original); err != nil {
		return nil, err
	}
	return original, nil
}

func containerRequestsByName(containers []corev1.Container) map[string]corev1.ResourceList {
	if len(containers) == 0 {
		return nil
	}
	index := make(map[string]corev1.ResourceList, len(containers))
	for _, c := range containers {
		index[c.Name] = c.Resources.Requests
	}
	return index
}

func podAffinityTerms(pod *corev1.Pod) []corev1.WeightedPodAffinityTerm {
	if pod.Spec.Affinity == nil || pod.Spec.Affinity.PodAffinity == nil {
		return nil
	}
	return pod.Spec.Affinity.PodAffinity.PreferredDuringSchedulingIgnoredDuringExecution
}

func nodeAffinityTerms(pod *corev1.Pod) []corev1.PreferredSchedulingTerm {
	if pod.Spec.Affinity == nil || pod.Spec.Affinity.NodeAffinity == nil {
		return nil
	}
	return pod.Spec.Affinity.NodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution
}

// syncResourceRequests overwrites CPU and memory requests in desiredPod's containers with the
// values from currentPod, but only for requests that are unchanged from what Koperator itself
// last declared (per original) - so a genuine change to the KafkaCluster's resource requests
// still takes effect instead of being silently discarded in favor of externally-applied drift.
func syncResourceRequests(desiredPod, currentPod, original *corev1.Pod) {
	var originalContainers, originalInitContainers []corev1.Container
	if original != nil {
		originalContainers = original.Spec.Containers
		originalInitContainers = original.Spec.InitContainers
	}
	syncContainerResourceRequests(desiredPod.Spec.Containers, currentPod.Spec.Containers, originalContainers)
	syncContainerResourceRequests(desiredPod.Spec.InitContainers, currentPod.Spec.InitContainers, originalInitContainers)
}

func syncContainerResourceRequests(desired, current, original []corev1.Container) {
	currentIndex := containerRequestsByName(current)
	originalIndex := containerRequestsByName(original)

	for i := range desired {
		c := &desired[i]
		currentReqs, ok := currentIndex[c.Name]
		if !ok {
			continue
		}
		originalReqs := originalIndex[c.Name]

		if c.Resources.Requests == nil {
			c.Resources.Requests = make(corev1.ResourceList)
		}
		for _, res := range []corev1.ResourceName{corev1.ResourceCPU, corev1.ResourceMemory} {
			desiredVal, desiredHas := c.Resources.Requests[res]
			originalVal, originalHas := originalReqs[res]
			if desiredHas != originalHas || (desiredHas && !desiredVal.Equal(originalVal)) {
				// The CR declares a different value than what Koperator last applied, meaning
				// it changed since then. Let that new value take effect instead of preserving
				// whatever is live on currentPod.
				continue
			}
			if currentVal, ok := currentReqs[res]; ok {
				c.Resources.Requests[res] = currentVal
			} else {
				delete(c.Resources.Requests, res)
			}
		}
	}
}

// syncAffinities preserves preferred pod/node affinity terms that an external controller added
// to currentPod on top of whatever Koperator itself declared, while still letting the
// KafkaCluster CR remove terms it previously declared (per original) without them being
// resurrected from currentPod on the next reconcile.
func syncAffinities(desiredPod, currentPod, original *corev1.Pod) {
	syncPodAffinities(desiredPod, currentPod, original)
	syncNodeAffinities(desiredPod, currentPod, original)
}

func syncPodAffinities(desiredPod, currentPod, original *corev1.Pod) {
	currentTerms := podAffinityTerms(currentPod)
	if len(currentTerms) == 0 {
		return
	}
	var originalTerms []corev1.WeightedPodAffinityTerm
	if original != nil {
		originalTerms = podAffinityTerms(original)
	}

	var externallyAddedTerms []corev1.WeightedPodAffinityTerm
	for _, term := range currentTerms {
		if !containsWeightedPodAffinityTerm(originalTerms, term) {
			externallyAddedTerms = append(externallyAddedTerms, term)
		}
	}
	if len(externallyAddedTerms) == 0 {
		return
	}

	if desiredPod.Spec.Affinity == nil {
		desiredPod.Spec.Affinity = &corev1.Affinity{}
	}
	if desiredPod.Spec.Affinity.PodAffinity == nil {
		desiredPod.Spec.Affinity.PodAffinity = &corev1.PodAffinity{}
	}

	existingTerms := desiredPod.Spec.Affinity.PodAffinity.PreferredDuringSchedulingIgnoredDuringExecution
	for _, term := range externallyAddedTerms {
		if !containsWeightedPodAffinityTerm(existingTerms, term) {
			existingTerms = append(existingTerms, term)
		}
	}
	desiredPod.Spec.Affinity.PodAffinity.PreferredDuringSchedulingIgnoredDuringExecution = existingTerms
}

func containsWeightedPodAffinityTerm(terms []corev1.WeightedPodAffinityTerm, target corev1.WeightedPodAffinityTerm) bool {
	for _, t := range terms {
		if t.Weight == target.Weight && reflect.DeepEqual(t.PodAffinityTerm, target.PodAffinityTerm) {
			return true
		}
	}
	return false
}

func syncNodeAffinities(desiredPod, currentPod, original *corev1.Pod) {
	currentTerms := nodeAffinityTerms(currentPod)
	if len(currentTerms) == 0 {
		return
	}
	var originalTerms []corev1.PreferredSchedulingTerm
	if original != nil {
		originalTerms = nodeAffinityTerms(original)
	}

	var externallyAddedTerms []corev1.PreferredSchedulingTerm
	for _, term := range currentTerms {
		if !containsPreferredSchedulingTerm(originalTerms, term) {
			externallyAddedTerms = append(externallyAddedTerms, term)
		}
	}
	if len(externallyAddedTerms) == 0 {
		return
	}

	if desiredPod.Spec.Affinity == nil {
		desiredPod.Spec.Affinity = &corev1.Affinity{}
	}
	if desiredPod.Spec.Affinity.NodeAffinity == nil {
		desiredPod.Spec.Affinity.NodeAffinity = &corev1.NodeAffinity{}
	}

	existingTerms := desiredPod.Spec.Affinity.NodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution
	for _, term := range externallyAddedTerms {
		if !containsPreferredSchedulingTerm(existingTerms, term) {
			existingTerms = append(existingTerms, term)
		}
	}
	desiredPod.Spec.Affinity.NodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution = existingTerms
}

func containsPreferredSchedulingTerm(terms []corev1.PreferredSchedulingTerm, target corev1.PreferredSchedulingTerm) bool {
	for _, t := range terms {
		if t.Weight == target.Weight && reflect.DeepEqual(t.Preference, target.Preference) {
			return true
		}
	}
	return false
}
