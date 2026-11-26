// Copyright © 2022 Cisco Systems, Inc. and/or its affiliates
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

package webhooks

import (
	"context"
	"errors"
	"strings"
	"testing"

	clusterregv1alpha1 "github.com/cisco-open/cluster-registry-controller/api/v1alpha1"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	//nolint:staticcheck

	runtimeClient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/go-logr/logr"

	"github.com/banzaicloud/koperator/api/v1alpha1"
	"github.com/banzaicloud/koperator/api/v1beta1"
	"github.com/banzaicloud/koperator/pkg/kafkaclient"
	"github.com/banzaicloud/koperator/pkg/util"
)

func newMockCluster() *v1beta1.KafkaCluster {
	return &v1beta1.KafkaCluster{
		TypeMeta: metav1.TypeMeta{
			Kind:       "KafkaCluster",
			APIVersion: "v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "test-namespace"},
		Spec:       v1beta1.KafkaClusterSpec{},
	}
}

func newMockTopic() *v1alpha1.KafkaTopic {
	return &v1alpha1.KafkaTopic{
		TypeMeta: metav1.TypeMeta{
			Kind:       "KafkaTopic",
			APIVersion: "v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{Name: "test-topic", Namespace: "test-namespace"},
		Spec: v1alpha1.KafkaTopicSpec{
			Name:              "test-topic",
			Partitions:        0,
			ReplicationFactor: 0,
			ClusterRef: v1alpha1.ClusterReference{
				Name:      "test-cluster",
				Namespace: "test-namespace",
			},
		},
	}
}

func newMockClients(cluster *v1beta1.KafkaCluster) (runtimeClient.WithWatch, kafkaclient.KafkaClient, func(client runtimeClient.Client, cluster *v1beta1.KafkaCluster) (kafkaclient.KafkaClient, func(), error)) {
	scheme := runtime.NewScheme()
	_ = v1beta1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	// Partially replicating the effects of AddKafkaTopicIndexers() from koperator/pkg/k8sutil/cache.go
	// for the controller-runtime/pkg/client/fake.Client used in the test.
	// We need this if we want to do Get/List operations with field selectors as options. See https://github.com/kubernetes-sigs/controller-runtime/pull/2025.
	// Currently we do such an operation in kafkatopic_validator.go in checkExistingKafkaTopicCRs() using "spec.name".
	nameIndexFunc := func(obj runtimeClient.Object) []string {
		return []string{obj.(*v1alpha1.KafkaTopic).Spec.Name}
	}
	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithIndex(&v1alpha1.KafkaTopic{}, "spec.name", nameIndexFunc).
		Build()

	kafkaClient, _, _ := kafkaclient.NewMockFromCluster(client, cluster)
	returnMockedKafkaClient := func(client runtimeClient.Client, cluster *v1beta1.KafkaCluster) (kafkaclient.KafkaClient, func(), error) {
		return kafkaClient, func() { _ = kafkaClient.Close() }, nil
	}
	return client, kafkaClient, returnMockedKafkaClient
}

func TestKafkaTopicValidateUpdateKafkaDependency(t *testing.T) {
	ctx := t.Context()
	cluster := newMockCluster()
	client, _, _ := newMockClients(cluster)
	require.NoError(t, client.Create(ctx, cluster))
	kafkaUnavailable := errors.New("Kafka unavailable")
	validator := KafkaTopicValidator{
		Client: client,
		Log:    logr.Discard(),
		NewKafkaFromCluster: func(runtimeClient.Client, *v1beta1.KafkaCluster) (kafkaclient.KafkaClient, func(), error) {
			return nil, nil, kafkaUnavailable
		},
	}
	for _, tc := range []struct {
		name          string
		change        func(oldTopic, newTopic *v1alpha1.KafkaTopic)
		requiresKafka bool
	}{
		{name: "label change", change: func(_, newTopic *v1alpha1.KafkaTopic) {
			newTopic.Labels = map[string]string{"example.com/team": "streaming"}
		}},
		{name: "unrelated annotation change", change: func(_, newTopic *v1alpha1.KafkaTopic) {
			newTopic.Annotations = map[string]string{"example.com/note": "updated"}
		}},
		{name: "finalizer addition", change: func(_, newTopic *v1alpha1.KafkaTopic) {
			newTopic.Finalizers = []string{"example.com/cleanup"}
		}},
		{name: "finalizer removal on a terminating topic", change: func(oldTopic, newTopic *v1alpha1.KafkaTopic) {
			deletedAt := metav1.Now()
			oldTopic.DeletionTimestamp, newTopic.DeletionTimestamp = &deletedAt, &deletedAt
			oldTopic.Finalizers = []string{"example.com/cleanup"}
		}},
		{name: "empty config omitted", change: func(oldTopic, _ *v1alpha1.KafkaTopic) {
			oldTopic.Spec.Config = map[string]string{}
		}},
		{name: "partition change", requiresKafka: true, change: func(_, newTopic *v1alpha1.KafkaTopic) {
			newTopic.Spec.Partitions++
		}},
		{name: "config change", requiresKafka: true, change: func(_, newTopic *v1alpha1.KafkaTopic) {
			newTopic.Spec.Config = map[string]string{"retention.ms": "1000"}
		}},
		{name: "managedBy changes", requiresKafka: true, change: func(oldTopic, newTopic *v1alpha1.KafkaTopic) {
			oldTopic.Annotations = map[string]string{TopicManagedByAnnotationKey: "external"}
			newTopic.Annotations = map[string]string{TopicManagedByAnnotationKey: TopicManagedByKoperatorAnnotationValue}
		}},
		{name: "empty managedBy added", requiresKafka: true, change: func(_, newTopic *v1alpha1.KafkaTopic) {
			newTopic.Annotations = map[string]string{TopicManagedByAnnotationKey: ""}
		}},
		{name: "Cluster Registry handoff on a terminating topic", requiresKafka: true, change: func(oldTopic, newTopic *v1alpha1.KafkaTopic) {
			deletedAt := metav1.Now()
			oldTopic.DeletionTimestamp, newTopic.DeletionTimestamp = &deletedAt, &deletedAt
			oldTopic.Finalizers, newTopic.Finalizers = []string{"example.com/cleanup"}, []string{"example.com/cleanup"}
			oldTopic.Annotations = map[string]string{clusterregv1alpha1.OwnershipAnnotation: "remote"}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			oldTopic := newMockTopic()
			oldTopic.Spec.Partitions = 2
			oldTopic.Spec.ReplicationFactor = 1
			newTopic := oldTopic.DeepCopy()
			tc.change(oldTopic, newTopic)

			_, err := validator.ValidateUpdate(ctx, oldTopic, newTopic)
			if tc.requiresKafka {
				require.True(t, apierrors.IsInternalError(err), "expected InternalError, got %v", err)
				require.ErrorContains(t, err, kafkaUnavailable.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestKafkaTopicValidateUpdateRetainsKubernetesValidation(t *testing.T) {
	for _, tc := range []struct {
		name         string
		invalidField string
	}{
		{name: "partitions", invalidField: "spec.partitions"},
		{name: "missing cluster", invalidField: "spec.clusterRef.name"},
		{name: "remote cluster", invalidField: "spec.clusterRef.name"},
		{name: "duplicate topic", invalidField: "spec.name"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			cluster := newMockCluster()
			client, _, _ := newMockClients(cluster)
			oldTopic := newMockTopic()
			oldTopic.Spec.Partitions = 2
			oldTopic.Spec.ReplicationFactor = 1
			switch tc.name {
			case "partitions":
				oldTopic.Spec.Partitions = 0
			case "remote cluster":
				cluster.Annotations = map[string]string{clusterregv1alpha1.OwnershipAnnotation: "remote"}
			case "duplicate topic":
				duplicate := oldTopic.DeepCopy()
				duplicate.Name = "duplicate"
				require.NoError(t, client.Create(ctx, duplicate))
			}
			if tc.name != "missing cluster" {
				require.NoError(t, client.Create(ctx, cluster))
			}
			newTopic := oldTopic.DeepCopy()
			newTopic.Labels = map[string]string{"example.com/team": "streaming"}
			validator := KafkaTopicValidator{
				Client: client,
				Log:    logr.Discard(),
				NewKafkaFromCluster: func(runtimeClient.Client, *v1beta1.KafkaCluster) (kafkaclient.KafkaClient, func(), error) {
					return nil, nil, errors.New("Kafka unavailable")
				},
			}
			_, err := validator.ValidateUpdate(ctx, oldTopic, newTopic)
			require.True(t, apierrors.IsInvalid(err), "expected Invalid, got %v", err)
			status := err.(apierrors.APIStatus).Status()
			require.Len(t, status.Details.Causes, 1)
			require.Equal(t, tc.invalidField, status.Details.Causes[0].Field)
		})
	}
}

func TestCheckKafkaTopicExist(t *testing.T) {
	cluster := newMockCluster()
	client, kafkaClient, returnMockedKafkaClient := newMockClients(cluster)

	kafkaTopicValidator := KafkaTopicValidator{
		Client:              client,
		NewKafkaFromCluster: returnMockedKafkaClient,
	}

	err := kafkaClient.CreateTopic(&kafkaclient.CreateTopicOptions{Name: "test-topic", ReplicationFactor: 1, Partitions: 2, Config: util.MapStringStringPointer(map[string]string{"testConfKey": "testConfVal"})})
	if err != nil {
		t.Error("creation of topic should have been successful")
	}

	testCases := []struct {
		testName       string
		kafkaTopic     v1alpha1.KafkaTopic
		expectedErrors []string
	}{
		{
			testName: "topic configuration is same and managedBy koperator",
			kafkaTopic: v1alpha1.KafkaTopic{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{TopicManagedByAnnotationKey: TopicManagedByKoperatorAnnotationValue},
				},
				Spec: v1alpha1.KafkaTopicSpec{
					Name:              "test-topic",
					Partitions:        2,
					ReplicationFactor: 1,
					Config:            map[string]string{"testConfKey": "testConfVal"},
					ClusterRef:        v1alpha1.ClusterReference{},
				},
			},
			expectedErrors: []string{},
		},
		{
			testName: "topic configuration is same and not managedBy koperator",
			kafkaTopic: v1alpha1.KafkaTopic{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
				Spec: v1alpha1.KafkaTopicSpec{
					Name:              "test-topic",
					Partitions:        2,
					ReplicationFactor: 1,
					Config:            map[string]string{"testConfKey": "testConfVal"},
					ClusterRef:        v1alpha1.ClusterReference{},
				},
			},
			expectedErrors: []string{TopicManagedByAnnotationKey},
		},
		{
			testName: "topic replication factor is different and managedBy koperator",
			kafkaTopic: v1alpha1.KafkaTopic{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{TopicManagedByAnnotationKey: TopicManagedByKoperatorAnnotationValue},
				},
				Spec: v1alpha1.KafkaTopicSpec{
					Name:              "test-topic",
					Partitions:        2,
					ReplicationFactor: 5,
					Config:            map[string]string{"testConfKey": "testConfVal"},
					ClusterRef:        v1alpha1.ClusterReference{},
				},
			},
			expectedErrors: []string{"replication"},
		},
		{
			testName: "topic partition is different and managedBy koperator",
			kafkaTopic: v1alpha1.KafkaTopic{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{TopicManagedByAnnotationKey: TopicManagedByKoperatorAnnotationValue},
				},
				Spec: v1alpha1.KafkaTopicSpec{
					Name:              "test-topic",
					Partitions:        5,
					ReplicationFactor: 1,
					Config:            map[string]string{"testConfKey": "testConfVal"},
					ClusterRef:        v1alpha1.ClusterReference{},
				},
			},
			expectedErrors: []string{"partition"},
		},

		{
			testName: "topic partition and replication is different and not managedBy koperator",
			kafkaTopic: v1alpha1.KafkaTopic{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{TopicManagedByAnnotationKey: "other"},
				},
				Spec: v1alpha1.KafkaTopicSpec{
					Name:              "test-topic",
					Partitions:        5,
					ReplicationFactor: 5,
					Config:            map[string]string{"testConfKey": "testConfVal"},
					ClusterRef:        v1alpha1.ClusterReference{},
				},
			},
			expectedErrors: []string{"replication", "partition", TopicManagedByAnnotationKey},
		},
		{
			testName: "topic configuration is different and managedBy koperator",
			kafkaTopic: v1alpha1.KafkaTopic{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{TopicManagedByAnnotationKey: TopicManagedByKoperatorAnnotationValue},
				},
				Spec: v1alpha1.KafkaTopicSpec{
					Name:              "test-topic",
					Partitions:        2,
					ReplicationFactor: 1,
					Config:            map[string]string{"testConfKey": "testConfVal1"},
					ClusterRef:        v1alpha1.ClusterReference{},
				},
			},
			expectedErrors: []string{"its configuration must be"},
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.testName, func(t *testing.T) {
			t.Parallel()
			fieldErrorList, err := kafkaTopicValidator.checkKafka(context.Background(), &testCase.kafkaTopic, cluster)
			if err != nil {
				t.Errorf("err should be nil, got: %s", err)
			}

			for _, err := range testCase.expectedErrors {
				if !strings.Contains(fieldErrorList.ToAggregate().Error(), err) {
					t.Errorf("missing error: %s from: %s", err, fieldErrorList.ToAggregate().Error())
				}
			}
		})
	}
}

func TestValidateTopic(t *testing.T) {
	topic := newMockTopic()
	cluster := newMockCluster()
	client, kafkaClient, returnMockedKafkaClient := newMockClients(cluster)

	kafkaTopicValidator := KafkaTopicValidator{
		Client:              client,
		NewKafkaFromCluster: returnMockedKafkaClient,
	}

	// Test non-existent kafka cluster
	fieldErrorList, err := kafkaTopicValidator.validateKafkaTopic(context.Background(), logr.Discard(), topic, true)
	if err != nil {
		t.Errorf("err should be nil, got: %s", err)
	}

	// Test kafka topic with invalid partitions, and replicas, and not found cluster
	if len(fieldErrorList) != 3 {
		t.Errorf("there should be 3 invalid field, got %d", len(fieldErrorList))
	}
	if !strings.Contains(fieldErrorList.ToAggregate().Error(), "not exist") {
		t.Error("Expected not found cluster")
	}

	if err := kafkaTopicValidator.Client.Create(context.TODO(), cluster); err != nil {
		t.Error("Expected no error, got:", err)
	}

	// set a valid partitions
	topic.Spec.Partitions = 2

	// Test kafka topic with invalid replication factor
	fieldErrorList, err = kafkaTopicValidator.validateKafkaTopic(context.Background(), logr.Discard(), topic, true)
	if err != nil {
		t.Errorf("err should be nil, got: %s", err)
	}
	if len(fieldErrorList) != 1 {
		t.Error("Expected not allowed due to invalid replication factor, got allowed")
	}

	// set a valid replication factor
	topic.Spec.ReplicationFactor = 1

	// test topic marked for deletion
	now := metav1.Now()
	topic.SetDeletionTimestamp(&now)
	fieldErrorList, err = kafkaTopicValidator.validateKafkaTopic(context.Background(), logr.Discard(), topic, true)
	if err != nil {
		t.Errorf("err should be nil, got: %s", err)
	}
	if len(fieldErrorList) != 0 {
		t.Error("Expected allowed due to topic marked for deletion")
	}
	// remove deletion timestamp
	topic.SetDeletionTimestamp(nil)

	// test cluster marked for deletion
	cluster.SetDeletionTimestamp(&now)

	fieldErrorList, err = kafkaTopicValidator.validateKafkaTopic(context.Background(), logr.Discard(), topic, true)
	if err != nil {
		t.Errorf("err should be nil, got: %s", err)
	}
	if len(fieldErrorList) != 0 {
		t.Error("Expected allowed due to cluster marked for deletion")
	}

	// remove deletion timestamp from cluster
	cluster.SetDeletionTimestamp(nil)
	if err := kafkaTopicValidator.Client.Update(context.TODO(), cluster); err != nil {
		t.Error("Expected no error, got:", err)
	}

	// test no rejection reasons
	fieldErrorList, err = kafkaTopicValidator.validateKafkaTopic(context.Background(), logr.Discard(), topic, true)
	if err != nil {
		t.Errorf("err should be nil, got: %s", err)
	}
	if len(fieldErrorList) != 0 {
		t.Error("Expected allowed due to no issues")
	}

	// Rejection reasons

	// Replication factor larger than num brokers
	topic.Spec.ReplicationFactor = 2
	fieldErrorList, err = kafkaTopicValidator.validateKafkaTopic(context.Background(), logr.Discard(), topic, true)
	if err != nil {
		t.Errorf("err should be nil, got: %s", err)
	}
	if len(fieldErrorList) != 1 {
		t.Error("Expected not allowed due to replication factor larger than num brokers, got allowed")
	}

	topic.Spec.ReplicationFactor = 1

	err = kafkaClient.CreateTopic(&kafkaclient.CreateTopicOptions{Name: "test-topic", ReplicationFactor: 1, Partitions: 2})
	if err != nil {
		t.Error("creation of topic should have been successful")
	}
	topic.Name = "test-topic"

	// Add topic and test existing topic reason
	if err := kafkaTopicValidator.Client.Create(context.TODO(), topic); err != nil {
		t.Error("Expected no error, got:", err)
	}

	// partition decrease attempt
	topic.Spec.Partitions = 1
	fieldErrorList, err = kafkaTopicValidator.validateKafkaTopic(context.Background(), logr.Discard(), topic, true)
	if err != nil {
		t.Errorf("err should be nil, got: %s", err)
	}

	if len(fieldErrorList) != 1 {
		t.Error("Expected not allowed due to partition decrease, got allowed")
	} else if !strings.Contains(fieldErrorList.ToAggregate().Error(), "kafka does not support decreasing partition count on an existing") {
		t.Error("Expected not allowed for reason: kafka does not support decreasing partition count")
	}

	// replication factor change attempt
	topic.Spec.Partitions = 2
	topic.Spec.ReplicationFactor = 2
	fieldErrorList, err = kafkaTopicValidator.validateKafkaTopic(context.Background(), logr.Discard(), topic, true)
	if err != nil {
		t.Errorf("err should be nil, got: %s", err)
	}
	if len(fieldErrorList) != 1 {
		t.Error("Expected not allowed due to replication factor change, got allowed")
	} else if !strings.Contains(fieldErrorList.ToAggregate().Error(), "kafka does not support changing the replication factor") {
		t.Error("Expected not allowed for reason: kafka does not support changing the replication factor")
	}
}
