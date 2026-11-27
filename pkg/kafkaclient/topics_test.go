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
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kfake"
	"github.com/twmb/franz-go/pkg/kmsg"

	"github.com/banzaicloud/koperator/pkg/errorfactory"
)

func TestTopicAdministration(t *testing.T) {
	cluster := kfake.MustCluster(kfake.NumBrokers(1))
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

	require.NoError(t, client.DeleteTopic(topic, true))
	detail, err = client.GetTopic(topic)
	require.NoError(t, err)
	require.Nil(t, detail)
}
