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

	"github.com/IBM/sarama"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/banzaicloud/koperator/api/v1alpha1"
	"github.com/banzaicloud/koperator/api/v1beta1"
	"github.com/banzaicloud/koperator/pkg/kafkaclient"
)

func topicOwnershipFixture(t *testing.T, state v1alpha1.TopicState) (*runtime.Scheme, *v1beta1.KafkaCluster, *v1alpha1.KafkaTopic) {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, v1alpha1.AddToScheme(scheme))
	require.NoError(t, v1beta1.AddToScheme(scheme))
	cluster := createTestKafkaCluster("ownership-cluster", "default")
	topic := &v1alpha1.KafkaTopic{
		ObjectMeta: metav1.ObjectMeta{
			Name: "shared-name", Namespace: cluster.Namespace,
			Finalizers: []string{topicFinalizer, "example.com/unrelated"},
		},
		Spec: v1alpha1.KafkaTopicSpec{
			Name: "shared-name", Partitions: 1, ReplicationFactor: 1,
			ClusterRef: v1alpha1.ClusterReference{Name: cluster.Name},
		},
		Status: v1alpha1.KafkaTopicStatus{State: state},
	}
	return scheme, cluster, topic
}

func forbidKafkaClient(t *testing.T, reason string) {
	t.Helper()
	SetNewKafkaFromCluster(func(client.Client, *v1beta1.KafkaCluster) (kafkaclient.KafkaClient, func(), error) {
		t.Fatal(reason)
		return nil, nil, nil
	})
	t.Cleanup(func() { SetNewKafkaFromCluster(kafkaclient.NewFromCluster) })
}

func useKafkaClient(t *testing.T, broker kafkaclient.KafkaClient) {
	t.Helper()
	SetNewKafkaFromCluster(func(client.Client, *v1beta1.KafkaCluster) (kafkaclient.KafkaClient, func(), error) {
		return broker, func() {}, nil
	})
	t.Cleanup(func() { SetNewKafkaFromCluster(kafkaclient.NewFromCluster) })
}

func requireKafkaTopicExists(t *testing.T, broker kafkaclient.KafkaClient, name string) {
	t.Helper()
	existing, err := broker.GetTopic(name)
	require.NoError(t, err)
	require.NotNil(t, existing, "a Kafka topic the operator never created or adopted must be preserved")
}

func TestDeletingTopicWithoutOwnershipEvidenceKeepsKafkaTopic(t *testing.T) {
	ctx := context.Background()
	scheme, cluster, topic := topicOwnershipFixture(t, "")
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(topic, cluster).WithStatusSubresource(topic).Build()
	require.NoError(t, c.Delete(ctx, topic))
	broker, closeBroker, err := kafkaclient.NewMockFromCluster(c, cluster)
	require.NoError(t, err)
	t.Cleanup(closeBroker)
	require.NoError(t, broker.CreateTopic(&kafkaclient.CreateTopicOptions{Name: topic.Spec.Name, Partitions: 3, ReplicationFactor: 1}))
	forbidKafkaClient(t, "a topic that never reached Kafka must not need Kafka to be deleted")

	result, err := (&KafkaTopicReconciler{Client: c, Scheme: scheme}).Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(topic)})
	require.NoError(t, err)
	require.Zero(t, result.RequeueAfter)
	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(topic), topic))
	require.Equal(t, []string{"example.com/unrelated"}, topic.Finalizers)
	requireKafkaTopicExists(t, broker, topic.Spec.Name)
}

func TestFailedTopicAdoptionDoesNotAuthorizeDeletion(t *testing.T) {
	ctx := context.Background()
	scheme, cluster, topic := topicOwnershipFixture(t, "")
	topic.Finalizers = nil
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(topic, cluster).WithStatusSubresource(topic).Build()
	broker, closeBroker, err := kafkaclient.NewMockFromCluster(c, cluster)
	require.NoError(t, err)
	t.Cleanup(closeBroker)
	// Created independently after admission; the operator cannot adopt it.
	require.NoError(t, broker.CreateTopic(&kafkaclient.CreateTopicOptions{Name: topic.Spec.Name, Partitions: 3, ReplicationFactor: 1}))
	useKafkaClient(t, broker)
	req := ctrl.Request{NamespacedName: client.ObjectKeyFromObject(topic)}

	_, err = (&KafkaTopicReconciler{Client: c, Scheme: scheme}).Reconcile(ctx, req)
	require.Error(t, err, "adoption of the independent topic must fail")
	require.NoError(t, c.Get(ctx, req.NamespacedName, topic))
	require.Contains(t, topic.Finalizers, topicFinalizer)
	require.Empty(t, topic.Status.State, "a failed adoption is not evidence of ownership")

	topic.Finalizers = append(topic.Finalizers, "example.com/unrelated")
	require.NoError(t, c.Update(ctx, topic))
	require.NoError(t, c.Delete(ctx, topic))
	forbidKafkaClient(t, "an unowned topic must not need Kafka to be deleted")
	_, err = (&KafkaTopicReconciler{Client: c, Scheme: scheme}).Reconcile(ctx, req)
	require.NoError(t, err)
	require.NoError(t, c.Get(ctx, req.NamespacedName, topic))
	require.Equal(t, []string{"example.com/unrelated"}, topic.Finalizers)
	requireKafkaTopicExists(t, broker, topic.Spec.Name)
}

// racingCreateClient creates the topic independently just before the operator's create request.
type racingCreateClient struct {
	kafkaclient.KafkaClient
}

func (c *racingCreateClient) GetTopic(string) (*sarama.TopicDetail, error) { return nil, nil }

func (c *racingCreateClient) CreateTopic(opts *kafkaclient.CreateTopicOptions) error {
	_ = c.KafkaClient.CreateTopic(&kafkaclient.CreateTopicOptions{Name: opts.Name, Partitions: 3, ReplicationFactor: 1})
	return c.KafkaClient.CreateTopic(opts)
}

func TestTopicCreateConflictOwnership(t *testing.T) {
	for _, tt := range []struct {
		name        string
		state       v1alpha1.TopicState
		wantState   v1alpha1.TopicState
		wantDeleted bool
	}{
		// The first request found the name taken, so the topic belongs to someone else.
		{name: "first attempt", state: "", wantState: ""},
		// An earlier attempt may have created the topic before its response was lost.
		{name: "retried attempt", state: v1alpha1.TopicStateCreating, wantState: v1alpha1.TopicStateCreating, wantDeleted: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			scheme, cluster, topic := topicOwnershipFixture(t, tt.state)
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(topic, cluster).WithStatusSubresource(topic).Build()
			broker, closeBroker, err := kafkaclient.NewMockFromCluster(c, cluster)
			require.NoError(t, err)
			t.Cleanup(closeBroker)
			useKafkaClient(t, &racingCreateClient{KafkaClient: broker})
			req := ctrl.Request{NamespacedName: client.ObjectKeyFromObject(topic)}

			_, err = (&KafkaTopicReconciler{Client: c, Scheme: scheme}).Reconcile(ctx, req)
			require.Error(t, err)
			require.NoError(t, c.Get(ctx, req.NamespacedName, topic))
			require.Equal(t, tt.wantState, topic.Status.State)

			useKafkaClient(t, broker)
			require.NoError(t, c.Delete(ctx, topic))
			for attempt := 0; attempt < 3; attempt++ {
				_, err = (&KafkaTopicReconciler{Client: c, Scheme: scheme}).Reconcile(ctx, req)
				require.NoError(t, err)
			}
			require.NoError(t, c.Get(ctx, req.NamespacedName, topic))
			require.Equal(t, []string{"example.com/unrelated"}, topic.Finalizers)
			existing, err := broker.GetTopic(topic.Spec.Name)
			require.NoError(t, err)
			require.Equal(t, tt.wantDeleted, existing == nil)
		})
	}
}

// catalogCountingClient counts full topic listings and can fail targeted lookups.
type catalogCountingClient struct {
	kafkaclient.KafkaClient
	listings    int
	describeErr error
}

func (c *catalogCountingClient) ListTopics() (map[string]sarama.TopicDetail, error) {
	c.listings++
	return c.KafkaClient.ListTopics()
}

func (c *catalogCountingClient) GetTopic(name string) (*sarama.TopicDetail, error) {
	c.listings++
	return c.KafkaClient.GetTopic(name)
}

func (c *catalogCountingClient) DescribeTopic(name string) (*sarama.TopicMetadata, error) {
	if c.describeErr != nil {
		return nil, c.describeErr
	}
	return c.KafkaClient.DescribeTopic(name)
}

func TestTopicDeletionPollUsesTargetedMetadata(t *testing.T) {
	for _, tt := range []struct {
		name        string
		describeErr error
		wantDone    bool
	}{
		{name: "explicit unknown topic means absent", wantDone: true},
		{name: "other errors keep the finalizer", describeErr: sarama.ErrTopicAuthorizationFailed},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			scheme, cluster, topic := topicOwnershipFixture(t, v1alpha1.TopicStateDeleting)
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(topic, cluster).WithStatusSubresource(topic).Build()
			require.NoError(t, c.Delete(ctx, topic))
			broker, closeBroker, err := kafkaclient.NewMockFromCluster(c, cluster)
			require.NoError(t, err)
			t.Cleanup(closeBroker)
			spy := &catalogCountingClient{KafkaClient: broker, describeErr: tt.describeErr}
			useKafkaClient(t, spy)

			_, err = (&KafkaTopicReconciler{Client: c, Scheme: scheme}).Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(topic)})
			require.Zero(t, spy.listings, "the deletion poll must not list every topic in the cluster")
			require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(topic), topic))
			if tt.wantDone {
				require.NoError(t, err)
				require.Equal(t, []string{"example.com/unrelated"}, topic.Finalizers)
				return
			}
			require.ErrorIs(t, err, tt.describeErr)
			require.Contains(t, topic.Finalizers, topicFinalizer)
		})
	}
}
