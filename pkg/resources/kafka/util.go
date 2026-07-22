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
	"sort"

	"emperror.dev/errors"
	"github.com/google/uuid"
	json "github.com/json-iterator/go"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/strategicpatch"

	"github.com/banzaicloud/k8s-objectmatcher/patch"

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

// podSpecIntentChanged reports whether koperator's own desired pod spec has
// changed since it last applied it. It diffs the last-applied-configuration
// annotation recorded on the current pod (original) against the freshly
// generated desired pod (modified). The live pod (current) is deliberately
// NOT part of the comparison.
//
// This is the mechanism that lets koperator coexist with any admission
// controller (autoscalers, mutating webhooks, the node lifecycle controller):
// a field is reconciled only when koperator's own CR-derived value differs
// from what koperator last applied. Mutations made to the running pod by other
// actors never enter the decision, so they can't trigger a rolling restart,
// while intentional changes made through the KafkaCluster CR always do. It
// therefore also handles preferred affinities generically — including the case
// where the operator intentionally edits a soft affinity in the CR, which the
// previous approach (stripping preferred affinities from the diff) silently
// swallowed.
//
// The returned patch is the two-way strategic merge patch from original to
// modified; it is only used as a change signal (and for logging), since a
// rolling upgrade recreates the pod rather than patching it in place.
func podSpecIntentChanged(currentPod, desiredPod *corev1.Pod) (bool, []byte, error) {
	original, err := patch.DefaultAnnotator.GetOriginalConfiguration(currentPod)
	if err != nil {
		return false, nil, errors.WrapIf(err, "could not read last-applied configuration from current pod")
	}

	modified, err := json.ConfigCompatibleWithStandardLibrary.Marshal(desiredPod)
	if err != nil {
		return false, nil, errors.WrapIf(err, "could not marshal desired pod")
	}
	// Mirror how the last-applied annotation is produced so absent fields on one
	// side don't masquerade as diffs.
	if modified, _, err = patch.DeleteNullInJson(modified); err != nil {
		return false, nil, errors.WrapIf(err, "could not clean desired pod json")
	}

	patchBytes, err := strategicpatch.CreateTwoWayMergePatch(original, modified, corev1.Pod{})
	if err != nil {
		return false, nil, errors.WrapIf(err, "could not create two-way merge patch")
	}
	if string(patchBytes) == "{}" {
		return false, patchBytes, nil
	}

	// A $setElementOrder directive can make the patch non-empty without any
	// actual change; confirm by applying it and re-diffing (mirrors
	// k8s-objectmatcher's PatchMaker.Calculate).
	patched, err := strategicpatch.StrategicMergePatch(original, patchBytes, corev1.Pod{})
	if err != nil {
		return false, nil, errors.WrapIf(err, "could not apply patch to last-applied configuration")
	}
	patchBytes, err = strategicpatch.CreateTwoWayMergePatch(original, patched, corev1.Pod{})
	if err != nil {
		return false, nil, errors.WrapIf(err, "could not recompute two-way merge patch")
	}
	return string(patchBytes) != "{}", patchBytes, nil
}
