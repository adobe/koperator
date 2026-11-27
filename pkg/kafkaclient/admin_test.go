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
	"context"
	"io"
	"net"
	"strconv"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kfake"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"

	"github.com/banzaicloud/koperator/pkg/errorfactory"
)

func openTestClient(t *testing.T, cluster *kfake.Cluster) KafkaClient {
	t.Helper()
	client := New(&KafkaConfig{BrokerURI: cluster.ListenAddrs()[0]})
	require.NoError(t, client.Open())
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	return client
}

func testBrokerMetadata(t *testing.T, cluster *kfake.Cluster) kmsg.MetadataResponseBroker {
	t.Helper()
	host, port, err := net.SplitHostPort(cluster.ListenAddrs()[0])
	require.NoError(t, err)
	portNumber, err := strconv.ParseInt(port, 10, 32)
	require.NoError(t, err)
	return kmsg.MetadataResponseBroker{NodeID: 0, Host: host, Port: int32(portNumber)}
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

func TestConfigReplacementDoesNotReplayLostReply(t *testing.T) {
	for _, tc := range []struct {
		name  string
		alter func(KafkaClient) error
	}{
		{"per broker", func(c KafkaClient) error { return c.AlterPerBrokerConfig(0, nil, false) }},
		{"cluster wide", func(c KafkaClient) error { return c.AlterClusterWideConfig(nil, false) }},
		{"topic", func(c KafkaClient) error { return c.EnsureTopicConfig("visible", nil) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cluster := kfake.MustCluster(kfake.NumBrokers(1), kfake.SeedTopics(1, "visible"))
			t.Cleanup(cluster.Close)
			client := openTestClient(t, cluster)
			_, err := client.DescribePerBrokerConfig(0, nil)
			require.NoError(t, err)

			var writes atomic.Int32
			cluster.ControlKey(int16(kmsg.AlterConfigs), func(kmsg.Request) (kmsg.Response, error, bool) {
				cluster.KeepControl()
				if writes.Add(1) == 1 {
					return nil, io.EOF, true
				}
				return nil, nil, false
			})
			require.ErrorIs(t, tc.alter(client), io.EOF)
			require.Equal(t, int32(1), writes.Load())
		})
	}
}

func TestDescribePerBrokerConfigWithEmptyKeys(t *testing.T) {
	cluster := kfake.MustCluster(kfake.NumBrokers(1))
	t.Cleanup(cluster.Close)
	client := openTestClient(t, cluster)

	all, err := client.DescribePerBrokerConfig(0, nil)
	require.NoError(t, err)
	require.NotEmpty(t, all)
	fromEmpty, err := client.DescribePerBrokerConfig(0, []string{})
	require.NoError(t, err)
	require.ElementsMatch(t, all, fromEmpty)
}

func TestDescribePerBrokerConfigReturnsBrokerRejection(t *testing.T) {
	cluster := kfake.MustCluster(kfake.NumBrokers(1))
	t.Cleanup(cluster.Close)
	client := openTestClient(t, cluster)
	cluster.ControlKey(int16(kmsg.DescribeConfigs), func(req kmsg.Request) (kmsg.Response, error, bool) {
		resp := req.ResponseKind().(*kmsg.DescribeConfigsResponse)
		resp.Resources = []kmsg.DescribeConfigsResponseResource{{
			ResourceType: kmsg.ConfigResourceTypeBroker, ResourceName: "0", ErrorCode: kerr.ClusterAuthorizationFailed.Code,
		}}
		return resp, nil, true
	})
	entries, err := client.DescribePerBrokerConfig(0, []string{"max.connections"})
	require.ErrorIs(t, err, kerr.ClusterAuthorizationFailed)
	require.Nil(t, entries)
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

func TestGetTopicIgnoresUnrelatedTopicDeletion(t *testing.T) {
	cluster := kfake.MustCluster(kfake.NumBrokers(1), kfake.SeedTopics(1, "visible", "disappearing"))
	t.Cleanup(cluster.Close)
	client := openTestClient(t, cluster)
	cluster.ControlKey(int16(kmsg.DescribeConfigs), func(req kmsg.Request) (kmsg.Response, error, bool) {
		resp := req.ResponseKind().(*kmsg.DescribeConfigsResponse)
		for _, resource := range req.(*kmsg.DescribeConfigsRequest).Resources {
			result := kmsg.NewDescribeConfigsResponseResource()
			result.ResourceType, result.ResourceName = resource.ResourceType, resource.ResourceName
			if result.ResourceName == "disappearing" {
				result.ErrorCode = kerr.UnknownTopicOrPartition.Code
			}
			resp.Resources = append(resp.Resources, result)
		}
		return resp, nil, true
	})
	detail, err := client.GetTopic("visible")
	require.NoError(t, err)
	require.NotNil(t, detail)
	require.Equal(t, int32(1), detail.NumPartitions)
}

func TestGetTopicRefreshesMetadataAfterLostConfigReply(t *testing.T) {
	for _, tc := range []struct {
		name              string
		missingAfterRetry bool
	}{
		{name: "topic still visible"},
		{name: "stale metadata reports topic missing", missingAfterRetry: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const topic = "visible"
			cluster := kfake.MustCluster(kfake.NumBrokers(1), kfake.SeedTopics(1, topic))
			t.Cleanup(cluster.Close)
			client := openTestClient(t, cluster)
			broker := testBrokerMetadata(t, cluster)
			var metadataReads atomic.Int32
			cluster.ControlKey(int16(kmsg.Metadata), func(req kmsg.Request) (kmsg.Response, error, bool) {
				if len(req.(*kmsg.MetadataRequest).Topics) != 1 {
					return nil, nil, false
				}
				if metadataReads.Add(1) == 1 || !tc.missingAfterRetry {
					return nil, nil, false
				}
				resp := req.ResponseKind().(*kmsg.MetadataResponse)
				resp.Brokers, resp.ControllerID = []kmsg.MetadataResponseBroker{broker}, broker.NodeID
				resp.Topics = []kmsg.MetadataResponseTopic{{Topic: kmsg.StringPtr(topic), ErrorCode: kerr.UnknownTopicOrPartition.Code}}
				return resp, nil, true
			})
			cluster.ControlKey(int16(kmsg.DescribeConfigs), func(kmsg.Request) (kmsg.Response, error, bool) {
				return nil, io.EOF, true
			})

			detail, err := client.GetTopic(topic)
			if tc.missingAfterRetry {
				require.ErrorIs(t, err, io.EOF)
				require.Nil(t, detail)
			} else {
				require.NoError(t, err)
				require.NotNil(t, detail)
			}
			require.Equal(t, int32(2), metadataReads.Load())
		})
	}
}

func TestTopicLookupReadsConfigFromMetadataBroker(t *testing.T) {
	const topic = "visible"
	cluster := kfake.MustCluster(kfake.NumBrokers(2), kfake.SeedTopics(1, topic))
	t.Cleanup(cluster.Close)
	addrs := cluster.ListenAddrs()
	client := New(&KafkaConfig{BrokerURI: addrs[0]}).(*kafkaClient)
	var err error
	client.client, err = kgo.NewClient(kgo.SeedBrokers(addrs[0]))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	host, port, err := net.SplitHostPort(addrs[1])
	require.NoError(t, err)
	portNumber, err := strconv.ParseInt(port, 10, 32)
	require.NoError(t, err)
	// The seed answers metadata but advertises only broker 1. An unpinned
	// configuration request then goes to broker 1 instead of the seed.
	cluster.ControlKey(int16(kmsg.Metadata), func(req kmsg.Request) (kmsg.Response, error, bool) {
		resp := req.ResponseKind().(*kmsg.MetadataResponse)
		resp.Brokers = []kmsg.MetadataResponseBroker{{NodeID: 1, Host: host, Port: int32(portNumber)}}
		resp.ControllerID = 1
		resp.Topics = []kmsg.MetadataResponseTopic{{
			Topic: kmsg.StringPtr(topic), Partitions: []kmsg.MetadataResponseTopicPartition{{Replicas: []int32{1}}},
		}}
		return resp, nil, true
	})
	var configBroker atomic.Int32
	configBroker.Store(-1)
	cluster.ControlKey(int16(kmsg.DescribeConfigs), func(kmsg.Request) (kmsg.Response, error, bool) {
		cluster.KeepControl()
		configBroker.Store(cluster.CurrentNode())
		return nil, nil, false
	})
	detail, err := client.GetTopic(topic)
	require.NoError(t, err)
	require.NotNil(t, detail)
	require.Equal(t, int32(0), configBroker.Load())

	req := kmsg.NewPtrDescribeConfigsRequest()
	req.Resources = []kmsg.DescribeConfigsRequestResource{{ResourceType: kmsg.ConfigResourceTypeTopic, ResourceName: topic}}
	_, err = req.RequestWith(t.Context(), client.client)
	require.NoError(t, err)
	require.Equal(t, int32(1), configBroker.Load())
}

func TestTopicLookupHasSeparateReadDeadlines(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		cluster, client, dialer := newTimedKafka(t, 2, kfake.NumBrokers(1), kfake.SeedTopics(1, "visible"))
		require.NoError(t, client.open(dialer))
		cluster.ControlKey(int16(kmsg.Metadata), func(kmsg.Request) (kmsg.Response, error, bool) {
			time.Sleep(1200 * time.Millisecond)
			return nil, nil, false
		})
		cluster.ControlKey(int16(kmsg.DescribeConfigs), func(req kmsg.Request) (kmsg.Response, error, bool) {
			time.Sleep(1200 * time.Millisecond)
			resp := req.ResponseKind().(*kmsg.DescribeConfigsResponse)
			resp.Resources = []kmsg.DescribeConfigsResponseResource{{ResourceName: "visible", ResourceType: kmsg.ConfigResourceTypeTopic}}
			return resp, nil, true
		})
		detail, err := client.GetTopic("visible")
		require.NoError(t, err)
		require.NotNil(t, detail)
	})
}

func TestCreateTopicWaitsForSlowBrokerReply(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		cluster, client, dialer := newTimedKafka(t, 0, kfake.NumBrokers(1))
		require.NoError(t, client.open(dialer))
		var brokerTimeout atomic.Int32
		cluster.ControlKey(int16(kmsg.CreateTopics), func(req kmsg.Request) (kmsg.Response, error, bool) {
			brokerTimeout.Store(req.(*kmsg.CreateTopicsRequest).TimeoutMillis)
			time.Sleep(5250 * time.Millisecond)
			return nil, nil, false
		})
		require.NoError(t, client.CreateTopic(&CreateTopicOptions{Name: "slow-reply", Partitions: 1, ReplicationFactor: 1}))
		require.Equal(t, int32(3000), brokerTimeout.Load())
	})
}

func TestCreateTopicKeepsBrokerTimeoutWhenClientBudgetAllows(t *testing.T) {
	cluster := kfake.MustCluster(kfake.NumBrokers(1))
	t.Cleanup(cluster.Close)
	client := New(&KafkaConfig{BrokerURI: cluster.ListenAddrs()[0], OperationTimeout: 4})
	require.NoError(t, client.Open())
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	var brokerTimeout atomic.Int32
	cluster.ControlKey(int16(kmsg.CreateTopics), func(req kmsg.Request) (kmsg.Response, error, bool) {
		brokerTimeout.Store(req.(*kmsg.CreateTopicsRequest).TimeoutMillis)
		return nil, nil, false
	})
	require.NoError(t, client.CreateTopic(&CreateTopicOptions{Name: "test", Partitions: 1, ReplicationFactor: 1}))
	require.Equal(t, int32(3000), brokerTimeout.Load())
}

func TestAdminDeadlineIncludesColdBrokerNegotiation(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		// Open uses broker 0 for bootstrap and broker 2 as controller, leaving broker 1 cold.
		cluster, client, dialer := newTimedKafka(t, 1, kfake.NumBrokers(3))
		require.NoError(t, client.open(dialer))
		var negotiations atomic.Int32
		release := make(chan struct{})
		defer func() {
			close(release)
			synctest.Wait()
		}()
		cluster.ControlKey(int16(kmsg.ApiVersions), func(req kmsg.Request) (kmsg.Response, error, bool) {
			if cluster.CurrentNode() != 1 {
				return nil, nil, false
			}
			cluster.KeepControl()
			if negotiations.Add(1) == 1 {
				time.Sleep(800 * time.Millisecond)
				resp := req.ResponseKind().(*kmsg.ApiVersionsResponse)
				resp.Version = 0
				resp.ErrorCode = kerr.UnsupportedVersion.Code
				return resp, nil, true
			}
			<-release
			return nil, nil, false
		})
		started := time.Now()
		_, err := client.DescribePerBrokerConfig(1, nil)
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.Equal(t, time.Second, time.Since(started))
	})
}

func TestDeleteTopicWaitUsesOnlyMetadata(t *testing.T) {
	const topic = "delete-me"
	cluster := kfake.MustCluster(kfake.NumBrokers(1), kfake.SeedTopics(1, topic, "other"))
	t.Cleanup(cluster.Close)
	client := openTestClient(t, cluster)
	broker := testBrokerMetadata(t, cluster)
	var metadataPolls atomic.Int32
	cluster.ControlKey(int16(kmsg.Metadata), func(req kmsg.Request) (kmsg.Response, error, bool) {
		for _, requested := range req.(*kmsg.MetadataRequest).Topics {
			if requested.Topic == nil || *requested.Topic != topic {
				continue
			}
			cluster.KeepControl()
			resp := req.ResponseKind().(*kmsg.MetadataResponse)
			resp.Brokers, resp.ControllerID = []kmsg.MetadataResponseBroker{broker}, broker.NodeID
			result := kmsg.NewMetadataResponseTopic()
			result.Topic = kmsg.StringPtr(topic)
			if metadataPolls.Add(1) > 1 {
				result.ErrorCode = kerr.UnknownTopicOrPartition.Code
			}
			resp.Topics = []kmsg.MetadataResponseTopic{result}
			return resp, nil, true
		}
		return nil, nil, false
	})
	var configRequests atomic.Int32
	cluster.ControlKey(int16(kmsg.DescribeConfigs), func(req kmsg.Request) (kmsg.Response, error, bool) {
		configRequests.Add(1)
		resp := req.ResponseKind().(*kmsg.DescribeConfigsResponse)
		resp.Resources = []kmsg.DescribeConfigsResponseResource{{
			ResourceName: "other", ResourceType: kmsg.ConfigResourceTypeTopic, ErrorCode: kerr.UnknownTopicOrPartition.Code,
		}}
		return resp, nil, true
	})
	require.NoError(t, client.DeleteTopic(topic, true))
	require.Equal(t, int32(2), metadataPolls.Load())
	require.Zero(t, configRequests.Load())
}

func TestReplicaHealthIncludesInternalTopics(t *testing.T) {
	cluster := kfake.MustCluster(kfake.NumBrokers(1))
	t.Cleanup(cluster.Close)
	client := openTestClient(t, cluster)
	broker := testBrokerMetadata(t, cluster)
	var reads atomic.Int32
	cluster.ControlKey(int16(kmsg.Metadata), func(req kmsg.Request) (kmsg.Response, error, bool) {
		if req.(*kmsg.MetadataRequest).Topics != nil {
			return nil, nil, false
		}
		cluster.KeepControl()
		reads.Add(1)
		resp := req.ResponseKind().(*kmsg.MetadataResponse)
		resp.Brokers = []kmsg.MetadataResponseBroker{broker}
		resp.Topics = []kmsg.MetadataResponseTopic{{
			Topic: kmsg.StringPtr("__internal-test"), IsInternal: true,
			Partitions: []kmsg.MetadataResponseTopicPartition{
				{Partition: 0, Replicas: []int32{0, 1, 2}, ISR: []int32{0}, OfflineReplicas: []int32{1}},
				{Partition: 1, Replicas: []int32{0, 1}, ISR: []int32{0}, OfflineReplicas: []int32{1}},
			},
		}}
		return resp, nil, true
	})
	offline, outOfSync, err := client.ReplicaHealth()
	require.NoError(t, err)
	require.Equal(t, []int32{1}, offline)
	require.Equal(t, []int32{1, 2}, outOfSync)
	require.Equal(t, int32(1), reads.Load())
}

func TestReplicaHealthUsesReplicaDataDespiteLeaderErrors(t *testing.T) {
	for _, tc := range []struct {
		name           string
		topicError     int16
		partitionError int16
	}{
		{name: "topic error", topicError: kerr.LeaderNotAvailable.Code},
		{name: "partition error", partitionError: kerr.LeaderNotAvailable.Code},
		{name: "missing listener", partitionError: kerr.ListenerNotFound.Code},
	} {
		t.Run(tc.name, func(t *testing.T) {
			metadata := &kmsg.MetadataResponse{
				Brokers: []kmsg.MetadataResponseBroker{{NodeID: 0}},
				Topics: []kmsg.MetadataResponseTopic{{
					Topic: kmsg.StringPtr("test-topic"), ErrorCode: tc.topicError,
					Partitions: []kmsg.MetadataResponseTopicPartition{{
						ErrorCode: tc.partitionError, Replicas: []int32{0, 1, 2},
						ISR: []int32{0}, OfflineReplicas: []int32{1},
					}},
				}},
			}
			offline, outOfSync, err := replicaHealthFromMetadata(metadata)
			require.NoError(t, err)
			require.Equal(t, []int32{1}, offline)
			require.Equal(t, []int32{1, 2}, outOfSync)
		})
	}
}

func TestReplicaHealthRejectsMissingListenerWithoutOfflineReplicas(t *testing.T) {
	metadata := &kmsg.MetadataResponse{
		Brokers: []kmsg.MetadataResponseBroker{{NodeID: 0}},
		Topics: []kmsg.MetadataResponseTopic{{
			Topic: kmsg.StringPtr("test-topic"),
			Partitions: []kmsg.MetadataResponseTopicPartition{{
				ErrorCode: kerr.ListenerNotFound.Code, Replicas: []int32{0}, ISR: []int32{0},
			}},
		}},
	}
	offline, outOfSync, err := replicaHealthFromMetadata(metadata)
	require.ErrorIs(t, err, kerr.ReplicaNotAvailable)
	require.Nil(t, offline)
	require.Nil(t, outOfSync)
}

func TestReplicaHealthRefreshesAfterTopicDeletion(t *testing.T) {
	cluster := kfake.MustCluster(kfake.NumBrokers(1))
	t.Cleanup(cluster.Close)
	client := openTestClient(t, cluster)
	broker := testBrokerMetadata(t, cluster)
	var reads atomic.Int32
	cluster.ControlKey(int16(kmsg.Metadata), func(req kmsg.Request) (kmsg.Response, error, bool) {
		cluster.KeepControl()
		resp := req.ResponseKind().(*kmsg.MetadataResponse)
		resp.Brokers = []kmsg.MetadataResponseBroker{broker}
		resp.Topics = []kmsg.MetadataResponseTopic{{
			Topic:      kmsg.StringPtr("live"),
			Partitions: []kmsg.MetadataResponseTopicPartition{{Replicas: []int32{0, 1, 2}, ISR: []int32{0}, OfflineReplicas: []int32{1}}},
		}}
		if reads.Add(1) == 1 {
			resp.Topics[0].Partitions[0].ISR = []int32{0, 1}
			resp.Topics[0].Partitions[0].OfflineReplicas = []int32{2}
			resp.Topics = append(resp.Topics, kmsg.MetadataResponseTopic{
				Topic: kmsg.StringPtr("deleted"), ErrorCode: kerr.UnknownTopicOrPartition.Code,
			})
		}
		return resp, nil, true
	})
	offline, outOfSync, err := client.ReplicaHealth()
	require.NoError(t, err)
	require.Equal(t, []int32{1}, offline)
	require.Equal(t, []int32{1, 2}, outOfSync)
	require.Equal(t, int32(2), reads.Load())
}

func TestReplicaHealthRejectsIncompleteMetadata(t *testing.T) {
	for _, code := range []*kerr.Error{kerr.UnknownTopicOrPartition, kerr.LeaderNotAvailable} {
		t.Run(code.Message, func(t *testing.T) {
			cluster := kfake.MustCluster(kfake.NumBrokers(1))
			t.Cleanup(cluster.Close)
			client := openTestClient(t, cluster)
			broker := testBrokerMetadata(t, cluster)
			cluster.ControlKey(int16(kmsg.Metadata), func(req kmsg.Request) (kmsg.Response, error, bool) {
				cluster.KeepControl()
				resp := req.ResponseKind().(*kmsg.MetadataResponse)
				resp.Brokers = []kmsg.MetadataResponseBroker{broker}
				resp.Topics = []kmsg.MetadataResponseTopic{{Topic: kmsg.StringPtr("incomplete"), ErrorCode: code.Code}}
				return resp, nil, true
			})
			_, _, err := client.ReplicaHealth()
			var notReady errorfactory.BrokersNotReady
			require.ErrorAs(t, err, &notReady)
		})
	}
}

func TestReplicaHealthRejectsEmptyBrokerList(t *testing.T) {
	cluster := kfake.MustCluster(kfake.NumBrokers(1))
	t.Cleanup(cluster.Close)
	client := openTestClient(t, cluster)
	cluster.ControlKey(int16(kmsg.Metadata), func(req kmsg.Request) (kmsg.Response, error, bool) {
		cluster.KeepControl()
		return req.ResponseKind(), nil, true
	})
	_, _, err := client.ReplicaHealth()
	var notReady errorfactory.BrokersNotReady
	require.ErrorAs(t, err, &notReady)
}

func TestReplicaHealthRejectsPartitionWithoutReplicas(t *testing.T) {
	cluster := kfake.MustCluster(kfake.NumBrokers(1))
	t.Cleanup(cluster.Close)
	client := openTestClient(t, cluster)
	broker := testBrokerMetadata(t, cluster)
	var reads atomic.Int32
	cluster.ControlKey(int16(kmsg.Metadata), func(req kmsg.Request) (kmsg.Response, error, bool) {
		if req.(*kmsg.MetadataRequest).Topics != nil {
			return nil, nil, false
		}
		cluster.KeepControl()
		reads.Add(1)
		resp := req.ResponseKind().(*kmsg.MetadataResponse)
		resp.Brokers = []kmsg.MetadataResponseBroker{broker}
		resp.Topics = []kmsg.MetadataResponseTopic{{
			Topic: kmsg.StringPtr("incomplete"), Partitions: []kmsg.MetadataResponseTopicPartition{{Partition: 0}},
		}}
		return resp, nil, true
	})
	_, _, err := client.ReplicaHealth()
	var notReady errorfactory.BrokersNotReady
	require.ErrorAs(t, err, &notReady)
	require.Equal(t, int32(2), reads.Load())
}

func TestReplicaHealthKeepsReadinessWhenRefreshTimesOut(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		cluster, client, dialer := newTimedKafka(t, 1, kfake.NumBrokers(1))
		require.NoError(t, client.open(dialer))
		broker := testBrokerMetadata(t, cluster)
		var reads atomic.Int32
		release := make(chan struct{})
		defer func() {
			close(release)
			synctest.Wait()
		}()
		cluster.ControlKey(int16(kmsg.Metadata), func(req kmsg.Request) (kmsg.Response, error, bool) {
			if req.(*kmsg.MetadataRequest).Topics != nil {
				return nil, nil, false
			}
			cluster.KeepControl()
			if reads.Add(1) == 1 {
				resp := req.ResponseKind().(*kmsg.MetadataResponse)
				resp.Brokers = []kmsg.MetadataResponseBroker{broker}
				resp.Topics = []kmsg.MetadataResponseTopic{{Topic: kmsg.StringPtr("deleted"), ErrorCode: kerr.UnknownTopicOrPartition.Code}}
				return resp, nil, true
			}
			<-release
			return nil, nil, false
		})
		_, _, err := client.ReplicaHealth()
		var notReady errorfactory.BrokersNotReady
		require.ErrorAs(t, err, &notReady)
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.Equal(t, int32(2), reads.Load())
	})
}
