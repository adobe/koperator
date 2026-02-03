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

package k8sutil

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/banzaicloud/koperator/api/v1beta1"
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

func TestReconcile_CreateResource(t *testing.T) {
	mockClient := &MockClient{}
	cluster := &v1beta1.KafkaCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "test-namespace",
		},
	}

	// Create a test ConfigMap
	desired := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-configmap",
			Namespace: "test-namespace",
		},
		Data: map[string]string{
			"key": "value",
		},
	}

	// Mock Get to return NotFound error (resource doesn't exist)
	mockClient.On("Get", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(errors.NewNotFound(schema.GroupResource{}, "test-configmap"))
	// Mock Create to succeed
	mockClient.On("Create", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	err := Reconcile(logr.Discard(), mockClient, desired, cluster)
	assert.NoError(t, err)

	// Verify that Create was called
	mockClient.AssertCalled(t, "Create", mock.Anything, mock.Anything, mock.Anything)
}

func TestReconcile_UpdateResource(t *testing.T) {
	mockClient := &MockClient{}
	cluster := &v1beta1.KafkaCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "test-namespace",
		},
	}

	// Create a test ConfigMap
	desired := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-configmap",
			Namespace: "test-namespace",
		},
		Data: map[string]string{
			"key": "value",
		},
	}

	// Mock Get to return existing resource
	existing := desired.DeepCopy()
	existing.Data["existing-key"] = "existing-value"
	mockClient.On("Get", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		obj := args.Get(2).(client.Object)
		// Set the existing data
		if cm, ok := obj.(*corev1.ConfigMap); ok {
			cm.Data = map[string]string{
				"existing-key": "existing-value",
			}
		}
	})
	// Mock Update to succeed
	mockClient.On("Update", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	err := Reconcile(logr.Discard(), mockClient, desired, cluster)
	assert.NoError(t, err)
}

func TestReconcile_ErrorHandling(t *testing.T) {
	mockClient := &MockClient{}
	cluster := &v1beta1.KafkaCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "test-namespace",
		},
	}

	// Create a test ConfigMap
	desired := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-configmap",
			Namespace: "test-namespace",
		},
		Data: map[string]string{
			"key": "value",
		},
	}

	// Mock Get to return an error
	mockClient.On("Get", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(assert.AnError)

	err := Reconcile(logr.Discard(), mockClient, desired, cluster)
	assert.Error(t, err)
}

func TestReconcile_CreateErrorHandling(t *testing.T) {
	mockClient := &MockClient{}
	cluster := &v1beta1.KafkaCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "test-namespace",
		},
	}

	// Create a test ConfigMap
	desired := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-configmap",
			Namespace: "test-namespace",
		},
		Data: map[string]string{
			"key": "value",
		},
	}

	// Mock Get to return NotFound error (resource doesn't exist)
	mockClient.On("Get", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(assert.AnError)
	// Mock Create to return an error
	mockClient.On("Create", mock.Anything, mock.Anything, mock.Anything).Return(assert.AnError)

	err := Reconcile(logr.Discard(), mockClient, desired, cluster)
	assert.Error(t, err)
}

func TestReconcile_UpdateErrorHandling(t *testing.T) {
	mockClient := &MockClient{}
	cluster := &v1beta1.KafkaCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "test-namespace",
		},
	}

	// Create a test ConfigMap
	desired := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-configmap",
			Namespace: "test-namespace",
		},
		Data: map[string]string{
			"key": "value",
		},
	}

	// Mock Get to return a different existing resource (so update is needed)
	existing := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-configmap",
			Namespace: "test-namespace",
		},
		Data: map[string]string{
			"key": "different-value",
		},
	}
	mockClient.On("Get", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		obj := args.Get(2).(*corev1.ConfigMap)
		*obj = *existing
	})
	// Mock Update to return an error
	mockClient.On("Update", mock.Anything, mock.Anything, mock.Anything).Return(assert.AnError)

	err := Reconcile(logr.Discard(), mockClient, desired, cluster)
	assert.Error(t, err)
}
