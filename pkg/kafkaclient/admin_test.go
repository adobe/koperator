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

package kafkaclient

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kfake"
	"github.com/twmb/franz-go/pkg/kmsg"

	"github.com/banzaicloud/koperator/pkg/errorfactory"
)

func openTestClient(t *testing.T, cluster *kfake.Cluster) KafkaClient {
	t.Helper()
	client := New(&KafkaConfig{BrokerURI: cluster.ListenAddrs()[0], OperationTimeout: 5})
	require.NoError(t, client.Open())
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	return client
}

func TestAlterPerBrokerConfigReturnsValidationError(t *testing.T) {
	cluster := kfake.MustCluster(kfake.NumBrokers(2))
	t.Cleanup(cluster.Close)
	client := openTestClient(t, cluster)

	cluster.ControlKey(int16(kmsg.AlterConfigs), func(req kmsg.Request) (kmsg.Response, error, bool) {
		alter := req.(*kmsg.AlterConfigsRequest)
		resp := alter.ResponseKind().(*kmsg.AlterConfigsResponse)
		result := kmsg.NewAlterConfigsResponseResource()
		result.ResourceType = kmsg.ConfigResourceTypeBroker
		result.ResourceName = "1"
		if cluster.CurrentNode() == 1 && alter.ValidateOnly && len(alter.Resources) == 1 && alter.Resources[0].ResourceName == "1" {
			result.ErrorCode = kerr.InvalidConfig.Code
			result.ErrorMessage = kmsg.StringPtr("rejected test configuration")
		}
		resp.Resources = append(resp.Resources, result)
		return resp, nil, true
	})
	changes := map[string]*string{"log.cleaner.threads": kmsg.StringPtr("not-a-number")}
	require.ErrorContains(t, client.AlterPerBrokerConfig(1, changes, true), "rejected test configuration")
	var notReady errorfactory.BrokersNotReady
	require.ErrorAs(t, client.AlterPerBrokerConfig(404, changes, true), &notReady)
}

func TestListTopicsKeepsMetadataWhenConfigIsUnauthorized(t *testing.T) {
	cluster := kfake.MustCluster(kfake.NumBrokers(1), kfake.SeedTopics(1, "visible"))
	t.Cleanup(cluster.Close)
	client := openTestClient(t, cluster)
	cluster.ControlKey(int16(kmsg.DescribeConfigs), func(req kmsg.Request) (kmsg.Response, error, bool) {
		resp := req.ResponseKind().(*kmsg.DescribeConfigsResponse)
		resp.Resources = []kmsg.DescribeConfigsResponseResource{{
			ResourceType: kmsg.ConfigResourceTypeTopic, ResourceName: "visible", ErrorCode: kerr.TopicAuthorizationFailed.Code,
		}}
		return resp, nil, true
	})
	topics, err := client.ListTopics()
	require.NoError(t, err)
	require.Contains(t, topics, "visible")
	require.Equal(t, int32(1), topics["visible"].NumPartitions)
	require.Empty(t, topics["visible"].ConfigEntries)
}

func TestGetTopicReturnsConfigMetadataError(t *testing.T) {
	cluster := kfake.MustCluster(kfake.NumBrokers(1), kfake.SeedTopics(1, "visible"))
	t.Cleanup(cluster.Close)
	client := openTestClient(t, cluster)
	cluster.ControlKey(int16(kmsg.DescribeConfigs), func(req kmsg.Request) (kmsg.Response, error, bool) {
		resp := req.ResponseKind().(*kmsg.DescribeConfigsResponse)
		resp.Resources = []kmsg.DescribeConfigsResponseResource{{
			ResourceType: kmsg.ConfigResourceTypeTopic, ResourceName: "visible", ErrorCode: kerr.UnknownTopicOrPartition.Code,
		}}
		return resp, nil, true
	})
	detail, err := client.GetTopic("visible")
	require.ErrorIs(t, err, kerr.UnknownTopicOrPartition)
	require.Nil(t, detail)
}

func TestReplicaHealthIncludesInternalTopics(t *testing.T) {
	cluster := kfake.MustCluster(kfake.NumBrokers(1))
	t.Cleanup(cluster.Close)
	client := openTestClient(t, cluster)
	cluster.ControlKey(int16(kmsg.Metadata), func(req kmsg.Request) (kmsg.Response, error, bool) {
		if req.(*kmsg.MetadataRequest).Topics != nil {
			return nil, nil, false
		}
		cluster.KeepControl()
		resp := req.ResponseKind().(*kmsg.MetadataResponse)
		resp.Topics = []kmsg.MetadataResponseTopic{{
			Topic: kmsg.StringPtr("__internal-test"), IsInternal: true,
			Partitions: []kmsg.MetadataResponseTopicPartition{
				{Partition: 0, Replicas: []int32{0, 1, 2}, ISR: []int32{0}, OfflineReplicas: []int32{1}},
				{Partition: 1, Replicas: []int32{0, 1}, ISR: []int32{0}, OfflineReplicas: []int32{1}},
			},
		}}
		return resp, nil, true
	})
	offline, err := client.AllOfflineReplicas()
	require.NoError(t, err)
	require.Equal(t, []int32{1}, offline)
	outOfSync, err := client.OutOfSyncReplicas()
	require.NoError(t, err)
	require.Equal(t, []int32{1, 2}, outOfSync)
}
