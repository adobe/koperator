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

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"

	"github.com/banzaicloud/koperator/api/v1beta1"
)

// externalResourceBaselineAnnotation records, on each broker Pod, the CPU/memory requests and
// preferred affinity terms that Koperator itself last declared for that Pod - as opposed to
// whatever an external controller (for example a mutating admission webhook such as ScaleOps)
// may have layered on top of the live Pod afterwards. It intentionally does not reuse
// k8s-objectmatcher's own "last-applied" annotation: that annotation is snapshotted from
// desiredPod *after* syncResourceRequests/syncAffinities run, so it would end up recording the
// externally-applied values instead of Koperator's own intent, making it useless for telling
// the two apart on the next reconcile.
const externalResourceBaselineAnnotation = "banzaicloud.io/external-resource-baseline"

// externalResourceBaseline is the payload stored under externalResourceBaselineAnnotation.
type externalResourceBaseline struct {
	ContainerRequests     map[string]corev1.ResourceList  `json:"containerRequests,omitempty"`
	InitContainerRequests map[string]corev1.ResourceList  `json:"initContainerRequests,omitempty"`
	PodAffinityTerms      []corev1.WeightedPodAffinityTerm `json:"podAffinityTerms,omitempty"`
	NodeAffinityTerms     []corev1.PreferredSchedulingTerm `json:"nodeAffinityTerms,omitempty"`
}

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

// buildExternalResourceBaseline captures the CPU/memory requests and preferred affinity terms
// Koperator itself declared on pod, for later comparison against a freshly-computed desired pod.
// Callers must pass a pod that has not been touched by syncResourceRequests/syncAffinities, so
// the recorded baseline reflects only Koperator's own intent.
func buildExternalResourceBaseline(pod *corev1.Pod) *externalResourceBaseline {
	baseline := &externalResourceBaseline{
		ContainerRequests:     containerRequestsByName(pod.Spec.Containers),
		InitContainerRequests: containerRequestsByName(pod.Spec.InitContainers),
		PodAffinityTerms:      podAffinityTerms(pod),
		NodeAffinityTerms:     nodeAffinityTerms(pod),
	}
	return baseline
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

// getExternalResourceBaseline reads back the baseline last recorded on pod by
// setExternalResourceBaseline, or returns nil if none is present yet (for example before
// externalResourceManagementEnabled was turned on, or on a pod created before this feature
// existed).
func getExternalResourceBaseline(pod *corev1.Pod) (*externalResourceBaseline, error) {
	raw, ok := pod.Annotations[externalResourceBaselineAnnotation]
	if !ok {
		return nil, nil
	}
	baseline := &externalResourceBaseline{}
	if err := json.Unmarshal([]byte(raw), baseline); err != nil {
		return nil, err
	}
	return baseline, nil
}

// setExternalResourceBaseline stamps baseline onto pod's annotations.
func setExternalResourceBaseline(pod *corev1.Pod, baseline *externalResourceBaseline) error {
	raw, err := json.Marshal(baseline)
	if err != nil {
		return err
	}
	if pod.Annotations == nil {
		pod.Annotations = make(map[string]string)
	}
	pod.Annotations[externalResourceBaselineAnnotation] = string(raw)
	return nil
}

// syncResourceRequests overwrites CPU and memory requests in desiredPod's containers with the
// values from currentPod, but only for requests that Koperator's own baseline shows as
// unchanged - so a genuine change to the KafkaCluster's resource requests still takes effect
// instead of being silently discarded in favor of externally-applied drift.
func syncResourceRequests(desiredPod, currentPod *corev1.Pod, baseline *externalResourceBaseline) {
	var baselineContainers, baselineInitContainers map[string]corev1.ResourceList
	if baseline != nil {
		baselineContainers = baseline.ContainerRequests
		baselineInitContainers = baseline.InitContainerRequests
	}
	syncContainerResourceRequests(desiredPod.Spec.Containers, currentPod.Spec.Containers, baselineContainers)
	syncContainerResourceRequests(desiredPod.Spec.InitContainers, currentPod.Spec.InitContainers, baselineInitContainers)
}

func syncContainerResourceRequests(desired, current []corev1.Container, baseline map[string]corev1.ResourceList) {
	currentIndex := containerRequestsByName(current)

	for i := range desired {
		c := &desired[i]
		currentReqs, ok := currentIndex[c.Name]
		if !ok {
			continue
		}
		baselineReqs := baseline[c.Name]

		if c.Resources.Requests == nil {
			c.Resources.Requests = make(corev1.ResourceList)
		}
		for _, res := range []corev1.ResourceName{corev1.ResourceCPU, corev1.ResourceMemory} {
			desiredVal, desiredHas := c.Resources.Requests[res]
			baselineVal, baselineHas := baselineReqs[res]
			if desiredHas != baselineHas || (desiredHas && !desiredVal.Equal(baselineVal)) {
				// The CR declares a different value than our recorded baseline, meaning it
				// changed since we last recorded it. Let that new value take effect instead
				// of preserving whatever is live on currentPod.
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
// KafkaCluster CR remove terms it previously declared (tracked via baseline) without them being
// resurrected from currentPod on the next reconcile.
func syncAffinities(desiredPod, currentPod *corev1.Pod, baseline *externalResourceBaseline) {
	syncPodAffinities(desiredPod, currentPod, baseline)
	syncNodeAffinities(desiredPod, currentPod, baseline)
}

func syncPodAffinities(desiredPod, currentPod *corev1.Pod, baseline *externalResourceBaseline) {
	currentTerms := podAffinityTerms(currentPod)
	if len(currentTerms) == 0 {
		return
	}
	var baselineTerms []corev1.WeightedPodAffinityTerm
	if baseline != nil {
		baselineTerms = baseline.PodAffinityTerms
	}

	var externallyAddedTerms []corev1.WeightedPodAffinityTerm
	for _, term := range currentTerms {
		if !containsWeightedPodAffinityTerm(baselineTerms, term) {
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

func syncNodeAffinities(desiredPod, currentPod *corev1.Pod, baseline *externalResourceBaseline) {
	currentTerms := nodeAffinityTerms(currentPod)
	if len(currentTerms) == 0 {
		return
	}
	var baselineTerms []corev1.PreferredSchedulingTerm
	if baseline != nil {
		baselineTerms = baseline.NodeAffinityTerms
	}

	var externallyAddedTerms []corev1.PreferredSchedulingTerm
	for _, term := range currentTerms {
		if !containsPreferredSchedulingTerm(baselineTerms, term) {
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
