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
	"errors"
	"maps"
	"slices"
	"sync"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/banzaicloud/koperator/api/v1alpha1"
	"github.com/banzaicloud/koperator/api/v1beta1"
)

type mockKafkaClient struct {
	mu     sync.Mutex
	topics map[string]TopicDetail
	acls   map[ACL]struct{}
}

func NewMockFromCluster(_ client.Client, _ *v1beta1.KafkaCluster) (KafkaClient, func(), error) {
	return &mockKafkaClient{topics: make(map[string]TopicDetail), acls: make(map[ACL]struct{})}, func() {}, nil
}

func (m *mockKafkaClient) Open() error  { return nil }
func (m *mockKafkaClient) Close() error { return nil }

func (m *mockKafkaClient) NumBrokers() int           { return 1 }
func (m *mockKafkaClient) Brokers() map[int32]string { return map[int32]string{0: ""} }

func (m *mockKafkaClient) DescribeCluster() ([]Broker, int32, error) {
	return []Broker{{ID: 0}}, 0, nil
}

func (m *mockKafkaClient) ListTopics() (map[string]TopicDetail, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return maps.Clone(m.topics), nil
}

func (m *mockKafkaClient) GetTopic(name string) (*TopicDetail, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if topic, ok := m.topics[name]; ok {
		return &topic, nil
	}
	return nil, nil
}

func (m *mockKafkaClient) DescribeTopic(name string) (*TopicMetadata, error) {
	topic, err := m.GetTopic(name)
	if err != nil {
		return nil, err
	}
	if topic == nil {
		return nil, errors.New("not found")
	}
	metadata := &TopicMetadata{Name: name}
	for id := range topic.NumPartitions {
		metadata.Partitions = append(metadata.Partitions, PartitionMetadata{ID: id})
	}
	return metadata, nil
}

func (m *mockKafkaClient) CreateTopic(opts *CreateTopicOptions) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.topics[opts.Name]; exists {
		return errors.New("already exists")
	}
	m.topics[opts.Name] = TopicDetail{
		NumPartitions: opts.Partitions, ReplicationFactor: opts.ReplicationFactor, ConfigEntries: opts.Config,
	}
	return nil
}

func (m *mockKafkaClient) DeleteTopic(name string, _ bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.topics[name]; !exists {
		return errors.New("does not exist")
	}
	delete(m.topics, name)
	return nil
}

func (m *mockKafkaClient) EnsurePartitionCount(name string, count int32) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	topic, exists := m.topics[name]
	if !exists {
		return false, errors.New("not found")
	}
	if topic.NumPartitions == count {
		return false, nil
	}
	if topic.NumPartitions > count {
		return true, errors.New("cannot decrease partition count")
	}
	topic.NumPartitions = count
	m.topics[name] = topic
	return true, nil
}

func (m *mockKafkaClient) EnsureTopicConfig(name string, config map[string]*string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	topic, exists := m.topics[name]
	if !exists {
		return errors.New("not found")
	}
	topic.ConfigEntries = config
	m.topics[name] = topic
	return nil
}

func (m *mockKafkaClient) CreateUserACLs(access v1alpha1.KafkaAccessType, pattern v1alpha1.KafkaPatternType, dn, topic string) error {
	common, specific, err := userACLs(access, pattern, dn, topic)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, binding := range append(common, specific...) {
		m.acls[ACL{
			ResourceType: binding.ResourceType.String(), ResourceName: binding.ResourceName, PatternType: binding.ResourcePatternType.String(),
			Principal: binding.Principal, Host: binding.Host, Operation: binding.Operation.String(), Permission: binding.PermissionType.String(),
		}] = struct{}{}
	}
	return nil
}

func (m *mockKafkaClient) ListUserACLs() ([]ACL, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Collect(maps.Keys(m.acls)), nil
}

func (m *mockKafkaClient) DeleteUserACLs(_ string, _ v1alpha1.KafkaPatternType) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	// The controller test fixture calls this method to reset the mock.
	clear(m.acls)
	return nil
}

func (m *mockKafkaClient) AllOfflineReplicas() ([]int32, error) { return nil, nil }
func (m *mockKafkaClient) OutOfSyncReplicas() ([]int32, error)  { return nil, nil }

func (m *mockKafkaClient) AlterPerBrokerConfig(_ int32, _ map[string]*string, _ bool) error {
	return nil
}

func (m *mockKafkaClient) DescribePerBrokerConfig(_ int32, _ []string) ([]ConfigEntry, error) {
	return nil, nil
}

func (m *mockKafkaClient) AlterClusterWideConfig(_ map[string]*string, _ bool) error {
	return nil
}

func (m *mockKafkaClient) DescribeClusterWideConfig() ([]ConfigEntry, error) {
	return nil, nil
}
