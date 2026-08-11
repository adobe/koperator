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

package cruisecontrol

import (
	"strconv"
	"testing"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/banzaicloud/koperator/api/v1alpha1"
	"github.com/banzaicloud/koperator/api/v1beta1"
	"github.com/banzaicloud/koperator/pkg/kafkaclient"
)

func newIssue301TestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 to scheme: %v", err)
	}
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add appsv1 to scheme: %v", err)
	}
	if err := v1beta1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add v1beta1 to scheme: %v", err)
	}
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add v1alpha1 to scheme: %v", err)
	}
	return scheme
}

func newIssue301TestKafkaCluster(name, namespace string, brokerIDs []int32) *v1beta1.KafkaCluster {
	quantity := resource.MustParse("10Gi")
	brokers := make([]v1beta1.Broker, 0, len(brokerIDs))
	brokerStates := make(map[string]v1beta1.BrokerState, len(brokerIDs))
	for _, id := range brokerIDs {
		brokers = append(brokers, v1beta1.Broker{
			Id: id,
			BrokerConfig: &v1beta1.BrokerConfig{
				StorageConfigs: []v1beta1.StorageConfig{
					{
						MountPath: "/kafka-logs",
						PvcSpec: &corev1.PersistentVolumeClaimSpec{
							Resources: corev1.VolumeResourceRequirements{
								Requests: corev1.ResourceList{corev1.ResourceStorage: quantity},
							},
						},
					},
				},
			},
		})
		brokerStates[strconv.Itoa(int(id))] = v1beta1.BrokerState{
			GracefulActionState: v1beta1.GracefulActionState{
				CruiseControlState: v1beta1.GracefulUpscaleSucceeded,
			},
		}
	}

	return &v1beta1.KafkaCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: v1beta1.KafkaClusterSpec{
			Brokers: brokers,
			ListenersConfig: v1beta1.ListenersConfig{
				InternalListeners: []v1beta1.InternalListenerConfig{
					{
						CommonListenerSpec: v1beta1.CommonListenerSpec{
							Type:                            "plaintext",
							Name:                            "internal",
							ContainerPort:                   29092,
							UsedForInnerBrokerCommunication: true,
						},
					},
				},
			},
			// auto-create short-circuits generateCCTopic before it needs a live Kafka client,
			// so the fake KafkaClientProvider passed to New() is never invoked.
			ReadOnlyConfig: "cruise.control.metrics.topic.auto.create=true",
		},
		Status: v1beta1.KafkaClusterStatus{
			BrokersState:             brokerStates,
			CruiseControlTopicStatus: v1beta1.CruiseControlTopicReady,
		},
	}
}

// TestReconcile_ScaleUpStillChangesPodTemplateWhenStaticOptInSet is an integration-level guard
// test for https://github.com/adobe/koperator/issues/301, driving the real CruiseControl
// Reconciler.Reconcile function (the same code path the KafkaClusterReconciler calls in
// production) against a fake Kubernetes API. It asserts that operators who explicitly opt into
// "static" mode still see the Deployment roll on capacity.json changes, so this fix does not
// silently remove that capability. The default (no restart on scale) path is already covered by
// the unit-level TestGeneratePodAnnotations_CapacityChangeDoesNotAffectPodTemplateByDefault
// (deployment_test.go) and by the existing envtest suite in controllers/tests/, so it is not
// duplicated here.
func TestReconcile_ScaleUpStillChangesPodTemplateWhenStaticOptInSet(t *testing.T) {
	scheme := newIssue301TestScheme(t)
	log := logr.Discard()
	namespace := "kafka-issue301-static"
	name := "kafkacluster-issue301-static"

	kafkaClusterBefore := newIssue301TestKafkaCluster(name, namespace, []int32{0, 1, 2})
	kafkaClusterBefore.Spec.CruiseControlConfig.CruiseControlAnnotations = map[string]string{
		capacityConfigAnnotation: string(staticCapacityConfig),
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(kafkaClusterBefore).Build()

	reconciler := New(fakeClient, kafkaClusterBefore, kafkaclient.NewMockProvider())
	if err := reconciler.Reconcile(log); err != nil {
		t.Fatalf("Reconcile (before scale-up) failed: %v", err)
	}

	deployment := &appsv1.Deployment{}
	deploymentKey := types.NamespacedName{Namespace: namespace, Name: name + "-cruisecontrol"}
	if err := fakeClient.Get(t.Context(), deploymentKey, deployment); err != nil {
		t.Fatalf("failed to get CruiseControl deployment after first reconcile: %v", err)
	}
	hashBefore, ok := deployment.Spec.Template.Annotations["cruiseControlCapacity.json"]
	if !ok {
		t.Fatalf("expected cruiseControlCapacity.json annotation to be present under static opt-in")
	}

	kafkaClusterAfter := kafkaClusterBefore.DeepCopy()
	kafkaClusterAfter.Spec.Brokers = append(kafkaClusterAfter.Spec.Brokers, v1beta1.Broker{
		Id: 3,
		BrokerConfig: &v1beta1.BrokerConfig{
			StorageConfigs: []v1beta1.StorageConfig{
				{
					MountPath: "/kafka-logs",
					PvcSpec: &corev1.PersistentVolumeClaimSpec{
						Resources: corev1.VolumeResourceRequirements{
							Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("10Gi")},
						},
					},
				},
			},
		},
	})
	kafkaClusterAfter.Status.BrokersState["3"] = v1beta1.BrokerState{
		GracefulActionState: v1beta1.GracefulActionState{
			CruiseControlState: v1beta1.GracefulUpscaleSucceeded,
		},
	}

	reconciler = New(fakeClient, kafkaClusterAfter, kafkaclient.NewMockProvider())
	if err := reconciler.Reconcile(log); err != nil {
		t.Fatalf("Reconcile (after scale-up) failed: %v", err)
	}

	if err := fakeClient.Get(t.Context(), deploymentKey, deployment); err != nil {
		t.Fatalf("failed to get CruiseControl deployment after second reconcile: %v", err)
	}
	hashAfter, ok := deployment.Spec.Template.Annotations["cruiseControlCapacity.json"]
	if !ok {
		t.Fatalf("expected cruiseControlCapacity.json annotation to remain present under static opt-in")
	}
	if hashBefore == hashAfter {
		t.Fatalf("expected capacity hash to change across scale-up when static opt-in is set")
	}
}
