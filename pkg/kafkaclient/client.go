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
	"fmt"
	"net"
	"slices"
	"strconv"
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

type Broker struct {
	ID   int32
	Addr string
}

// KafkaClient is the exported interface for kafka operations
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

	// AllOfflineReplicas returns the list of unique offline replica (broker) ids
	AllOfflineReplicas() ([]int32, error)

	// OutOfSyncReplicas returns the list of unique out of sync replica (broker) ids
	OutOfSyncReplicas() ([]int32, error)

	AlterPerBrokerConfig(int32, map[string]*string, bool) error
	DescribePerBrokerConfig(int32, []string) ([]ConfigEntry, error)

	AlterClusterWideConfig(map[string]*string, bool) error
	DescribeClusterWideConfig() ([]ConfigEntry, error)

	Open() error
	Close() error
}

type kafkaClient struct {
	opts    *KafkaConfig
	admin   *kadm.Client
	client  *kgo.Client
	timeout time.Duration
	brokers []Broker
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
	opts := []kgo.Opt{kgo.SeedBrokers(k.opts.BrokerURI), kgo.ClientID(clientID)}
	switch {
	case k.opts.UseSSL && k.opts.TLSConfig == nil:
		opts = append(opts, kgo.DialTLS())
	case k.opts.UseSSL:
		opts = append(opts, kgo.DialTLSConfig(k.opts.TLSConfig))
	}
	conn, err := kgo.NewClient(opts...)
	if err == nil {
		ctx, cancel := k.requestContext()
		defer cancel()
		err = conn.Ping(ctx)
	}
	if err != nil {
		if conn != nil {
			conn.Close()
		}
		return errorfactory.New(errorfactory.BrokersUnreachable{}, err, fmt.Sprintf("could not connect to kafka brokers: %s", k.opts.BrokerURI))
	}

	brokers, controllerID, err := k.describeCluster(conn)
	// In KRaft, Kafka advertises a broker that can forward admin requests.
	if err == nil && !slices.ContainsFunc(brokers, func(broker Broker) bool { return broker.ID == controllerID }) {
		err = fmt.Errorf("kafka metadata has no broker for controller ID %d", controllerID)
	}
	if err != nil {
		conn.Close()
		return errorfactory.New(errorfactory.BrokersNotReady{}, err, "could not describe kafka cluster")
	}
	_ = k.Close()
	k.client, k.admin, k.brokers = conn, kadm.NewClient(conn), brokers
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
	return k.describeCluster(k.client)
}

func (k *kafkaClient) describeCluster(conn *kgo.Client) ([]Broker, int32, error) {
	ctx, cancel := k.requestContext()
	defer cancel()
	req := kmsg.NewPtrMetadataRequest()
	req.Topics = []kmsg.MetadataRequestTopic{}
	meta, err := req.RequestWith(ctx, conn)
	if err != nil {
		return nil, -1, err
	}
	brokers := make([]Broker, 0, len(meta.Brokers))
	for _, broker := range meta.Brokers {
		brokers = append(brokers, Broker{ID: broker.NodeID, Addr: net.JoinHostPort(broker.Host, strconv.Itoa(int(broker.Port)))})
	}
	return brokers, meta.ControllerID, nil
}

func (k *kafkaClient) requestContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), k.timeout)
}
