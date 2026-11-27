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

package main

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kfake"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"
)

type polledRecords chan *kgo.Record

func (records polledRecords) OnFetchRecordUnbuffered(record *kgo.Record, polled bool) {
	if polled {
		select {
		case records <- record:
		default:
		}
	}
}

func TestConsumerRequiresTopic(t *testing.T) {
	cluster := kfake.MustCluster(kfake.NumBrokers(1))
	t.Cleanup(cluster.Close)
	client, err := kgo.NewClient(kgo.SeedBrokers(cluster.ListenAddrs()...))
	require.NoError(t, err)
	t.Cleanup(client.Close)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	require.ErrorContains(t, consume(ctx, client), kafkaTopic)
}

func TestSmokeProducerAndConsumer(t *testing.T) {
	cluster := kfake.MustCluster(kfake.NumBrokers(1), kfake.SeedTopics(1, kafkaTopic))
	t.Cleanup(cluster.Close)
	seed := kgo.SeedBrokers(cluster.ListenAddrs()...)
	writer, err := kgo.NewClient(seed)
	require.NoError(t, err)
	t.Cleanup(writer.Close)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	require.NoError(t, writer.ProduceSync(ctx, &kgo.Record{Topic: kafkaTopic, Value: []byte("old record")}).FirstErr())

	fetchStarted := make(chan struct{})
	var firstFetch sync.Once
	cluster.ControlKey(int16(kmsg.Fetch), func(kmsg.Request) (kmsg.Response, error, bool) {
		firstFetch.Do(func() { close(fetchStarted) })
		return nil, nil, false
	})
	records := make(polledRecords, 1)
	consumerDone := make(chan error, 1)
	go func() { consumerDone <- run(ctx, "consumer", seed, kgo.WithHooks(records)) }()
	select {
	case <-fetchStarted:
	case err := <-consumerDone:
		t.Fatalf("consumer exited before fetching: %v", err)
	case <-ctx.Done():
		t.Fatal("consumer did not start fetching")
	}

	producerDone := make(chan error, 1)
	go func() { producerDone <- run(ctx, "producer", seed) }()
	select {
	case record := <-records:
		require.Equal(t, kafkaTopic, record.Topic)
		require.Equal(t, int32(kafkaPartition), record.Partition)
		require.Equal(t, int64(1), record.Offset)
		require.Equal(t, "hello world", string(record.Value))
	case err := <-producerDone:
		t.Fatalf("producer exited before sending: %v", err)
	case <-ctx.Done():
		t.Fatal("consumer did not read the new record")
	}

	cancel()
	for _, done := range []chan error{producerDone, consumerDone} {
		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(time.Second):
			t.Fatal("smoke mode did not exit after cancellation")
		}
	}
}
