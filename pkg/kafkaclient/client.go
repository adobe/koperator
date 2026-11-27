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
	"io"
	"net"
	"slices"
	"strconv"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/banzaicloud/koperator/api/v1alpha1"
	"github.com/banzaicloud/koperator/api/v1beta1"
	"github.com/banzaicloud/koperator/pkg/errorfactory"
)

var log = logf.Log.WithName("kafka_util")

const clientID = "koperator"

const (
	kafkaStartupTimeout    = 5 * time.Second
	kafkaTopicWriteTimeout = 3 * time.Second
)

type Broker struct {
	ID   int32
	Addr string
}

// KafkaClient is the interface for Kafka administration.
type KafkaClient interface {
	NumBrokers() int
	ListTopics() (map[string]TopicDetail, error)
	CreateTopic(*CreateTopicOptions) error
	EnsurePartitionCount(string, int32) (bool, error)
	EnsureTopicConfig(string, map[string]*string) error
	DeleteTopic(string, bool) error
	GetTopic(string) (*TopicDetail, error)
	DescribeTopic(string) (*TopicMetadata, error)
	CreateUserACLs(v1alpha1.KafkaAccessType, v1alpha1.KafkaPatternType, string, string) error
	ListUserACLs() ([]ACL, error)
	DeleteUserACLs(string, v1alpha1.KafkaPatternType) error

	Brokers() map[int32]string
	DescribeCluster() ([]Broker, int32, error)

	// ReplicaHealth returns both sets of broker IDs from the same validated metadata snapshot.
	ReplicaHealth() (offline, outOfSync []int32, err error)

	AlterPerBrokerConfig(int32, map[string]*string, bool) error
	DescribePerBrokerConfig(int32, []string) ([]ConfigEntry, error)

	AlterClusterWideConfig(map[string]*string, bool) error
	DescribeClusterWideConfig() ([]ConfigEntry, error)

	Open() error
	Close() error
}

type kafkaClient struct {
	opts         *KafkaConfig
	admin        *kadm.Client
	client       *kgo.Client
	timeout      time.Duration
	brokers      []Broker
	controllerID atomic.Int32
}

func New(opts *KafkaConfig) KafkaClient {
	timeout := opts.OperationTimeout
	if timeout <= 0 {
		timeout = kafkaDefaultTimeout
	}
	return &kafkaClient{
		opts:    opts,
		timeout: time.Duration(timeout) * time.Second,
	}
}

func (k *kafkaClient) Open() error {
	return k.open()
}

func (k *kafkaClient) open(extraOpts ...kgo.Opt) error {
	opts := []kgo.Opt{
		kgo.SeedBrokers(k.opts.BrokerURI), kgo.ClientID(clientID),
		kgo.RequestTimeoutOverhead(min(k.timeout, 15*time.Minute)),
	}
	switch {
	case k.opts.UseSSL && k.opts.TLSConfig == nil:
		opts = append(opts, kgo.DialTLS())
	case k.opts.UseSSL:
		opts = append(opts, kgo.DialTLSConfig(k.opts.TLSConfig))
	}
	conn, err := kgo.NewClient(append(opts, extraOpts...)...)
	if err != nil {
		return errorfactory.New(errorfactory.BrokersUnreachable{}, err, fmt.Sprintf("could not connect to kafka brokers: %s", k.opts.BrokerURI))
	}

	ctx, cancel := context.WithTimeout(context.Background(), min(k.timeout, kafkaStartupTimeout))
	defer cancel()
	metadata, err := brokerMetadataRequest().RequestWith(ctx, conn)
	if err != nil {
		conn.Close()
		return errorfactory.New(errorfactory.BrokersUnreachable{}, err, fmt.Sprintf("could not connect to kafka brokers: %s", k.opts.BrokerURI))
	}
	metadata, err = readControllerMetadata(ctx, conn, metadata)
	if isBrokerConnectionError(err) {
		// A controller can restart after discovery. Rediscover once in the same startup budget.
		metadata, err = brokerMetadataRequest().RequestWith(ctx, conn)
		if err == nil {
			metadata, err = readControllerMetadata(ctx, conn, metadata)
		}
	}
	if err != nil {
		conn.Close()
		return errorfactory.New(errorfactory.BrokersNotReady{}, err, "could not describe kafka cluster")
	}
	admin := kadm.NewClient(conn)
	// Give the broker time to reply before the client deadline expires.
	replyBudget := min(time.Second, k.timeout/2)
	admin.SetTimeoutMillis(int32(min(kafkaTopicWriteTimeout, k.timeout-replyBudget).Milliseconds()))
	_ = k.Close()
	k.client, k.admin, k.brokers = conn, admin, clusterBrokers(metadata)
	k.controllerID.Store(metadata.ControllerID)
	return nil
}

func (k *kafkaClient) Close() error {
	if k.client != nil {
		k.client.Close()
	}
	return nil
}

// NewFromCluster is a convenience wrapper around New() and ClusterConfig()
func NewFromCluster(k8sclient client.Client, cluster *v1beta1.KafkaCluster) (KafkaClient, func(), error) {
	var client KafkaClient
	var err error
	opts, err := ClusterConfig(k8sclient, cluster)
	if err != nil {
		return nil, nil, err
	}
	client = New(opts)
	err = client.Open()
	close := func() {
		if err := client.Close(); err != nil {
			log.Error(err, "Error closing Kafka client")
		} else {
			log.V(1).Info("Kafka client closed cleanly")
		}
	}
	return client, close, err
}

func (k *kafkaClient) Brokers() map[int32]string {
	out := make(map[int32]string, len(k.brokers))
	for _, broker := range k.brokers {
		out[broker.ID] = broker.Addr
	}
	return out
}

func (k *kafkaClient) NumBrokers() int {
	return len(k.brokers)
}

func (k *kafkaClient) DescribeCluster() ([]Broker, int32, error) {
	ctx, cancel := k.requestContext()
	defer cancel()
	meta, err := brokerMetadataRequest().RequestWith(ctx, k.client)
	if err != nil {
		return nil, -1, err
	}
	return clusterBrokers(meta), meta.ControllerID, nil
}

func brokerMetadataRequest() *kmsg.MetadataRequest {
	req := kmsg.NewPtrMetadataRequest()
	req.Topics = []kmsg.MetadataRequestTopic{}
	return req
}

func isBrokerConnectionError(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, net.ErrClosed) ||
		errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.EPIPE) || isBrokerDialError(err)
}

func isBrokerDialError(err error) bool {
	var opErr *net.OpError
	return errors.As(err, &opErr) && opErr.Op == "dial"
}

func readControllerMetadata(ctx context.Context, conn *kgo.Client, metadata *kmsg.MetadataResponse) (*kmsg.MetadataResponse, error) {
	id, err := metadataController(metadata)
	if err != nil {
		return nil, err
	}
	metadata, err = brokerMetadataRequest().RequestWith(ctx, conn.Broker(int(id)))
	if err != nil {
		return nil, err
	}
	_, err = metadataController(metadata)
	return metadata, err
}

func metadataController(meta *kmsg.MetadataResponse) (int32, error) {
	// In KRaft, Kafka advertises a broker that can forward admin requests.
	id := meta.ControllerID
	if id < 0 || !slices.ContainsFunc(meta.Brokers, func(broker kmsg.MetadataResponseBroker) bool { return broker.NodeID == id }) {
		return -1, fmt.Errorf("kafka metadata has no broker for controller ID %d", id)
	}
	return id, nil
}

func clusterBrokers(meta *kmsg.MetadataResponse) []Broker {
	brokers := make([]Broker, 0, len(meta.Brokers))
	for _, broker := range meta.Brokers {
		brokers = append(brokers, Broker{ID: broker.NodeID, Addr: net.JoinHostPort(broker.Host, strconv.Itoa(int(broker.Port)))})
	}
	return brokers
}

func (k *kafkaClient) requestContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), k.timeout)
}
