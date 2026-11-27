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
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"

	"github.com/banzaicloud/koperator/pkg/errorfactory"
)

// TestKafkaAdminIntegration checks Kafka administration against a real broker.
// Set KOPERATOR_TEST_KAFKA_DOCKER_IMAGE to run against a fresh Docker broker,
// including configuration replacement at the cluster and broker levels.
// Use an image with Kafka's command-line scripts, such as apache/kafka:3.9.2.
//
// Alternatively, set KOPERATOR_TEST_KAFKA_ADDR to the address of a disposable
// broker with an ACL authorizer. In this mode the test exercises topics and ACLs
// without changing broker configuration. Kafka's advertised addresses must be
// reachable. For TLS, set KOPERATOR_TEST_KAFKA_TLS_DIR to a directory containing
// tls.crt, tls.key, and ca.crt. Set KOPERATOR_TEST_KAFKA_TLS_CLIENT_AUTH=optional
// for a listener that does not require client certificates. The default is
// required. The test does not support SASL.
func TestKafkaAdminIntegration(t *testing.T) {
	addr := os.Getenv("KOPERATOR_TEST_KAFKA_ADDR")
	image := os.Getenv("KOPERATOR_TEST_KAFKA_DOCKER_IMAGE")
	tlsDir := os.Getenv("KOPERATOR_TEST_KAFKA_TLS_DIR")
	if addr == "" && image == "" {
		t.Skip("set KOPERATOR_TEST_KAFKA_DOCKER_IMAGE or KOPERATOR_TEST_KAFKA_ADDR")
	}
	if image != "" {
		require.Empty(t, addr, "select either the Docker broker or an external broker")
		require.Empty(t, tlsDir, "the Docker fixture uses plaintext on loopback")
		addr = startKafkaAdminFixture(t, image)
	}
	clientAuth := os.Getenv("KOPERATOR_TEST_KAFKA_TLS_CLIENT_AUTH")
	require.Contains(t, []string{"", "required", "optional"}, clientAuth, "invalid client certificate mode")
	config := &KafkaConfig{BrokerURI: addr, OperationTimeout: 10}
	observerOpts := []kgo.Opt{kgo.SeedBrokers(addr)}
	if tlsDir != "" {
		cert, err := tls.LoadX509KeyPair(filepath.Join(tlsDir, "tls.crt"), filepath.Join(tlsDir, "tls.key"))
		require.NoError(t, err)
		caPEM, err := os.ReadFile(filepath.Join(tlsDir, "ca.crt"))
		require.NoError(t, err)
		roots := x509.NewCertPool()
		require.True(t, roots.AppendCertsFromPEM(caPEM))
		config.UseSSL = true
		config.TLSConfig = &tls.Config{RootCAs: roots, Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
		observerOpts = append(observerOpts, kgo.DialTLSConfig(config.TLSConfig))
	}
	client := New(config).(*kafkaClient)
	require.EventuallyWithT(t, func(c *assert.CollectT) { require.NoError(c, client.Open()) }, time.Minute, time.Second)
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	if config.UseSSL && clientAuth != "optional" {
		withoutCert := New(&KafkaConfig{
			BrokerURI: addr, UseSSL: true, OperationTimeout: 2,
			TLSConfig: &tls.Config{RootCAs: config.TLSConfig.RootCAs, MinVersion: tls.VersionTLS12},
		})
		t.Cleanup(func() { require.NoError(t, withoutCert.Close()) })
		var unreachable errorfactory.BrokersUnreachable
		require.ErrorAs(t, withoutCert.Open(), &unreachable)
	}

	brokers, _, err := client.DescribeCluster()
	require.NoError(t, err)
	require.NotEmpty(t, brokers)
	require.Equal(t, len(brokers), client.NumBrokers())
	brokerID := brokers[0].ID

	suffix := strings.ToLower(rand.Text())
	topic := "koperator-admin-smoke-" + suffix
	deleted := false
	t.Cleanup(func() {
		if !deleted {
			err := client.DeleteTopic(topic, false)
			if err != nil && !errors.Is(err, kerr.UnknownTopicOrPartition) {
				t.Errorf("clean up fixture topic %s: %v", topic, err)
			}
		}
	})
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
	offline, outOfSync, err := client.ReplicaHealth()
	require.NoError(t, err)
	require.Empty(t, offline)
	require.Empty(t, outOfSync)

	reader := "CN=smoke-reader-" + suffix
	cleanUpFixtureACLs(t, client, reader, topic)
	require.NoError(t, client.CreateUserACLs("read", "literal", reader, topic))
	observer, err := kgo.NewClient(observerOpts...)
	require.NoError(t, err)
	t.Cleanup(observer.Close)
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		req := kmsg.NewPtrDescribeACLsRequest()
		req.ResourceType, req.ResourcePatternType = kmsg.ACLResourceTypeAny, kmsg.ACLResourcePatternTypeAny
		req.Operation, req.PermissionType = kmsg.ACLOperationAny, kmsg.ACLPermissionTypeAny
		req.Principal = kmsg.StringPtr("User:" + reader)
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

	if image != "" {
		checkBrokerConfig(t, "101", client.DescribeClusterWideConfig, client.AlterClusterWideConfig)
		checkBrokerConfig(t, "102", func() ([]ConfigEntry, error) {
			return client.DescribePerBrokerConfig(brokerID, []string{"max.connections", "max.connection.creation.rate"})
		}, func(change map[string]*string, validateOnly bool) error {
			return client.AlterPerBrokerConfig(brokerID, change, validateOnly)
		})
	}
	require.NoError(t, client.DeleteTopic(topic, true))
	detail, err := client.GetTopic(topic)
	require.NoError(t, err)
	require.Nil(t, detail)
	deleted = true
}

func startKafkaAdminFixture(t *testing.T, image string) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := listener.Addr().String()
	require.NoError(t, listener.Close())
	name := "koperator-admin-" + strings.ToLower(rand.Text())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		output, err := exec.CommandContext(ctx, "docker", "rm", "-f", name).CombinedOutput()
		if err != nil && !strings.Contains(string(output), "No such container") {
			t.Errorf("remove Kafka fixture %s: %v: %s", name, err, output)
		}
	})
	properties := fmt.Sprintf(`process.roles=broker,controller
node.id=0
controller.quorum.voters=0@127.0.0.1:9093
listeners=PLAINTEXT://0.0.0.0:9092,CONTROLLER://127.0.0.1:9093
advertised.listeners=PLAINTEXT://%s
listener.security.protocol.map=PLAINTEXT:PLAINTEXT,CONTROLLER:PLAINTEXT
controller.listener.names=CONTROLLER
inter.broker.listener.name=PLAINTEXT
authorizer.class.name=org.apache.kafka.metadata.authorizer.StandardAuthorizer
super.users=User:ANONYMOUS
log.dirs=/tmp/koperator-kafka-data
`, addr)
	const start = `
bin="${KAFKA_HOME:-/opt/kafka}/bin"
printf '%s\n' "$KOPERATOR_BROKER_CONFIG" > /tmp/koperator-server.properties
cluster_id=$("$bin/kafka-storage.sh" random-uuid)
"$bin/kafka-storage.sh" format --cluster-id "$cluster_id" --config /tmp/koperator-server.properties
exec "$bin/kafka-server-start.sh" /tmp/koperator-server.properties
`
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	output, err := exec.CommandContext(ctx, "docker", "run", "--rm", "--detach", "--name", name,
		"--publish", addr+":9092", "--env", "KOPERATOR_BROKER_CONFIG="+properties,
		"--entrypoint", "/bin/sh", image, "-ec", start).CombinedOutput()
	require.NoError(t, err, "%s", output)
	return addr
}

func cleanUpFixtureACLs(t *testing.T, client *kafkaClient, reader, topic string) {
	t.Helper()
	common, specific, err := userACLs("read", "literal", reader, topic)
	require.NoError(t, err)
	req := kmsg.NewPtrDeleteACLsRequest()
	for _, binding := range append(common, specific...) {
		req.Filters = append(req.Filters, kmsg.DeleteACLsRequestFilter{
			ResourceType: binding.ResourceType, ResourceName: kmsg.StringPtr(binding.ResourceName),
			ResourcePatternType: binding.ResourcePatternType, Principal: kmsg.StringPtr(binding.Principal),
			Host: kmsg.StringPtr(binding.Host), Operation: binding.Operation, PermissionType: binding.PermissionType,
		})
	}
	t.Cleanup(func() {
		err := client.retryACLControllerChange(func(ctx context.Context, broker *kgo.Broker) error {
			resp, err := req.RequestWith(ctx, broker)
			if err != nil {
				return err
			}
			if len(resp.Results) != len(req.Filters) {
				return fmt.Errorf("received %d results for %d ACL cleanup filters", len(resp.Results), len(req.Filters))
			}
			for _, result := range resp.Results {
				if err := brokerResponseError(result.ErrorCode, result.ErrorMessage); err != nil {
					return err
				}
				for _, match := range result.MatchingACLs {
					if err := brokerResponseError(match.ErrorCode, match.ErrorMessage); err != nil {
						return err
					}
				}
			}
			return nil
		})
		require.NoError(t, err, "clean up grants for %s", reader)
	})
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
