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
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kfake"
	"github.com/twmb/franz-go/pkg/kmsg"

	"github.com/banzaicloud/koperator/pkg/errorfactory"
)

func TestTopicAdministration(t *testing.T) {
	cluster := kfake.MustCluster(kfake.NumBrokers(1), kfake.AllowAutoTopicCreation())
	t.Cleanup(cluster.Close)
	client := openTestClient(t, cluster)

	const topic = "test-topic"
	detail, err := client.GetTopic(topic)
	require.NoError(t, err)
	require.Nil(t, detail)
	_, err = client.DescribeTopic(topic)
	require.ErrorIs(t, err, kerr.UnknownTopicOrPartition)
	var notFound errorfactory.TopicNotFound
	_, err = client.EnsurePartitionCount(topic, 2)
	require.ErrorAs(t, err, &notFound)

	opts := &CreateTopicOptions{
		Name: topic, Partitions: 1, ReplicationFactor: 1,
		Config: map[string]*string{"retention.ms": kmsg.StringPtr("60000")},
	}
	require.NoError(t, client.CreateTopic(opts))
	var createErr errorfactory.CreateTopicError
	require.ErrorAs(t, client.CreateTopic(opts), &createErr)

	detail, err = client.GetTopic(topic)
	require.NoError(t, err)
	require.NotNil(t, detail)
	require.Equal(t, int32(1), detail.NumPartitions)
	require.Equal(t, int16(1), detail.ReplicationFactor)
	require.Equal(t, map[string]*string{"retention.ms": kmsg.StringPtr("60000")}, detail.ConfigEntries)

	changed, err := client.EnsurePartitionCount(topic, 1)
	require.NoError(t, err)
	require.False(t, changed)
	changed, err = client.EnsurePartitionCount(topic, 2)
	require.NoError(t, err)
	require.True(t, changed)
	require.NoError(t, client.EnsureTopicConfig(topic, map[string]*string{"cleanup.policy": kmsg.StringPtr("compact")}))

	detail, err = client.GetTopic(topic)
	require.NoError(t, err)
	require.NotNil(t, detail)
	require.Equal(t, int32(2), detail.NumPartitions)
	require.Equal(t, map[string]*string{"cleanup.policy": kmsg.StringPtr("compact")}, detail.ConfigEntries)
	require.NoError(t, client.EnsureTopicConfig(topic, map[string]*string{}))
	detail, err = client.GetTopic(topic)
	require.NoError(t, err)
	require.NotNil(t, detail)
	require.Empty(t, detail.ConfigEntries)

	require.NoError(t, client.DeleteTopic(topic, true))
	detail, err = client.GetTopic(topic)
	require.NoError(t, err)
	require.Nil(t, detail)
}

func TestEnsurePartitionCountReturnsBrokerRejection(t *testing.T) {
	const topic = "test-topic"
	cluster := kfake.MustCluster(kfake.NumBrokers(1), kfake.SeedTopics(1, topic))
	t.Cleanup(cluster.Close)
	client := openTestClient(t, cluster)
	cluster.ControlKey(int16(kmsg.CreatePartitions), func(req kmsg.Request) (kmsg.Response, error, bool) {
		resp := req.ResponseKind().(*kmsg.CreatePartitionsResponse)
		resp.Topics = []kmsg.CreatePartitionsResponseTopic{{
			Topic: topic, ErrorCode: kerr.TopicAuthorizationFailed.Code,
			ErrorMessage: kmsg.StringPtr("partition change rejected by broker"),
		}}
		return resp, nil, true
	})
	changed, err := client.EnsurePartitionCount(topic, 2)
	require.True(t, changed)
	require.ErrorIs(t, err, kerr.TopicAuthorizationFailed)
	require.ErrorContains(t, err, "partition change rejected by broker")
}

func TestCreateTopicReturnsBrokerMessage(t *testing.T) {
	const topic = "test-topic"
	cluster := kfake.MustCluster(kfake.NumBrokers(1))
	t.Cleanup(cluster.Close)
	client := openTestClient(t, cluster)
	cluster.ControlKey(int16(kmsg.CreateTopics), func(req kmsg.Request) (kmsg.Response, error, bool) {
		resp := req.ResponseKind().(*kmsg.CreateTopicsResponse)
		resp.Topics = []kmsg.CreateTopicsResponseTopic{{
			Topic: topic, ErrorCode: kerr.InvalidConfig.Code,
			ErrorMessage: kmsg.StringPtr("topic configuration rejected by broker"),
		}}
		return resp, nil, true
	})
	err := client.CreateTopic(&CreateTopicOptions{Name: topic, Partitions: 1, ReplicationFactor: 1})
	var createErr errorfactory.CreateTopicError
	require.ErrorAs(t, err, &createErr)
	require.ErrorContains(t, err, "topic configuration rejected by broker")
}

func TestTopicMetadataRejection(t *testing.T) {
	const topic = "test-topic"
	cluster := kfake.MustCluster(kfake.NumBrokers(1))
	t.Cleanup(cluster.Close)
	client := openTestClient(t, cluster)
	broker := testBrokerMetadata(t, cluster)
	cluster.ControlKey(int16(kmsg.Metadata), func(req kmsg.Request) (kmsg.Response, error, bool) {
		for _, requested := range req.(*kmsg.MetadataRequest).Topics {
			if requested.Topic != nil && *requested.Topic == topic {
				cluster.KeepControl()
				resp := req.ResponseKind().(*kmsg.MetadataResponse)
				resp.Brokers, resp.ControllerID = []kmsg.MetadataResponseBroker{broker}, broker.NodeID
				resp.Topics = []kmsg.MetadataResponseTopic{{
					Topic: kmsg.StringPtr(topic), ErrorCode: kerr.TopicAuthorizationFailed.Code,
				}}
				return resp, nil, true
			}
		}
		return nil, nil, false
	})
	t.Run("describe returns rejection", func(t *testing.T) {
		metadata, err := client.DescribeTopic(topic)
		require.ErrorIs(t, err, kerr.TopicAuthorizationFailed)
		require.Nil(t, metadata)
	})
	t.Run("partition update stops before writing", func(t *testing.T) {
		var writes atomic.Int32
		cluster.ControlKey(int16(kmsg.CreatePartitions), func(kmsg.Request) (kmsg.Response, error, bool) {
			writes.Add(1)
			return nil, nil, false
		})
		changed, err := client.EnsurePartitionCount(topic, 2)
		var requestErr errorfactory.BrokersRequestError
		require.ErrorAs(t, err, &requestErr)
		require.ErrorIs(t, err, kerr.TopicAuthorizationFailed)
		require.False(t, changed)
		require.Zero(t, writes.Load())
	})
}

func TestEnsureTopicConfigReturnsBrokerRejection(t *testing.T) {
	const topic = "test-topic"
	cluster := kfake.MustCluster(kfake.NumBrokers(1), kfake.SeedTopics(1, topic))
	t.Cleanup(cluster.Close)
	client := openTestClient(t, cluster)
	cluster.ControlKey(int16(kmsg.AlterConfigs), func(req kmsg.Request) (kmsg.Response, error, bool) {
		resp := req.ResponseKind().(*kmsg.AlterConfigsResponse)
		resp.Resources = []kmsg.AlterConfigsResponseResource{{
			ResourceName: topic, ResourceType: kmsg.ConfigResourceTypeTopic, ErrorCode: kerr.InvalidConfig.Code,
		}}
		return resp, nil, true
	})
	err := client.EnsureTopicConfig(topic, map[string]*string{"retention.ms": kmsg.StringPtr("invalid")})
	require.ErrorIs(t, err, kerr.InvalidConfig)
}

func TestDeleteTopicReturnsBrokerRejectionBeforePolling(t *testing.T) {
	const topic = "test-topic"
	cluster := kfake.MustCluster(kfake.NumBrokers(1))
	t.Cleanup(cluster.Close)
	client := openTestClient(t, cluster)
	cluster.ControlKey(int16(kmsg.DeleteTopics), func(req kmsg.Request) (kmsg.Response, error, bool) {
		resp := req.ResponseKind().(*kmsg.DeleteTopicsResponse)
		resp.Topics = []kmsg.DeleteTopicsResponseTopic{{
			Topic: kmsg.StringPtr(topic), ErrorCode: kerr.TopicDeletionDisabled.Code,
		}}
		return resp, nil, true
	})
	var polls atomic.Int32
	cluster.ControlKey(int16(kmsg.Metadata), func(req kmsg.Request) (kmsg.Response, error, bool) {
		for _, requested := range req.(*kmsg.MetadataRequest).Topics {
			if requested.Topic != nil && *requested.Topic == topic {
				polls.Add(1)
			}
		}
		return nil, nil, false
	})
	require.ErrorIs(t, client.DeleteTopic(topic, true), kerr.TopicDeletionDisabled)
	require.Zero(t, polls.Load())
}

func TestDeleteTopicWithoutWaiting(t *testing.T) {
	const topic = "test-topic"
	cluster := kfake.MustCluster(kfake.NumBrokers(1), kfake.SeedTopics(1, topic))
	t.Cleanup(cluster.Close)
	client := openTestClient(t, cluster)
	var deletes, polls atomic.Int32
	cluster.ControlKey(int16(kmsg.DeleteTopics), func(kmsg.Request) (kmsg.Response, error, bool) {
		deletes.Add(1)
		return nil, nil, false
	})
	cluster.ControlKey(int16(kmsg.Metadata), func(req kmsg.Request) (kmsg.Response, error, bool) {
		for _, requested := range req.(*kmsg.MetadataRequest).Topics {
			if requested.Topic != nil && *requested.Topic == topic {
				polls.Add(1)
			}
		}
		return nil, nil, false
	})
	require.NoError(t, client.DeleteTopic(topic, false))
	require.Equal(t, int32(1), deletes.Load())
	require.Zero(t, polls.Load())
}
