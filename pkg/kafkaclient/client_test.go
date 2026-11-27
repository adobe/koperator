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
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kfake"
	"github.com/twmb/franz-go/pkg/kmsg"

	"github.com/banzaicloud/koperator/pkg/errorfactory"
)

func TestOpen(t *testing.T) {
	cluster := kfake.MustCluster(kfake.NumBrokers(1))
	t.Cleanup(cluster.Close)
	client := openTestClient(t, cluster)
	require.Equal(t, 1, client.NumBrokers())
	require.Equal(t, map[int32]string{0: cluster.ListenAddrs()[0]}, client.Brokers())
}

func TestOpenUnreachable(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := listener.Addr().String()
	require.NoError(t, listener.Close())

	client := New(&KafkaConfig{BrokerURI: addr, OperationTimeout: 1})
	var unreachable errorfactory.BrokersUnreachable
	require.ErrorAs(t, client.Open(), &unreachable)
	require.NoError(t, client.Close())
}

func TestOpenBoundsProtocolNegotiation(t *testing.T) {
	t.Parallel()
	// Accept connections in the kernel without replying to any Kafka requests.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, listener.Close()) })

	client := New(&KafkaConfig{BrokerURI: listener.Addr().String()})
	start := time.Now()
	var unreachable errorfactory.BrokersUnreachable
	require.ErrorAs(t, client.Open(), &unreachable)
	require.Less(t, time.Since(start), 8*time.Second)
}

func TestOpenSharesDeadlineBetweenMetadataRequests(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		cluster, client, dialer := newTimedKafka(t, 0, kfake.NumBrokers(1))
		var reads atomic.Int32
		release := make(chan struct{})
		defer close(release)
		cluster.ControlKey(int16(kmsg.Metadata), func(kmsg.Request) (kmsg.Response, error, bool) {
			cluster.KeepControl()
			if reads.Add(1) == 1 {
				time.Sleep(3 * time.Second)
			} else {
				<-release
			}
			return nil, nil, false
		})
		var notReady errorfactory.BrokersNotReady
		started := time.Now()
		require.ErrorAs(t, client.open(dialer), &notReady)
		require.Equal(t, kafkaStartupTimeout, time.Since(started))
		require.Equal(t, int32(2), reads.Load())
	})
}

func TestOpenRequiresReachableAdvertisedController(t *testing.T) {
	cluster := kfake.MustCluster(kfake.NumBrokers(1))
	t.Cleanup(cluster.Close)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := int32(listener.Addr().(*net.TCPAddr).Port)
	require.NoError(t, listener.Close())
	cluster.ControlKey(int16(kmsg.Metadata), func(req kmsg.Request) (kmsg.Response, error, bool) {
		cluster.KeepControl()
		resp := req.ResponseKind().(*kmsg.MetadataResponse)
		resp.ControllerID = 1
		resp.Brokers = []kmsg.MetadataResponseBroker{{NodeID: 1, Host: "127.0.0.1", Port: port}}
		return resp, nil, true
	})
	client := New(&KafkaConfig{BrokerURI: cluster.ListenAddrs()[0], OperationTimeout: 1})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	var notReady errorfactory.BrokersNotReady
	require.ErrorAs(t, client.Open(), &notReady)
}

func TestOpenRetriesControllerMetadataRead(t *testing.T) {
	cluster := kfake.MustCluster(kfake.NumBrokers(2))
	t.Cleanup(cluster.Close)
	var metadataReads, controllerReads atomic.Int32
	cluster.ControlKey(int16(kmsg.Metadata), func(kmsg.Request) (kmsg.Response, error, bool) {
		cluster.KeepControl()
		metadataReads.Add(1)
		if cluster.CurrentNode() == 1 && controllerReads.Add(1) == 1 {
			return nil, io.EOF, true
		}
		return nil, nil, false
	})
	client := openTestClient(t, cluster)
	require.Equal(t, 2, client.NumBrokers())
	require.Equal(t, int32(4), metadataReads.Load()) // Discovery and controller check, then both again after the EOF.
	require.GreaterOrEqual(t, controllerReads.Load(), int32(2))
}

func TestOpenRediscoversAnUnreachableController(t *testing.T) {
	cluster := kfake.MustCluster(kfake.NumBrokers(1))
	t.Cleanup(cluster.Close)
	broker := testBrokerMetadata(t, cluster)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	unreachable := kmsg.MetadataResponseBroker{NodeID: 1, Host: "127.0.0.1", Port: int32(listener.Addr().(*net.TCPAddr).Port)}
	require.NoError(t, listener.Close())
	var firstMetadata sync.Once
	cluster.ControlKey(int16(kmsg.Metadata), func(req kmsg.Request) (kmsg.Response, error, bool) {
		cluster.KeepControl()
		resp := req.ResponseKind().(*kmsg.MetadataResponse)
		resp.Brokers, resp.ControllerID = []kmsg.MetadataResponseBroker{broker}, broker.NodeID
		firstMetadata.Do(func() {
			resp.Brokers = append(resp.Brokers, unreachable)
			resp.ControllerID = unreachable.NodeID
		})
		return resp, nil, true
	})
	client := New(&KafkaConfig{BrokerURI: cluster.ListenAddrs()[0], OperationTimeout: 2})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	require.NoError(t, client.Open())
	require.Equal(t, map[int32]string{0: cluster.ListenAddrs()[0]}, client.Brokers())
}

func TestOpenRejectsIncompleteControllerReply(t *testing.T) {
	cluster := kfake.MustCluster(kfake.NumBrokers(1))
	t.Cleanup(cluster.Close)
	var requests atomic.Int32
	cluster.ControlKey(int16(kmsg.Metadata), func(req kmsg.Request) (kmsg.Response, error, bool) {
		cluster.KeepControl()
		if requests.Add(1) == 1 {
			return nil, nil, false
		}
		return req.ResponseKind(), nil, true
	})
	client := New(&KafkaConfig{BrokerURI: cluster.ListenAddrs()[0]})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	var notReady errorfactory.BrokersNotReady
	require.ErrorAs(t, client.Open(), &notReady)
}

func TestOpenTLSDoesNotFallBackToPlaintext(t *testing.T) {
	cluster := kfake.MustCluster(kfake.NumBrokers(1))
	t.Cleanup(cluster.Close)
	client := New(&KafkaConfig{BrokerURI: cluster.ListenAddrs()[0], UseSSL: true, OperationTimeout: 1})
	var unreachable errorfactory.BrokersUnreachable
	require.ErrorAs(t, client.Open(), &unreachable)
	require.NoError(t, client.Close())
}

func TestOpenWithoutBrokerMetadata(t *testing.T) {
	cluster := kfake.MustCluster(kfake.NumBrokers(1))
	t.Cleanup(cluster.Close)
	cluster.ControlKey(int16(kmsg.Metadata), func(req kmsg.Request) (kmsg.Response, error, bool) {
		cluster.KeepControl()
		return req.ResponseKind(), nil, true
	})
	client := New(&KafkaConfig{BrokerURI: cluster.ListenAddrs()[0], OperationTimeout: 1})
	var notReady errorfactory.BrokersNotReady
	require.ErrorAs(t, client.Open(), &notReady)
	require.Zero(t, client.NumBrokers())
	require.NoError(t, client.Close())
}

func TestOpenWithoutControllerMetadata(t *testing.T) {
	cluster := kfake.MustCluster(kfake.NumBrokers(1))
	t.Cleanup(cluster.Close)
	broker := testBrokerMetadata(t, cluster)
	cluster.ControlKey(int16(kmsg.Metadata), func(req kmsg.Request) (kmsg.Response, error, bool) {
		cluster.KeepControl()
		resp := req.ResponseKind().(*kmsg.MetadataResponse)
		resp.ControllerID = -1
		resp.Brokers = []kmsg.MetadataResponseBroker{broker}
		return resp, nil, true
	})

	client := New(&KafkaConfig{BrokerURI: cluster.ListenAddrs()[0], OperationTimeout: 1})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	var notReady errorfactory.BrokersNotReady
	require.ErrorAs(t, client.Open(), &notReady)
}

func TestCloseAndReopen(t *testing.T) {
	cluster := kfake.MustCluster(kfake.NumBrokers(1))
	t.Cleanup(cluster.Close)
	client := openTestClient(t, cluster)
	require.NoError(t, client.Close())
	_, err := client.ListTopics()
	require.Error(t, err)
	require.NoError(t, client.Open())
	_, err = client.ListTopics()
	require.NoError(t, err)
}
