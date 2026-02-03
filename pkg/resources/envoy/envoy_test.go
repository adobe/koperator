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

package envoy

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/banzaicloud/koperator/api/v1beta1"
	"github.com/banzaicloud/koperator/pkg/util"
)

// MockClient is a mock implementation of client.Client
type MockClient struct {
	mock.Mock
}

func (m *MockClient) Get(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
	args := m.Called(ctx, key, obj, opts)
	return args.Error(0)
}

func (m *MockClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	args := m.Called(ctx, list, opts)
	return args.Error(0)
}

func (m *MockClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	args := m.Called(ctx, obj, opts)
	return args.Error(0)
}

func (m *MockClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	args := m.Called(ctx, obj, opts)
	return args.Error(0)
}

func (m *MockClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	args := m.Called(ctx, obj, opts)
	return args.Error(0)
}

func (m *MockClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	args := m.Called(ctx, obj, patch, opts)
	return args.Error(0)
}

func (m *MockClient) DeleteAllOf(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
	args := m.Called(ctx, obj, opts)
	return args.Error(0)
}

func (m *MockClient) Status() client.StatusWriter {
	args := m.Called()
	return args.Get(0).(client.StatusWriter)
}

func (m *MockClient) Scheme() *runtime.Scheme {
	args := m.Called()
	return args.Get(0).(*runtime.Scheme)
}

func (m *MockClient) RESTMapper() meta.RESTMapper {
	args := m.Called()
	return args.Get(0).(meta.RESTMapper)
}

func (m *MockClient) GroupVersionKindFor(obj runtime.Object) (schema.GroupVersionKind, error) {
	args := m.Called(obj)
	return args.Get(0).(schema.GroupVersionKind), args.Error(1)
}

func (m *MockClient) IsObjectNamespaced(obj runtime.Object) (bool, error) {
	args := m.Called(obj)
	return args.Bool(0), args.Error(1)
}

func (m *MockClient) Apply(ctx context.Context, obj runtime.ApplyConfiguration, opts ...client.ApplyOption) error {
	args := m.Called(ctx, obj, opts)
	return args.Error(0)
}

func (m *MockClient) SubResource(subResource string) client.SubResourceClient {
	args := m.Called(subResource)
	return args.Get(0).(client.SubResourceClient)
}

func TestNew(t *testing.T) {
	mockClient := &MockClient{}
	cluster := &v1beta1.KafkaCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "test-namespace",
		},
	}

	reconciler := New(mockClient, cluster)

	assert.NotNil(t, reconciler)
	assert.Equal(t, mockClient, reconciler.Client)
	assert.Equal(t, cluster, reconciler.KafkaCluster)
}

func TestLabelsForEnvoyIngress(t *testing.T) {
	tests := []struct {
		name           string
		crName         string
		eLName         string
		expectedLabels map[string]string
	}{
		{
			name:   "basic labels",
			crName: "test-cluster",
			eLName: "external",
			expectedLabels: map[string]string{
				v1beta1.AppLabelKey:               "envoyingress",
				v1beta1.KafkaCRLabelKey:           "test-cluster",
				util.ExternalListenerLabelNameKey: "external",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			labels := labelsForEnvoyIngress(tt.crName, tt.eLName)
			assert.Equal(t, tt.expectedLabels, labels)
		})
	}
}

func TestLabelsForEnvoyIngressWithoutEListenerName(t *testing.T) {
	tests := []struct {
		name           string
		crName         string
		expectedLabels map[string]string
	}{
		{
			name:   "basic labels without external listener",
			crName: "test-cluster",
			expectedLabels: map[string]string{
				v1beta1.AppLabelKey:     "envoyingress",
				v1beta1.KafkaCRLabelKey: "test-cluster",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			labels := labelsForEnvoyIngressWithoutEListenerName(tt.crName)
			assert.Equal(t, tt.expectedLabels, labels)
		})
	}
}

func TestReconcile_WithEnvoyIngressController(t *testing.T) {
	mockClient := &MockClient{}
	cluster := &v1beta1.KafkaCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "test-namespace",
		},
		Spec: v1beta1.KafkaClusterSpec{
			ListenersConfig: v1beta1.ListenersConfig{
				ExternalListeners: []v1beta1.ExternalListenerConfig{
					{
						CommonListenerSpec: v1beta1.CommonListenerSpec{
							Name:          "external",
							ContainerPort: 9094,
						},
						AccessMethod:         corev1.ServiceTypeLoadBalancer,
						ExternalStartingPort: 19090,
					},
				},
			},
			IngressController: "envoy",
		},
	}

	reconciler := New(mockClient, cluster)

	// Mock the k8sutil.Reconcile calls
	mockClient.On("Get", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	mockClient.On("Create", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	mockClient.On("Update", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	log := logr.Discard()
	err := reconciler.Reconcile(log)
	assert.NoError(t, err)
}

func TestReconcile_WithRemoveUnusedIngressResources(t *testing.T) {
	mockClient := &MockClient{}
	cluster := &v1beta1.KafkaCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "test-namespace",
		},
		Spec: v1beta1.KafkaClusterSpec{
			ListenersConfig: v1beta1.ListenersConfig{
				ExternalListeners: []v1beta1.ExternalListenerConfig{
					{
						CommonListenerSpec: v1beta1.CommonListenerSpec{
							Name:          "external",
							ContainerPort: 9094,
						},
						AccessMethod:         corev1.ServiceTypeNodePort, // Not LoadBalancer
						ExternalStartingPort: 19090,
					},
				},
			},
			IngressController:            "nginx", // Not envoy
			RemoveUnusedIngressResources: true,
		},
	}

	reconciler := New(mockClient, cluster)

	// Mock the List call to return empty list (no resources to delete)
	mockClient.On("List", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	log := logr.Discard()
	err := reconciler.Reconcile(log)
	assert.NoError(t, err)
}

func TestReconcile_NoExternalListeners(t *testing.T) {
	mockClient := &MockClient{}
	cluster := &v1beta1.KafkaCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "test-namespace",
		},
		Spec: v1beta1.KafkaClusterSpec{
			ListenersConfig: v1beta1.ListenersConfig{
				ExternalListeners: nil, // No external listeners
			},
		},
	}

	reconciler := New(mockClient, cluster)

	log := logr.Discard()
	err := reconciler.Reconcile(log)
	assert.NoError(t, err)
}

func TestReconcile_ErrorHandling(t *testing.T) {
	mockClient := &MockClient{}
	cluster := &v1beta1.KafkaCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "test-namespace",
		},
		Spec: v1beta1.KafkaClusterSpec{
			ListenersConfig: v1beta1.ListenersConfig{
				ExternalListeners: []v1beta1.ExternalListenerConfig{
					{
						CommonListenerSpec: v1beta1.CommonListenerSpec{
							Name:          "external",
							ContainerPort: 9094,
						},
						AccessMethod:         corev1.ServiceTypeLoadBalancer,
						ExternalStartingPort: 19090,
					},
				},
			},
			IngressController: "envoy",
		},
	}

	reconciler := New(mockClient, cluster)

	// Mock k8sutil.Reconcile to return an error
	mockClient.On("Get", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(assert.AnError)

	log := logr.Discard()
	err := reconciler.Reconcile(log)
	assert.Error(t, err)
}
