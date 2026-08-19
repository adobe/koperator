// Copyright 2026 Adobe. All rights reserved.
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

//go:build e2e

package e2e

import (
	"encoding/json"

	"github.com/gruntwork-io/terratest/modules/k8s"

	"github.com/banzaicloud/koperator/api/v1beta1"
)

// patchKafkaClusterBrokers fetches the running KafkaCluster CR, applies mutate to its current spec.brokers,
// and patches only that field back - leaving every other field untouched. The broker-removal test previously
// applied a second manifest to change broker count; besides the removal itself, that manifest can silently
// carry unrelated drift (e.g. simplekafkacluster.yaml's headlessServiceEnabled: false, which raced the
// operator's headless Service teardown against an in-flight broker removal).
// Patching only spec.brokers cannot introduce that kind of unrelated drift.
func patchKafkaClusterBrokers(kubectlOptions k8s.KubectlOptions, clusterName string, mutate func([]v1beta1.Broker) []v1beta1.Broker) error {
	raw, err := runKubectlSilent(kubectlOptions, "get", "kafkacluster", clusterName, "-o", "json")
	if err != nil {
		return err
	}

	var current struct {
		Spec struct {
			Brokers []v1beta1.Broker `json:"brokers"`
		} `json:"spec"`
	}
	if err := json.Unmarshal([]byte(raw), &current); err != nil {
		return err
	}

	mergePatch, err := json.Marshal(map[string]any{
		"spec": map[string]any{
			"brokers": mutate(current.Spec.Brokers),
		},
	})
	if err != nil {
		return err
	}

	_, err = runKubectlSilent(kubectlOptions, "patch", "kafkacluster", clusterName, "--type=merge", "-p", string(mergePatch))
	return err
}

// removeKafkaClusterBrokers patches spec.brokers to exclude the given IDs. See patchKafkaClusterBrokers.
func removeKafkaClusterBrokers(kubectlOptions k8s.KubectlOptions, clusterName string, brokerIDsToRemove ...int32) error {
	remove := make(map[int32]bool, len(brokerIDsToRemove))
	for _, id := range brokerIDsToRemove {
		remove[id] = true
	}
	return patchKafkaClusterBrokers(kubectlOptions, clusterName, func(brokers []v1beta1.Broker) []v1beta1.Broker {
		remaining := make([]v1beta1.Broker, 0, len(brokers))
		for _, broker := range brokers {
			if !remove[broker.Id] {
				remaining = append(remaining, broker)
			}
		}
		return remaining
	})
}

// addKafkaClusterBroker patches spec.brokers to append newBroker. See patchKafkaClusterBrokers.
func addKafkaClusterBroker(kubectlOptions k8s.KubectlOptions, clusterName string, newBroker v1beta1.Broker) error {
	return patchKafkaClusterBrokers(kubectlOptions, clusterName, func(brokers []v1beta1.Broker) []v1beta1.Broker {
		return append(brokers, newBroker)
	})
}
