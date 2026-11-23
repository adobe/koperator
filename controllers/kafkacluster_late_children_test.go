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

package controllers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/banzaicloud/koperator/api/v1alpha1"
	"github.com/banzaicloud/koperator/api/v1beta1"
)

func terminatingParentFixture(t *testing.T) (*runtime.Scheme, *v1beta1.KafkaCluster) {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, k8sscheme.AddToScheme(scheme))
	require.NoError(t, v1alpha1.AddToScheme(scheme))
	require.NoError(t, v1beta1.AddToScheme(scheme))
	cluster := createTestKafkaCluster("terminating-cluster", "default")
	cluster.Finalizers = []string{clusterFinalizer, clusterUsersFinalizer}
	cluster.DeletionTimestamp = &metav1.Time{Time: metav1.Now().Time}
	return scheme, cluster
}

func TestTopicIsNotEnrolledUnderTerminatingCluster(t *testing.T) {
	ctx := context.Background()
	scheme, cluster := terminatingParentFixture(t)
	topic := &v1alpha1.KafkaTopic{
		ObjectMeta: metav1.ObjectMeta{Name: "late-topic", Namespace: cluster.Namespace},
		Spec: v1alpha1.KafkaTopicSpec{
			Name: "late-topic", Partitions: 1, ReplicationFactor: 1,
			ClusterRef: v1alpha1.ClusterReference{Name: cluster.Name},
		},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(topic, cluster).WithStatusSubresource(topic).Build()
	forbidKafkaClient(t, "a topic must not be provisioned against a terminating cluster")

	_, err := (&KafkaTopicReconciler{Client: c, Scheme: scheme}).Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(topic)})
	require.NoError(t, err)
	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(topic), topic))
	require.Empty(t, topic.Finalizers, "a terminating cluster's delete sweep has already run")
	require.NotContains(t, topic.Labels, clusterRefLabel)
}

func TestTerminatingClusterDeletesChildTopicsLabeledAfterItsSweep(t *testing.T) {
	ctx := context.Background()
	scheme, cluster := terminatingParentFixture(t)
	child := func(name, clusterName string) *v1alpha1.KafkaTopic {
		return &v1alpha1.KafkaTopic{
			ObjectMeta: metav1.ObjectMeta{
				Name: name, Namespace: cluster.Namespace,
				Labels:     map[string]string{clusterRefLabel: clusterLabelString(cluster)},
				Finalizers: []string{"example.com/hold"},
			},
			Spec: v1alpha1.KafkaTopicSpec{Name: name, ClusterRef: v1alpha1.ClusterReference{Name: clusterName}},
		}
	}
	late := child("late-topic", cluster.Name)
	// The label is user metadata; a child of another cluster must not be deleted through it.
	mislabeled := child("other-cluster-topic", "other-cluster")
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster, late, mislabeled).Build()
	r := &KafkaClusterReconciler{Client: c, DirectClient: c, Namespaces: []string{cluster.Namespace}}

	result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(cluster)})
	require.NoError(t, err)
	require.Positive(t, result.RequeueAfter, "the cluster waits for its child topics")
	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(late), late))
	require.NotNil(t, late.DeletionTimestamp, "a live child found while waiting must be deleted")
	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(mislabeled), mislabeled))
	require.Nil(t, mislabeled.DeletionTimestamp, "a topic of another cluster must not be deleted")
}

func TestEnsureFinalizersSkipsWriteWhenComplete(t *testing.T) {
	ctx := context.Background()
	scheme, cluster := terminatingParentFixture(t)
	cluster.DeletionTimestamp = nil
	cluster.Finalizers = []string{clusterFinalizer, clusterTopicsFinalizer, clusterUsersFinalizer}
	base := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster).Build()
	updates := 0
	c := interceptor.NewClient(base, interceptor.Funcs{
		Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
			updates++
			return c.Update(ctx, obj, opts...)
		},
	})
	require.NoError(t, base.Get(ctx, client.ObjectKeyFromObject(cluster), cluster))

	got, err := (&KafkaClusterReconciler{Client: c, DirectClient: base}).ensureFinalizers(ctx, cluster)
	require.NoError(t, err)
	require.Zero(t, updates, "a cluster with every finalizer must not be written again")
	require.Equal(t, cluster.ResourceVersion, got.ResourceVersion)
}
