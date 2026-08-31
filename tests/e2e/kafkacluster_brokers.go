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
	"fmt"
	"strings"

	"github.com/gruntwork-io/terratest/modules/k8s"

	"github.com/banzaicloud/koperator/api/v1beta1"
	properties "github.com/banzaicloud/koperator/properties/pkg"
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

// patchKafkaClusterStorageConfigs fetches the running KafkaCluster CR, applies mutate to the current
// spec.brokerConfigGroups[groupName].storageConfigs, and patches back only that field - leaving every
// other field untouched. Like patchKafkaClusterBrokers, this avoids the unrelated drift a full manifest
// re-apply can silently carry (the disk-removal test previously applied a whole simplekafkacluster_2disk.yaml
// just to drop two storageConfigs entries).
func patchKafkaClusterStorageConfigs(kubectlOptions k8s.KubectlOptions, clusterName, groupName string, mutate func([]v1beta1.StorageConfig) []v1beta1.StorageConfig) error {
	raw, err := runKubectlSilent(kubectlOptions, "get", "kafkacluster", clusterName, "-o", "json")
	if err != nil {
		return err
	}

	var current struct {
		Spec struct {
			BrokerConfigGroups map[string]struct {
				StorageConfigs []v1beta1.StorageConfig `json:"storageConfigs"`
			} `json:"brokerConfigGroups"`
		} `json:"spec"`
	}
	if err := json.Unmarshal([]byte(raw), &current); err != nil {
		return err
	}

	group, ok := current.Spec.BrokerConfigGroups[groupName]
	if !ok {
		return fmt.Errorf("broker config group %q not found in KafkaCluster %q", groupName, clusterName)
	}

	// A JSON merge patch replaces list-valued keys wholesale, so patching only
	// spec.brokerConfigGroups[groupName].storageConfigs swaps the storage list without
	// touching any other field of the group or the cluster.
	mergePatch, err := json.Marshal(map[string]any{
		"spec": map[string]any{
			"brokerConfigGroups": map[string]any{
				groupName: map[string]any{
					"storageConfigs": mutate(group.StorageConfigs),
				},
			},
		},
	})
	if err != nil {
		return err
	}

	_, err = runKubectlSilent(kubectlOptions, "patch", "kafkacluster", clusterName, "--type=merge", "-p", string(mergePatch))
	return err
}

// removeKafkaClusterStorageConfigs patches the given broker config group's storageConfigs to exclude
// the entries whose mountPath is in mountPathsToRemove. See patchKafkaClusterStorageConfigs.
func removeKafkaClusterStorageConfigs(kubectlOptions k8s.KubectlOptions, clusterName, groupName string, mountPathsToRemove ...string) error {
	remove := make(map[string]bool, len(mountPathsToRemove))
	for _, mountPath := range mountPathsToRemove {
		remove[mountPath] = true
	}
	return patchKafkaClusterStorageConfigs(kubectlOptions, clusterName, groupName, func(configs []v1beta1.StorageConfig) []v1beta1.StorageConfig {
		remaining := make([]v1beta1.StorageConfig, 0, len(configs))
		for _, config := range configs {
			if !remove[config.MountPath] {
				remaining = append(remaining, config)
			}
		}
		return remaining
	})
}

// patchKafkaClusterReadOnlyConfigAndRemoveDisks atomically, in a SINGLE merge patch, sets a read-only
// broker property (which forces a rolling restart) and drops the given mount paths from a broker config
// group's storageConfigs. Applying both in one patch makes the operator observe the config change and
// the disk removal in the same reconcile — the config-change + disk-removal deadlock scenario. Like the
// other patch helpers here, a targeted patch avoids the unrelated drift a whole-manifest re-apply can
// silently carry (see patchKafkaClusterStorageConfigs). The read-only property is upserted via the same
// properties library the operator uses to parse spec.readOnlyConfig, so an existing key is replaced
// (not duplicated).
func patchKafkaClusterReadOnlyConfigAndRemoveDisks(kubectlOptions k8s.KubectlOptions, clusterName, groupName, property string, mountPathsToRemove ...string) error {
	raw, err := runKubectlSilent(kubectlOptions, "get", "kafkacluster", clusterName, "-o", "json")
	if err != nil {
		return err
	}

	var current struct {
		Spec struct {
			ReadOnlyConfig     string `json:"readOnlyConfig"`
			BrokerConfigGroups map[string]struct {
				StorageConfigs []v1beta1.StorageConfig `json:"storageConfigs"`
			} `json:"brokerConfigGroups"`
		} `json:"spec"`
	}
	if err := json.Unmarshal([]byte(raw), &current); err != nil {
		return err
	}

	group, ok := current.Spec.BrokerConfigGroups[groupName]
	if !ok {
		return fmt.Errorf("broker config group %q not found in KafkaCluster %q", groupName, clusterName)
	}

	remove := make(map[string]bool, len(mountPathsToRemove))
	for _, mountPath := range mountPathsToRemove {
		remove[mountPath] = true
	}
	remaining := make([]v1beta1.StorageConfig, 0, len(group.StorageConfigs))
	for _, config := range group.StorageConfigs {
		if !remove[config.MountPath] {
			remaining = append(remaining, config)
		}
	}

	key, value, found := strings.Cut(property, "=")
	if !found {
		return fmt.Errorf("invalid read-only property %q: expected key=value", property)
	}
	readOnlyConfig, err := properties.NewFromString(current.Spec.ReadOnlyConfig)
	if err != nil {
		return fmt.Errorf("parsing current spec.readOnlyConfig failed: %w", err)
	}
	if err := readOnlyConfig.Set(strings.TrimSpace(key), value); err != nil {
		return fmt.Errorf("setting read-only property %q failed: %w", property, err)
	}

	// A JSON merge patch replaces list- and scalar-valued keys wholesale, so patching only
	// spec.readOnlyConfig and spec.brokerConfigGroups[groupName].storageConfigs changes exactly those
	// two fields and nothing else.
	mergePatch, err := json.Marshal(map[string]any{
		"spec": map[string]any{
			"readOnlyConfig": readOnlyConfig.String(),
			"brokerConfigGroups": map[string]any{
				groupName: map[string]any{
					"storageConfigs": remaining,
				},
			},
		},
	})
	if err != nil {
		return err
	}

	_, err = runKubectlSilent(kubectlOptions, "patch", "kafkacluster", clusterName, "--type=merge", "-p", string(mergePatch))
	return err
}
