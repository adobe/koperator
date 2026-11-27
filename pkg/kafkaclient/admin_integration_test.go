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
	"crypto/tls"
	"crypto/x509"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"

	"github.com/banzaicloud/koperator/pkg/errorfactory"
)

// TestKafkaAdminIntegration creates a topic and replaces broker configuration.
// Run it against a disposable cluster with an ACL authorizer and admin credentials.
func TestKafkaAdminIntegration(t *testing.T) {
	addr := os.Getenv("KOPERATOR_TEST_KAFKA_ADDR")
	if addr == "" {
		t.Skip("set KOPERATOR_TEST_KAFKA_ADDR to run against a disposable Kafka cluster")
	}
	config := &KafkaConfig{BrokerURI: addr, OperationTimeout: 10}
	observerOpts := []kgo.Opt{kgo.SeedBrokers(addr)}
	if dir := os.Getenv("KOPERATOR_TEST_KAFKA_TLS_DIR"); dir != "" {
		cert, err := tls.LoadX509KeyPair(filepath.Join(dir, "tls.crt"), filepath.Join(dir, "tls.key"))
		require.NoError(t, err)
		caPEM, err := os.ReadFile(filepath.Join(dir, "ca.crt"))
		require.NoError(t, err)
		roots := x509.NewCertPool()
		require.True(t, roots.AppendCertsFromPEM(caPEM))
		config.UseSSL = true
		config.TLSConfig = &tls.Config{RootCAs: roots, Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
		observerOpts = append(observerOpts, kgo.DialTLSConfig(config.TLSConfig))
	}
	client := New(config)
	require.EventuallyWithT(t, func(c *assert.CollectT) { require.NoError(c, client.Open()) }, time.Minute, time.Second)
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	if config.UseSSL {
		withoutCert := New(&KafkaConfig{
			BrokerURI: addr, UseSSL: true, OperationTimeout: 2,
			TLSConfig: &tls.Config{RootCAs: config.TLSConfig.RootCAs, MinVersion: tls.VersionTLS12},
		})
		var unreachable errorfactory.BrokersUnreachable
		require.ErrorAs(t, withoutCert.Open(), &unreachable)
		require.NoError(t, withoutCert.Close())
	}

	brokers, _, err := client.DescribeCluster()
	require.NoError(t, err)
	require.NotEmpty(t, brokers)
	require.Equal(t, len(brokers), client.NumBrokers())
	brokerID := brokers[0].ID

	const topic = "koperator-admin-smoke"
	require.NoError(t, client.CreateTopic(&CreateTopicOptions{Name: topic, Partitions: 1, ReplicationFactor: 1}))
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		metadata, err := client.DescribeTopic(topic)
		require.NoError(c, err)
		require.Len(c, metadata.Partitions, 1)
	}, 15*time.Second, 250*time.Millisecond)
	changed, err := client.EnsurePartitionCount(topic, 2)
	require.NoError(t, err)
	require.True(t, changed)
	require.NoError(t, client.EnsureTopicConfig(topic, map[string]*string{"retention.ms": kmsg.StringPtr("60000")}))
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		detail, err := client.GetTopic(topic)
		require.NoError(c, err)
		require.NotNil(c, detail)
		require.Equal(c, int32(2), detail.NumPartitions)
		require.Equal(c, kmsg.StringPtr("60000"), detail.ConfigEntries["retention.ms"])
	}, 15*time.Second, 250*time.Millisecond)
	offline, err := client.AllOfflineReplicas()
	require.NoError(t, err)
	require.Empty(t, offline)
	outOfSync, err := client.OutOfSyncReplicas()
	require.NoError(t, err)
	require.Empty(t, outOfSync)

	require.NoError(t, client.CreateUserACLs("read", "literal", "CN=smoke-reader", topic))
	observer, err := kgo.NewClient(observerOpts...)
	require.NoError(t, err)
	t.Cleanup(observer.Close)
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		req := kmsg.NewPtrDescribeACLsRequest()
		req.ResourceType, req.ResourcePatternType = kmsg.ACLResourceTypeAny, kmsg.ACLResourcePatternTypeAny
		req.Operation, req.PermissionType = kmsg.ACLOperationAny, kmsg.ACLPermissionTypeAny
		req.Principal = kmsg.StringPtr("User:CN=smoke-reader")
		response, err := req.RequestWith(ctx, observer)
		require.NoError(c, err)
		require.Zero(c, response.ErrorCode)
		var grants []string
		for _, resource := range response.Resources {
			for _, binding := range resource.ACLs {
				grants = append(grants, resource.ResourceName+"/"+binding.Operation.String())
			}
		}
		require.ElementsMatch(c, []string{topic + "/READ", topic + "/DESCRIBE", topic + "/DESCRIBE_CONFIGS", "*/READ"}, grants)
	}, 15*time.Second, 250*time.Millisecond)

	checkBrokerConfig(t, "101", client.DescribeClusterWideConfig, client.AlterClusterWideConfig)
	checkBrokerConfig(t, "102", func() ([]ConfigEntry, error) {
		return client.DescribePerBrokerConfig(brokerID, []string{"max.connections", "max.connection.creation.rate"})
	}, func(change map[string]*string, validateOnly bool) error {
		return client.AlterPerBrokerConfig(brokerID, change, validateOnly)
	})
	require.NoError(t, client.DeleteTopic(topic, true))
	detail, err := client.GetTopic(topic)
	require.NoError(t, err)
	require.Nil(t, detail)
}

func checkBrokerConfig(t *testing.T, value string, describe func() ([]ConfigEntry, error), alter func(map[string]*string, bool) error) {
	t.Helper()
	const (
		key             = "max.connections"
		omitted         = "max.connection.creation.rate"
		initialOverride = "150"
	)
	read := func(c require.TestingT, name string) string {
		entries, err := describe()
		require.NoError(c, err)
		for _, entry := range entries {
			if entry.Name == name {
				return entry.Value
			}
		}
		return ""
	}
	before := read(t, key)
	beforeOmitted := read(t, omitted)
	require.NotEqual(t, value, before)
	require.NotEqual(t, initialOverride, beforeOmitted)
	require.NoError(t, alter(map[string]*string{omitted: kmsg.StringPtr(initialOverride)}, false))
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		require.Equal(c, initialOverride, read(c, omitted))
	}, 15*time.Second, 250*time.Millisecond)

	change := map[string]*string{key: &value}
	require.NoError(t, alter(change, true))
	require.Equal(t, before, read(t, key))
	require.Equal(t, initialOverride, read(t, omitted))
	require.Error(t, alter(map[string]*string{key: kmsg.StringPtr("invalid")}, true))
	require.Equal(t, before, read(t, key))
	require.NoError(t, alter(change, false))
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		require.Equal(c, value, read(c, key))
		require.Equal(c, beforeOmitted, read(c, omitted))
	}, 15*time.Second, 250*time.Millisecond)
}
