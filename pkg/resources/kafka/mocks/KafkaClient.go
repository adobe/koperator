// Copyright 2024 Cisco Systems, Inc. and/or its affiliates
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
//

// Code generated by MockGen. DO NOT EDIT.
// Source: pkg/kafkaclient/client.go
//
// Generated by this command:
//
//	mockgen -copyright_file /Users/amuraru/go/src/github.com/koperator/hack/boilerplate/header.generated.txt -package mocks -source pkg/kafkaclient/client.go -destination pkg/resources/kafka/mocks/KafkaClient.go
//

// Package mocks is a generated GoMock package.
package mocks

import (
	reflect "reflect"

	sarama "github.com/IBM/sarama"
	v1alpha1 "github.com/banzaicloud/koperator/api/v1alpha1"
	kafkaclient "github.com/banzaicloud/koperator/pkg/kafkaclient"
	gomock "go.uber.org/mock/gomock"
)

// MockKafkaClient is a mock of KafkaClient interface.
type MockKafkaClient struct {
	ctrl     *gomock.Controller
	recorder *MockKafkaClientMockRecorder
}

// MockKafkaClientMockRecorder is the mock recorder for MockKafkaClient.
type MockKafkaClientMockRecorder struct {
	mock *MockKafkaClient
}

// NewMockKafkaClient creates a new mock instance.
func NewMockKafkaClient(ctrl *gomock.Controller) *MockKafkaClient {
	mock := &MockKafkaClient{ctrl: ctrl}
	mock.recorder = &MockKafkaClientMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockKafkaClient) EXPECT() *MockKafkaClientMockRecorder {
	return m.recorder
}

// AllOfflineReplicas mocks base method.
func (m *MockKafkaClient) AllOfflineReplicas() ([]int32, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "AllOfflineReplicas")
	ret0, _ := ret[0].([]int32)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// AllOfflineReplicas indicates an expected call of AllOfflineReplicas.
func (mr *MockKafkaClientMockRecorder) AllOfflineReplicas() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "AllOfflineReplicas", reflect.TypeOf((*MockKafkaClient)(nil).AllOfflineReplicas))
}

// AlterClusterWideConfig mocks base method.
func (m *MockKafkaClient) AlterClusterWideConfig(arg0 map[string]*string, arg1 bool) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "AlterClusterWideConfig", arg0, arg1)
	ret0, _ := ret[0].(error)
	return ret0
}

// AlterClusterWideConfig indicates an expected call of AlterClusterWideConfig.
func (mr *MockKafkaClientMockRecorder) AlterClusterWideConfig(arg0, arg1 any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "AlterClusterWideConfig", reflect.TypeOf((*MockKafkaClient)(nil).AlterClusterWideConfig), arg0, arg1)
}

// AlterPerBrokerConfig mocks base method.
func (m *MockKafkaClient) AlterPerBrokerConfig(arg0 int32, arg1 map[string]*string, arg2 bool) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "AlterPerBrokerConfig", arg0, arg1, arg2)
	ret0, _ := ret[0].(error)
	return ret0
}

// AlterPerBrokerConfig indicates an expected call of AlterPerBrokerConfig.
func (mr *MockKafkaClientMockRecorder) AlterPerBrokerConfig(arg0, arg1, arg2 any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "AlterPerBrokerConfig", reflect.TypeOf((*MockKafkaClient)(nil).AlterPerBrokerConfig), arg0, arg1, arg2)
}

// Brokers mocks base method.
func (m *MockKafkaClient) Brokers() map[int32]string {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Brokers")
	ret0, _ := ret[0].(map[int32]string)
	return ret0
}

// Brokers indicates an expected call of Brokers.
func (mr *MockKafkaClientMockRecorder) Brokers() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Brokers", reflect.TypeOf((*MockKafkaClient)(nil).Brokers))
}

// Close mocks base method.
func (m *MockKafkaClient) Close() error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Close")
	ret0, _ := ret[0].(error)
	return ret0
}

// Close indicates an expected call of Close.
func (mr *MockKafkaClientMockRecorder) Close() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Close", reflect.TypeOf((*MockKafkaClient)(nil).Close))
}

// CreateTopic mocks base method.
func (m *MockKafkaClient) CreateTopic(arg0 *kafkaclient.CreateTopicOptions) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CreateTopic", arg0)
	ret0, _ := ret[0].(error)
	return ret0
}

// CreateTopic indicates an expected call of CreateTopic.
func (mr *MockKafkaClientMockRecorder) CreateTopic(arg0 any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CreateTopic", reflect.TypeOf((*MockKafkaClient)(nil).CreateTopic), arg0)
}

// CreateUserACLs mocks base method.
func (m *MockKafkaClient) CreateUserACLs(arg0 v1alpha1.KafkaAccessType, arg1 v1alpha1.KafkaPatternType, arg2, arg3 string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CreateUserACLs", arg0, arg1, arg2, arg3)
	ret0, _ := ret[0].(error)
	return ret0
}

// CreateUserACLs indicates an expected call of CreateUserACLs.
func (mr *MockKafkaClientMockRecorder) CreateUserACLs(arg0, arg1, arg2, arg3 any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CreateUserACLs", reflect.TypeOf((*MockKafkaClient)(nil).CreateUserACLs), arg0, arg1, arg2, arg3)
}

// DeleteTopic mocks base method.
func (m *MockKafkaClient) DeleteTopic(arg0 string, arg1 bool) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DeleteTopic", arg0, arg1)
	ret0, _ := ret[0].(error)
	return ret0
}

// DeleteTopic indicates an expected call of DeleteTopic.
func (mr *MockKafkaClientMockRecorder) DeleteTopic(arg0, arg1 any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DeleteTopic", reflect.TypeOf((*MockKafkaClient)(nil).DeleteTopic), arg0, arg1)
}

// DeleteUserACLs mocks base method.
func (m *MockKafkaClient) DeleteUserACLs(arg0 string, arg1 v1alpha1.KafkaPatternType) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DeleteUserACLs", arg0, arg1)
	ret0, _ := ret[0].(error)
	return ret0
}

// DeleteUserACLs indicates an expected call of DeleteUserACLs.
func (mr *MockKafkaClientMockRecorder) DeleteUserACLs(arg0, arg1 any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DeleteUserACLs", reflect.TypeOf((*MockKafkaClient)(nil).DeleteUserACLs), arg0, arg1)
}

// DescribeCluster mocks base method.
func (m *MockKafkaClient) DescribeCluster() ([]*sarama.Broker, int32, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DescribeCluster")
	ret0, _ := ret[0].([]*sarama.Broker)
	ret1, _ := ret[1].(int32)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// DescribeCluster indicates an expected call of DescribeCluster.
func (mr *MockKafkaClientMockRecorder) DescribeCluster() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DescribeCluster", reflect.TypeOf((*MockKafkaClient)(nil).DescribeCluster))
}

// DescribeClusterWideConfig mocks base method.
func (m *MockKafkaClient) DescribeClusterWideConfig() ([]sarama.ConfigEntry, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DescribeClusterWideConfig")
	ret0, _ := ret[0].([]sarama.ConfigEntry)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// DescribeClusterWideConfig indicates an expected call of DescribeClusterWideConfig.
func (mr *MockKafkaClientMockRecorder) DescribeClusterWideConfig() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DescribeClusterWideConfig", reflect.TypeOf((*MockKafkaClient)(nil).DescribeClusterWideConfig))
}

// DescribePerBrokerConfig mocks base method.
func (m *MockKafkaClient) DescribePerBrokerConfig(arg0 int32, arg1 []string) ([]*sarama.ConfigEntry, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DescribePerBrokerConfig", arg0, arg1)
	ret0, _ := ret[0].([]*sarama.ConfigEntry)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// DescribePerBrokerConfig indicates an expected call of DescribePerBrokerConfig.
func (mr *MockKafkaClientMockRecorder) DescribePerBrokerConfig(arg0, arg1 any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DescribePerBrokerConfig", reflect.TypeOf((*MockKafkaClient)(nil).DescribePerBrokerConfig), arg0, arg1)
}

// DescribeTopic mocks base method.
func (m *MockKafkaClient) DescribeTopic(arg0 string) (*sarama.TopicMetadata, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DescribeTopic", arg0)
	ret0, _ := ret[0].(*sarama.TopicMetadata)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// DescribeTopic indicates an expected call of DescribeTopic.
func (mr *MockKafkaClientMockRecorder) DescribeTopic(arg0 any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DescribeTopic", reflect.TypeOf((*MockKafkaClient)(nil).DescribeTopic), arg0)
}

// EnsurePartitionCount mocks base method.
func (m *MockKafkaClient) EnsurePartitionCount(arg0 string, arg1 int32) (bool, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "EnsurePartitionCount", arg0, arg1)
	ret0, _ := ret[0].(bool)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// EnsurePartitionCount indicates an expected call of EnsurePartitionCount.
func (mr *MockKafkaClientMockRecorder) EnsurePartitionCount(arg0, arg1 any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "EnsurePartitionCount", reflect.TypeOf((*MockKafkaClient)(nil).EnsurePartitionCount), arg0, arg1)
}

// EnsureTopicConfig mocks base method.
func (m *MockKafkaClient) EnsureTopicConfig(arg0 string, arg1 map[string]*string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "EnsureTopicConfig", arg0, arg1)
	ret0, _ := ret[0].(error)
	return ret0
}

// EnsureTopicConfig indicates an expected call of EnsureTopicConfig.
func (mr *MockKafkaClientMockRecorder) EnsureTopicConfig(arg0, arg1 any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "EnsureTopicConfig", reflect.TypeOf((*MockKafkaClient)(nil).EnsureTopicConfig), arg0, arg1)
}

// GetTopic mocks base method.
func (m *MockKafkaClient) GetTopic(arg0 string) (*sarama.TopicDetail, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetTopic", arg0)
	ret0, _ := ret[0].(*sarama.TopicDetail)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetTopic indicates an expected call of GetTopic.
func (mr *MockKafkaClientMockRecorder) GetTopic(arg0 any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetTopic", reflect.TypeOf((*MockKafkaClient)(nil).GetTopic), arg0)
}

// ListTopics mocks base method.
func (m *MockKafkaClient) ListTopics() (map[string]sarama.TopicDetail, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ListTopics")
	ret0, _ := ret[0].(map[string]sarama.TopicDetail)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ListTopics indicates an expected call of ListTopics.
func (mr *MockKafkaClientMockRecorder) ListTopics() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ListTopics", reflect.TypeOf((*MockKafkaClient)(nil).ListTopics))
}

// ListUserACLs mocks base method.
func (m *MockKafkaClient) ListUserACLs() ([]sarama.ResourceAcls, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ListUserACLs")
	ret0, _ := ret[0].([]sarama.ResourceAcls)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ListUserACLs indicates an expected call of ListUserACLs.
func (mr *MockKafkaClientMockRecorder) ListUserACLs() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ListUserACLs", reflect.TypeOf((*MockKafkaClient)(nil).ListUserACLs))
}

// NumBrokers mocks base method.
func (m *MockKafkaClient) NumBrokers() int {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "NumBrokers")
	ret0, _ := ret[0].(int)
	return ret0
}

// NumBrokers indicates an expected call of NumBrokers.
func (mr *MockKafkaClientMockRecorder) NumBrokers() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "NumBrokers", reflect.TypeOf((*MockKafkaClient)(nil).NumBrokers))
}

// Open mocks base method.
func (m *MockKafkaClient) Open() error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Open")
	ret0, _ := ret[0].(error)
	return ret0
}

// Open indicates an expected call of Open.
func (mr *MockKafkaClientMockRecorder) Open() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Open", reflect.TypeOf((*MockKafkaClient)(nil).Open))
}

// OutOfSyncReplicas mocks base method.
func (m *MockKafkaClient) OutOfSyncReplicas() ([]int32, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "OutOfSyncReplicas")
	ret0, _ := ret[0].([]int32)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// OutOfSyncReplicas indicates an expected call of OutOfSyncReplicas.
func (mr *MockKafkaClientMockRecorder) OutOfSyncReplicas() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "OutOfSyncReplicas", reflect.TypeOf((*MockKafkaClient)(nil).OutOfSyncReplicas))
}

// TopicMetaToStatus mocks base method.
func (m *MockKafkaClient) TopicMetaToStatus(meta *sarama.TopicMetadata) *v1alpha1.KafkaTopicStatus {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "TopicMetaToStatus", meta)
	ret0, _ := ret[0].(*v1alpha1.KafkaTopicStatus)
	return ret0
}

// TopicMetaToStatus indicates an expected call of TopicMetaToStatus.
func (mr *MockKafkaClientMockRecorder) TopicMetaToStatus(meta any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "TopicMetaToStatus", reflect.TypeOf((*MockKafkaClient)(nil).TopicMetaToStatus), meta)
}
