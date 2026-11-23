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
	"sync"
	"time"

	"github.com/IBM/sarama"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/banzaicloud/koperator/api/v1beta1"
)

const (
	testTopicName = "test-topic"
	// withErrorTopicName is a mock topic name that triggers error responses.
	withErrorTopicName = "with-error"
)

type mockClusterAdmin struct {
	sarama.ClusterAdmin
	sarama.Client
	sync.Mutex
	failOps    bool
	mockTopics map[string]sarama.TopicDetail
	mockACLs   map[sarama.Resource]*sarama.ResourceAcls
}

// Coordinator resolves the ambiguity between sarama.ClusterAdmin.Coordinator and sarama.Client.Coordinator
func (m *mockClusterAdmin) Coordinator(coordinatorType string) (*sarama.Broker, error) {
	// Return a default mock coordinator
	return &sarama.Broker{}, nil
}

func NewMockFromCluster(client client.Client, cluster *v1beta1.KafkaCluster) (KafkaClient, func(), error) {
	cl := newOpenedMockClient()
	return cl, func() { _ = cl.Close() }, nil
}

func newEmptyMockClusterAdmin(failOps bool) *mockClusterAdmin {
	return &mockClusterAdmin{
		mockTopics: make(map[string]sarama.TopicDetail, 0),
		mockACLs:   make(map[sarama.Resource]*sarama.ResourceAcls, 0),
		failOps:    failOps,
	}
}

func newMockClusterAdmin([]string, *sarama.Config) (sarama.ClusterAdmin, error) {
	return newEmptyMockClusterAdmin(false), nil
}

func newMockKafkaClient([]string, *sarama.Config) (sarama.Client, error) {
	return newEmptyMockClusterAdmin(false), nil
}

func newMockClusterAdminError([]string, *sarama.Config) (sarama.ClusterAdmin, error) {
	return newEmptyMockClusterAdmin(false), errors.New("bad client")
}

func newMockClusterAdminFailOps([]string, *sarama.Config) (sarama.ClusterAdmin, error) {
	return newEmptyMockClusterAdmin(true), nil
}

func newMockOpts() *KafkaConfig {
	return &KafkaConfig{
		OperationTimeout: kafkaDefaultTimeout,
	}
}

func newMockClient() *kafkaClient {
	return &kafkaClient{
		opts:            newMockOpts(),
		timeout:         time.Duration(kafkaDefaultTimeout) * time.Second,
		newClusterAdmin: newMockClusterAdmin,
		newClient:       newMockKafkaClient,
	}
}

func newOpenedMockClient() *kafkaClient {
	client := newMockClient()
	_ = client.Open()
	return client
}

func (m *mockClusterAdmin) Close() error { return nil }

func (m *mockClusterAdmin) DescribeCluster() ([]*sarama.Broker, int32, error) {
	if m.failOps {
		return []*sarama.Broker{}, 0, errors.New("bad describe cluster")
	}
	return []*sarama.Broker{{}}, 0, nil
}

func (m *mockClusterAdmin) ListTopics() (map[string]sarama.TopicDetail, error) {
	m.Lock()
	defer m.Unlock()

	if m.failOps {
		return map[string]sarama.TopicDetail{}, errors.New("bad list topics")
	}
	return shallowCopy(m.mockTopics), nil
}

func (m *mockClusterAdmin) Topics() ([]string, error) {
	m.Lock()
	defer m.Unlock()

	if m.failOps {
		return nil, errors.New("bad list topics")
	}
	topics := make([]string, 0, len(m.mockTopics))
	for topicName := range m.mockTopics {
		topics = append(topics, topicName)
	}
	return topics, nil
}

func (m *mockClusterAdmin) DescribeTopics(topics []string) ([]*sarama.TopicMetadata, error) {
	if m.failOps {
		return []*sarama.TopicMetadata{}, errors.New("bad describe topics")
	}
	switch topics[0] {
	case testTopicName, "already-created-topic":
		return []*sarama.TopicMetadata{
			{
				Name:       topics[0],
				Partitions: []*sarama.PartitionMetadata{{}},
				Err:        sarama.ErrNoError,
			},
		}, nil
	case "not-exists":
		return []*sarama.TopicMetadata{}, nil
	case withErrorTopicName:
		return []*sarama.TopicMetadata{
			{
				Name:       withErrorTopicName,
				Partitions: []*sarama.PartitionMetadata{{}},
				Err:        sarama.ErrUnknown,
			},
		}, nil
	default:
		m.Lock()
		defer m.Unlock()
		detail, ok := m.mockTopics[topics[0]]
		if !ok {
			// Brokers answer an unknown topic with an explicit error, not an empty response.
			return []*sarama.TopicMetadata{{Name: topics[0], Err: sarama.ErrUnknownTopicOrPartition}}, nil
		}
		partitions := make([]*sarama.PartitionMetadata, detail.NumPartitions)
		for i := range partitions {
			partitions[i] = &sarama.PartitionMetadata{ID: int32(i)}
		}
		return []*sarama.TopicMetadata{{Name: topics[0], Partitions: partitions, Err: sarama.ErrNoError}}, nil
	}
}

func (m *mockClusterAdmin) CreateTopic(name string, detail *sarama.TopicDetail, validateOnly bool) error {
	m.Lock()
	defer m.Unlock()

	if m.failOps {
		return errors.New("bad create topic")
	}
	if _, ok := m.mockTopics[name]; ok {
		return &sarama.TopicError{Err: sarama.ErrTopicAlreadyExists}
	}
	m.mockTopics[name] = *detail
	return nil
}

func (m *mockClusterAdmin) DeleteTopic(name string) error {
	m.Lock()
	defer m.Unlock()

	if m.failOps {
		return errors.New("bad delete topic")
	}
	if _, ok := m.mockTopics[name]; ok {
		delete(m.mockTopics, name)
		return nil
	} else {
		return errors.New("does not exist")
	}
}

func (m *mockClusterAdmin) AlterConfig(resource sarama.ConfigResourceType, topic string, conf map[string]*string, validateOnly bool) error {
	if m.failOps {
		return errors.New("bad alter config")
	}
	return nil
}

func (m *mockClusterAdmin) CreatePartitions(topic string, count int32, assn [][]int32, validateOnly bool) error {
	m.Lock()
	defer m.Unlock()
	if detail, ok := m.mockTopics[topic]; ok && count < detail.NumPartitions {
		// Kafka can only add partitions.
		return sarama.ErrInvalidPartitions
	}
	return nil
}

func (m *mockClusterAdmin) CreateACL(resource sarama.Resource, acl sarama.Acl) error {
	m.Lock()
	defer m.Unlock()

	if m.failOps {
		return errors.New("bad create acl")
	}
	var resourceAcls *sarama.ResourceAcls
	if val, ok := m.mockACLs[resource]; ok {
		resourceAcls = val
	} else {
		resourceAcls = &sarama.ResourceAcls{
			Resource: resource,
		}
	}
	for _, v := range resourceAcls.Acls {
		if *v == acl {
			return nil
		}
	}
	resourceAcls.Acls = append(resourceAcls.Acls, &acl)
	m.mockACLs[resource] = resourceAcls
	return nil
}

func (m *mockClusterAdmin) CreateACLs(resourceACLs []*sarama.ResourceAcls) error {
	for _, ra := range resourceACLs {
		for _, acl := range ra.Acls {
			if err := m.CreateACL(ra.Resource, *acl); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *mockClusterAdmin) ListAcls(filter sarama.AclFilter) ([]sarama.ResourceAcls, error) {
	m.Lock()
	defer m.Unlock()

	acls := make([]sarama.ResourceAcls, 0, len(m.mockACLs))
	for _, acl := range m.mockACLs {
		acls = append(acls, *acl)
	}
	return acls, nil
}

func (m *mockClusterAdmin) DeleteACL(filter sarama.AclFilter, validateOnly bool) ([]sarama.MatchingAcl, error) {
	m.Lock()
	defer m.Unlock()

	if m.failOps {
		return []sarama.MatchingAcl{}, errors.New("bad create acl")
	}
	if filter.Principal != nil && *filter.Principal == "User:"+withErrorTopicName {
		return []sarama.MatchingAcl{{Err: sarama.ErrUnknown}}, nil
	}
	// Match bindings the way a broker applies an ACL filter.
	matches := []sarama.MatchingAcl{}
	for resource, resourceAcls := range m.mockACLs {
		if !aclResourceMatches(filter, resource) {
			continue
		}
		kept := make([]*sarama.Acl, 0, len(resourceAcls.Acls))
		for _, acl := range resourceAcls.Acls {
			if aclEntryMatches(filter, acl) {
				matches = append(matches, sarama.MatchingAcl{Resource: resource, Acl: *acl})
				continue
			}
			kept = append(kept, acl)
		}
		if validateOnly {
			continue
		}
		if len(kept) == 0 {
			delete(m.mockACLs, resource)
		} else {
			resourceAcls.Acls = kept
		}
	}
	return matches, nil
}

func aclResourceMatches(filter sarama.AclFilter, resource sarama.Resource) bool {
	return (filter.ResourceType == sarama.AclResourceAny || filter.ResourceType == resource.ResourceType) &&
		(filter.ResourceName == nil || *filter.ResourceName == resource.ResourceName) &&
		(filter.ResourcePatternTypeFilter == sarama.AclPatternAny || filter.ResourcePatternTypeFilter == resource.ResourcePatternType)
}

func aclEntryMatches(filter sarama.AclFilter, acl *sarama.Acl) bool {
	return (filter.Principal == nil || *filter.Principal == acl.Principal) &&
		(filter.Host == nil || *filter.Host == acl.Host) &&
		(filter.Operation == sarama.AclOperationAny || filter.Operation == acl.Operation) &&
		(filter.PermissionType == sarama.AclPermissionAny || filter.PermissionType == acl.PermissionType)
}

func (m *mockClusterAdmin) DescribeConfig(resource sarama.ConfigResource) ([]sarama.ConfigEntry, error) {
	return []sarama.ConfigEntry{}, nil
}

func (m *mockClusterAdmin) DescribeConfigs(resources []*sarama.ConfigResource, _ sarama.DescribeConfigsOptions) ([]*sarama.ConfigResourceResult, error) {
	results := make([]*sarama.ConfigResourceResult, len(resources))
	for i := range resources {
		results[i] = &sarama.ConfigResourceResult{}
	}
	return results, nil
}

func (m *mockClusterAdmin) Controller() (*sarama.Broker, error) {
	return &sarama.Broker{}, nil
}

func shallowCopy(original map[string]sarama.TopicDetail) map[string]sarama.TopicDetail {
	returnMap := make(map[string]sarama.TopicDetail, len(original))
	for k, v := range original {
		returnMap[k] = v
	}
	return returnMap
}
