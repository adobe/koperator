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
	"net"
	"testing"

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
	cluster.ControlKey(int16(kmsg.Metadata), func(req kmsg.Request) (kmsg.Response, error, bool) {
		cluster.KeepControl()
		resp := req.ResponseKind().(*kmsg.MetadataResponse)
		resp.ControllerID = -1
		resp.Brokers = []kmsg.MetadataResponseBroker{{NodeID: 0, Host: "127.0.0.1", Port: 9092}}
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
