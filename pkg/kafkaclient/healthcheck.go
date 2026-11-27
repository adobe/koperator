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

package kafkaclient

import (
	"fmt"
	"maps"
	"slices"

	"github.com/twmb/franz-go/pkg/kerr"
)

func (k *kafkaClient) AllOfflineReplicas() ([]int32, error) {
	offline, _, err := k.replicaHealth()
	return offline, err
}

func (k *kafkaClient) OutOfSyncReplicas() ([]int32, error) {
	_, outOfSync, err := k.replicaHealth()
	return outOfSync, err
}

func (k *kafkaClient) replicaHealth() (offline, outOfSync []int32, err error) {
	ctx, cancel := k.requestContext()
	defer cancel()
	metadata, err := topicMetadataRequest().RequestWith(ctx, k.client)
	if err != nil {
		return nil, nil, fmt.Errorf("could not fetch topics: %w", err)
	}
	offlineIDs := make(map[int32]struct{})
	outOfSyncIDs := make(map[int32]struct{})
	for _, topic := range metadata.Topics {
		if topic.Topic == nil {
			continue
		}
		name := *topic.Topic
		// Leader elections can still return replica health in the metadata.
		if err := kerr.ErrorForCode(topic.ErrorCode); err != nil && err != kerr.LeaderNotAvailable {
			return nil, nil, fmt.Errorf("could not fetch partitions for topic %q: %w", name, err)
		}
		for _, partition := range topic.Partitions {
			if partition.ErrorCode == kerr.ReplicaNotAvailable.Code {
				return nil, nil, fmt.Errorf("could not fetch replicas for topic %q partition %d: %w", name, partition.Partition, kerr.ReplicaNotAvailable)
			}
			for _, brokerID := range partition.OfflineReplicas {
				offlineIDs[brokerID] = struct{}{}
			}
			for _, brokerID := range partition.Replicas {
				if !slices.Contains(partition.ISR, brokerID) {
					outOfSyncIDs[brokerID] = struct{}{}
				}
			}
		}
	}

	return slices.Sorted(maps.Keys(offlineIDs)), slices.Sorted(maps.Keys(outOfSyncIDs)), nil
}
