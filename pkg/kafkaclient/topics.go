// Copyright © 2019 Cisco Systems, Inc. and/or its affiliates
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

package kafkaclient

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"

	"github.com/banzaicloud/koperator/pkg/errorfactory"
)

type TopicDetail struct {
	NumPartitions     int32
	ReplicationFactor int16
	ReplicaAssignment map[int32][]int32
	ConfigEntries     map[string]*string
}

type TopicMetadata struct {
	Name       string
	IsInternal bool
	Partitions []PartitionMetadata
}

type PartitionMetadata struct {
	ID              int32
	Leader          int32
	Replicas        []int32
	InSyncReplicas  []int32
	OfflineReplicas []int32
}

// CreateTopicOptions holds info about topic configuration
type CreateTopicOptions struct {
	Name              string
	Partitions        int32
	ReplicationFactor int16
	Config            map[string]*string
}

func topicMetadataRequest(names ...string) *kmsg.MetadataRequest {
	req := kmsg.NewPtrMetadataRequest()
	for _, name := range names {
		req.Topics = append(req.Topics, kmsg.MetadataRequestTopic{Topic: kmsg.StringPtr(name)})
	}
	return req
}

// ListTopics returns the details for all visible topics.
func (k *kafkaClient) ListTopics() (map[string]TopicDetail, error) {
	return k.readTopicDetails("")
}

// readTopicDetails reads all topics when topicName is empty.
func (k *kafkaClient) readTopicDetails(topicName string) (map[string]TopicDetail, error) {
	metadataRequest := topicMetadataRequest()
	if topicName != "" {
		metadataRequest = topicMetadataRequest(topicName)
	}
	ctx, cancel := k.requestContext()
	// Read fresh topic metadata and describe configs on the same broker.
	shard := k.client.RequestSharded(ctx, metadataRequest)[0]
	cancel()
	if shard.Err != nil {
		return nil, shard.Err
	}
	ctx, cancel = k.requestContext()
	defer cancel()
	topics, err := k.readTopicConfigs(ctx, topicName, shard)
	if !isBrokerConnectionError(err) {
		return topics, err
	}

	// Repeat both reads under the original configuration deadline. Another
	// broker's older metadata must not turn a known topic into a missing one.
	refreshed, retryErr := k.readTopicConfigs(ctx, topicName, k.client.RequestSharded(ctx, metadataRequest)[0])
	if retryErr == nil && topicName != "" && len(refreshed) == 0 {
		return nil, fmt.Errorf("could not confirm config for previously observed topic %q: %w", topicName, err)
	}
	return refreshed, retryErr
}

func (k *kafkaClient) readTopicConfigs(ctx context.Context, topicName string, shard kgo.ResponseShard) (map[string]TopicDetail, error) {
	if shard.Err != nil {
		return nil, shard.Err
	}
	metadata := shard.Resp.(*kmsg.MetadataResponse)
	topics := make(map[string]TopicDetail, len(metadata.Topics))
	req := kmsg.NewPtrDescribeConfigsRequest()
	for _, meta := range metadata.Topics {
		if meta.Topic == nil {
			continue
		}
		name := *meta.Topic
		if topicName != "" {
			if name != topicName || meta.ErrorCode == kerr.UnknownTopicOrPartition.Code {
				continue
			}
			if err := kerr.ErrorForCode(meta.ErrorCode); err != nil {
				return nil, err
			}
		}
		partitions, replicas := len(meta.Partitions), 0
		if partitions > 0 {
			replicas = len(meta.Partitions[0].Replicas)
		}
		if partitions > math.MaxInt32 || replicas > math.MaxInt16 {
			return nil, fmt.Errorf("topic %q has an invalid partition or replica count", name)
		}
		detail := TopicDetail{
			NumPartitions:     int32(partitions),
			ReplicationFactor: int16(replicas),
		}
		if partitions > 0 {
			detail.ReplicaAssignment = make(map[int32][]int32, partitions)
			for _, partition := range meta.Partitions {
				detail.ReplicaAssignment[partition.Partition] = partition.Replicas
			}
		}
		topics[name] = detail
		req.Resources = append(req.Resources, kmsg.DescribeConfigsRequestResource{
			ResourceType: kmsg.ConfigResourceTypeTopic, ResourceName: name,
		})
	}
	if len(req.Resources) == 0 {
		return topics, nil
	}
	configs, err := req.RequestWith(ctx, k.client.Broker(int(shard.Meta.NodeID)))
	if err != nil {
		return nil, err
	}
	for _, resource := range configs.Resources {
		detail, exists := topics[resource.ResourceName]
		if !exists {
			continue
		}
		// Keep visible topics when config authorization differs from metadata authorization.
		if err := brokerResponseError(resource.ErrorCode, resource.ErrorMessage); err != nil && resource.ErrorCode != kerr.TopicAuthorizationFailed.Code {
			return nil, fmt.Errorf("could not describe config for topic %q: %w", resource.ResourceName, err)
		}
		detail.ConfigEntries = make(map[string]*string)
		for _, config := range resource.Configs {
			if !config.IsDefault && config.Source != kmsg.ConfigSourceDefaultConfig && !config.IsSensitive {
				detail.ConfigEntries[config.Name] = config.Value
			}
		}
		topics[resource.ResourceName] = detail
	}
	for name, detail := range topics {
		if detail.ConfigEntries == nil {
			return nil, fmt.Errorf("missing describe config response for topic %q", name)
		}
	}
	return topics, nil
}

// GetTopic is used to check the existence, and retrieve details for a topic
func (k *kafkaClient) GetTopic(topicName string) (*TopicDetail, error) {
	if topicName == "" {
		return nil, nil
	}
	topics, err := k.readTopicDetails(topicName)
	if err != nil {
		return nil, err
	}
	if found, exists := topics[topicName]; exists {
		return &found, nil
	}
	return nil, nil
}

// DescribeTopic is used during status syncs to retrieve topic metadata
func (k *kafkaClient) DescribeTopic(topic string) (*TopicMetadata, error) {
	ctx, cancel := k.requestContext()
	defer cancel()
	metadata, err := topicMetadataRequest(topic).RequestWith(ctx, k.client)
	if err != nil {
		return nil, err
	}
	for _, meta := range metadata.Topics {
		if meta.Topic == nil || *meta.Topic != topic {
			continue
		}
		if err := kerr.ErrorForCode(meta.ErrorCode); err != nil {
			return nil, err
		}
		result := TopicMetadata{Name: topic, IsInternal: meta.IsInternal}
		for _, p := range meta.Partitions {
			result.Partitions = append(result.Partitions, PartitionMetadata{
				ID: p.Partition, Leader: p.Leader, Replicas: p.Replicas, InSyncReplicas: p.ISR, OfflineReplicas: p.OfflineReplicas,
			})
		}
		return &result, nil
	}
	return nil, kerr.UnknownTopicOrPartition
}

// CreateTopic creates a topic with the given options
func (k *kafkaClient) CreateTopic(opts *CreateTopicOptions) error {
	ctx, cancel := k.requestContext()
	defer cancel()
	result, err := k.admin.CreateTopic(ctx, opts.Partitions, opts.ReplicationFactor, opts.Config, opts.Name)
	if err != nil {
		if result.ErrMessage != "" {
			err = fmt.Errorf("%s: %w", result.ErrMessage, err)
		}
		return errorfactory.New(errorfactory.CreateTopicError{}, err, "failed to create topic")
	}
	return nil
}

// DeleteTopic deletes a topic - when wait is specified, the method will not
// return until the topic doesn't appear in the cluster topic list.
func (k *kafkaClient) DeleteTopic(topicName string, wait bool) error {
	ctx, cancel := k.requestContext()
	_, err := k.admin.DeleteTopic(ctx, topicName)
	cancel()
	if err != nil {
		return err
	}
	// TODO (tinyzimmer): Check here if we actually support topic deletion
	if wait {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			_, err := k.DescribeTopic(topicName)
			if errors.Is(err, kerr.UnknownTopicOrPartition) {
				return nil
			}
			if err != nil {
				return err
			}
			log.Info(fmt.Sprintf("Topic %s still going down for deletion", topicName))
		}
	}
	return nil
}

// EnsurePartitionCount will check if a partition increase is requested and apply
// the changed.
func (k *kafkaClient) EnsurePartitionCount(topic string, desired int32) (bool, error) {
	meta, err := k.DescribeTopic(topic)
	if err != nil {
		if errors.Is(err, kerr.UnknownTopicOrPartition) {
			return false, errorfactory.New(errorfactory.TopicNotFound{}, err, fmt.Sprintf("could not find topic %s", topic))
		}
		return false, errorfactory.New(errorfactory.BrokersRequestError{}, err, "error describing topics")
	}
	if int(desired) == len(meta.Partitions) {
		return false, nil
	}

	ctx, cancel := k.requestContext()
	defer cancel()
	results, err := k.admin.UpdatePartitions(ctx, int(desired), topic)
	if err != nil {
		return true, err
	}
	result, ok := results[topic]
	if !ok {
		return true, fmt.Errorf("missing create partitions response for topic %q", topic)
	}
	if result.Err != nil && result.ErrMessage != "" {
		return true, fmt.Errorf("%s: %w", result.ErrMessage, result.Err)
	}
	return true, result.Err
}

// EnsureTopicConfig is an idempotent call to ensure topic configuration overrides
func (k *kafkaClient) EnsureTopicConfig(topic string, desiredConf map[string]*string) error {
	return k.alterConfig(nil, kmsg.ConfigResourceTypeTopic, topic, desiredConf, false)
}
