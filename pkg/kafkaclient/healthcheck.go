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
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kmsg"

	"github.com/banzaicloud/koperator/pkg/errorfactory"
)

func (k *kafkaClient) ReplicaHealth() (offline, outOfSync []int32, err error) {
	ctx, cancel := k.requestContext()
	defer cancel()
	for attempt := 0; ; attempt++ {
		metadata, err := topicMetadataRequest().RequestWith(ctx, k.client)
		if err != nil {
			if attempt > 0 {
				return nil, nil, errorfactory.New(errorfactory.BrokersNotReady{}, err, "could not refresh incomplete replica health metadata")
			}
			return nil, nil, fmt.Errorf("could not fetch topics: %w", err)
		}
		offline, outOfSync, err := replicaHealthFromMetadata(metadata)
		switch {
		case errors.Is(err, kerr.UnknownTopicOrPartition), errors.Is(err, kerr.LeaderNotAvailable), errors.Is(err, kerr.ReplicaNotAvailable):
			// Retry the whole snapshot. A disappearing topic must not hide an unavailable replica elsewhere.
			if attempt == 0 {
				continue
			}
			return nil, nil, errorfactory.New(errorfactory.BrokersNotReady{}, err, "replica health metadata is incomplete")
		default:
			return offline, outOfSync, err
		}
	}
}

func replicaHealthFromMetadata(metadata *kmsg.MetadataResponse) (offline, outOfSync []int32, err error) {
	if len(metadata.Brokers) == 0 {
		return nil, nil, fmt.Errorf("metadata has no brokers: %w", kerr.ReplicaNotAvailable)
	}
	offlineIDs := make(map[int32]struct{})
	outOfSyncIDs := make(map[int32]struct{})
	for _, topic := range metadata.Topics {
		if topic.Topic == nil {
			return nil, nil, fmt.Errorf("topic metadata has no name: %w", kerr.UnknownTopicOrPartition)
		}
		name := *topic.Topic
		// Leader elections can still return replica health in the metadata.
		if err := kerr.ErrorForCode(topic.ErrorCode); err != nil && err != kerr.LeaderNotAvailable {
			return nil, nil, fmt.Errorf("could not fetch partitions for topic %q: %w", name, err)
		}
		if len(topic.Partitions) == 0 {
			return nil, nil, fmt.Errorf("topic %q has no partition metadata: %w", name, kerr.LeaderNotAvailable)
		}
		for _, partition := range topic.Partitions {
			if err := kerr.ErrorForCode(partition.ErrorCode); err != nil && err != kerr.LeaderNotAvailable && err != kerr.ListenerNotFound {
				return nil, nil, fmt.Errorf("could not fetch replicas for topic %q partition %d: %w", name, partition.Partition, err)
			}
			if len(partition.Replicas) == 0 {
				return nil, nil, fmt.Errorf("topic %q partition %d has no replica metadata: %w", name, partition.Partition, kerr.ReplicaNotAvailable)
			}
			if partition.ErrorCode == kerr.ListenerNotFound.Code && len(partition.OfflineReplicas) == 0 {
				return nil, nil, fmt.Errorf("topic %q partition %d has no offline replica metadata for a missing listener: %w", name, partition.Partition, kerr.ReplicaNotAvailable)
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
