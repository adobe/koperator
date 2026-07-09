// Copyright © 2023 Cisco Systems, Inc. and/or its affiliates
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

package kafka

import (
	"encoding/base64"
	"reflect"
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/banzaicloud/k8s-objectmatcher/patch"

	"github.com/banzaicloud/koperator/api/v1beta1"
	"github.com/banzaicloud/koperator/pkg/resources"
)

func TestGenerateClusterID(t *testing.T) {
	// one random cluster ID serves for the entire Kafka cluster, therefore testing 100000 cluster IDs should be enough
	numOfIDs := 100000
	test := make(map[string]bool, numOfIDs)
	for i := 0; i < numOfIDs; i++ {
		clusterID := generateRandomClusterID()
		_, err := base64.URLEncoding.DecodeString(clusterID)
		if err != nil {
			t.Errorf("expected nil error, got: %v", err)
		}

		if test[clusterID] {
			t.Error("expected random cluster ID that does not collide with previous ones")
		}

		// mark the map to note that this cluster ID has been generated
		test[clusterID] = true
	}
}

//nolint:funlen
func TestGenerateQuorumVoters(t *testing.T) {
	kafkaCluster := &v1beta1.KafkaCluster{}

	tests := []struct {
		testName             string
		brokers              []v1beta1.Broker
		listenersStatuses    map[string]v1beta1.ListenerStatusList
		expectedQuorumVoters []string
	}{
		{
			testName: "brokers with ascending order by IDs; controller listener statuses has the same order as brokers",
			brokers: []v1beta1.Broker{
				{
					Id: int32(0),
					BrokerConfig: &v1beta1.BrokerConfig{
						Roles: []string{"broker"},
					},
				},
				{
					Id: int32(10),
					BrokerConfig: &v1beta1.BrokerConfig{
						Roles: []string{"broker"},
					},
				},
				{
					Id: int32(20),
					BrokerConfig: &v1beta1.BrokerConfig{
						Roles: []string{"broker"},
					},
				},
				{
					Id: int32(30),
					BrokerConfig: &v1beta1.BrokerConfig{
						Roles: []string{"broker"},
					},
				},
				{
					Id: int32(40),
					BrokerConfig: &v1beta1.BrokerConfig{
						Roles: []string{"controller"},
					},
				},
				{
					Id: int32(50),
					BrokerConfig: &v1beta1.BrokerConfig{
						Roles: []string{"controller"},
					},
				},
				{
					Id: int32(60),
					BrokerConfig: &v1beta1.BrokerConfig{
						Roles: []string{"controller"},
					},
				},
			},
			listenersStatuses: map[string]v1beta1.ListenerStatusList{
				"test-listener": {
					{
						Name:    "broker-0",
						Address: "fakeKafka-0.fakeKafka-headless.default.svc.cluster.local:29093",
					},
					{
						Name:    "broker-10",
						Address: "fakeKafka-10.fakeKafka-headless.default.svc.cluster.local:29093",
					},
					{
						Name:    "broker-20",
						Address: "fakeKafka-20.fakeKafka-headless.default.svc.cluster.local:29093",
					},
					{
						Name:    "broker-30",
						Address: "fakeKafka-30.fakeKafka-headless.default.svc.cluster.local:29093",
					},
					{
						Name:    "broker-40",
						Address: "fakeKafka-40.fakeKafka-headless.default.svc.cluster.local:29093",
					},
					{
						Name:    "broker-50",
						Address: "fakeKafka-50.fakeKafka-headless.default.svc.cluster.local:29093",
					},
					{
						Name:    "broker-60",
						Address: "fakeKafka-60.fakeKafka-headless.default.svc.cluster.local:29093",
					},
				},
			},
			expectedQuorumVoters: []string{
				"40@fakeKafka-40.fakeKafka-headless.default.svc.cluster.local:29093",
				"50@fakeKafka-50.fakeKafka-headless.default.svc.cluster.local:29093",
				"60@fakeKafka-60.fakeKafka-headless.default.svc.cluster.local:29093"},
		},
		{
			testName: "brokers with descending order by IDs; controller listener statuses has the same order as brokers",
			brokers: []v1beta1.Broker{
				{
					Id: int32(60),
					BrokerConfig: &v1beta1.BrokerConfig{
						Roles: []string{"broker"},
					},
				},
				{
					Id: int32(50),
					BrokerConfig: &v1beta1.BrokerConfig{
						Roles: []string{"broker"},
					},
				},
				{
					Id: int32(40),
					BrokerConfig: &v1beta1.BrokerConfig{
						Roles: []string{"broker"},
					},
				},
				{
					Id: int32(30),
					BrokerConfig: &v1beta1.BrokerConfig{
						Roles: []string{"broker"},
					},
				},
				{
					Id: int32(20),
					BrokerConfig: &v1beta1.BrokerConfig{
						Roles: []string{"controller"},
					},
				},
				{
					Id: int32(10),
					BrokerConfig: &v1beta1.BrokerConfig{
						Roles: []string{"controller"},
					},
				},
				{
					Id: int32(0),
					BrokerConfig: &v1beta1.BrokerConfig{
						Roles: []string{"controller"},
					},
				},
			},
			listenersStatuses: map[string]v1beta1.ListenerStatusList{
				"test-listener": {
					{
						Name:    "broker-60",
						Address: "fakeKafka-60.fakeKafka-headless.default.svc.cluster.local:29093",
					},
					{
						Name:    "broker-50",
						Address: "fakeKafka-50.fakeKafka-headless.default.svc.cluster.local:29093",
					},
					{
						Name:    "broker-40",
						Address: "fakeKafka-40.fakeKafka-headless.default.svc.cluster.local:29093",
					},
					{
						Name:    "broker-30",
						Address: "fakeKafka-30.fakeKafka-headless.default.svc.cluster.local:29093",
					},
					{
						Name:    "broker-20",
						Address: "fakeKafka-20.fakeKafka-headless.default.svc.cluster.local:29093",
					},
					{
						Name:    "broker-10",
						Address: "fakeKafka-10.fakeKafka-headless.default.svc.cluster.local:29093",
					},
					{
						Name:    "broker-0",
						Address: "fakeKafka-0.fakeKafka-headless.default.svc.cluster.local:29093",
					},
				},
			},
			expectedQuorumVoters: []string{
				"0@fakeKafka-0.fakeKafka-headless.default.svc.cluster.local:29093",
				"10@fakeKafka-10.fakeKafka-headless.default.svc.cluster.local:29093",
				"20@fakeKafka-20.fakeKafka-headless.default.svc.cluster.local:29093"},
		},
		{
			testName: "brokers with ascending order by IDs; controller listener statuses has the opposite order as brokers",
			brokers: []v1beta1.Broker{
				{
					Id: int32(0),
					BrokerConfig: &v1beta1.BrokerConfig{
						Roles: []string{"broker"},
					},
				},
				{
					Id: int32(10),
					BrokerConfig: &v1beta1.BrokerConfig{
						Roles: []string{"broker"},
					},
				},
				{
					Id: int32(20),
					BrokerConfig: &v1beta1.BrokerConfig{
						Roles: []string{"broker"},
					},
				},
				{
					Id: int32(30),
					BrokerConfig: &v1beta1.BrokerConfig{
						Roles: []string{"broker"},
					},
				},
				{
					Id: int32(40),
					BrokerConfig: &v1beta1.BrokerConfig{
						Roles: []string{"controller"},
					},
				},
				{
					Id: int32(50),
					BrokerConfig: &v1beta1.BrokerConfig{
						Roles: []string{"controller"},
					},
				},
				{
					Id: int32(60),
					BrokerConfig: &v1beta1.BrokerConfig{
						Roles: []string{"controller"},
					},
				},
			},
			listenersStatuses: map[string]v1beta1.ListenerStatusList{
				"test-listener": {
					{
						Name:    "broker-60",
						Address: "fakeKafka-60.fakeKafka-headless.default.svc.cluster.local:29093",
					},
					{
						Name:    "broker-50",
						Address: "fakeKafka-50.fakeKafka-headless.default.svc.cluster.local:29093",
					},
					{
						Name:    "broker-40",
						Address: "fakeKafka-40.fakeKafka-headless.default.svc.cluster.local:29093",
					},
					{
						Name:    "broker-30",
						Address: "fakeKafka-30.fakeKafka-headless.default.svc.cluster.local:29093",
					},
					{
						Name:    "broker-20",
						Address: "fakeKafka-20.fakeKafka-headless.default.svc.cluster.local:29093",
					},
					{
						Name:    "broker-10",
						Address: "fakeKafka-10.fakeKafka-headless.default.svc.cluster.local:29093",
					},
					{
						Name:    "broker-0",
						Address: "fakeKafka-0.fakeKafka-headless.default.svc.cluster.local:29093",
					},
				},
			},
			expectedQuorumVoters: []string{
				"40@fakeKafka-40.fakeKafka-headless.default.svc.cluster.local:29093",
				"50@fakeKafka-50.fakeKafka-headless.default.svc.cluster.local:29093",
				"60@fakeKafka-60.fakeKafka-headless.default.svc.cluster.local:29093"},
		},
		{
			testName: "brokers and controller listener statuses with random order",
			brokers: []v1beta1.Broker{
				{
					Id: int32(100),
					BrokerConfig: &v1beta1.BrokerConfig{
						Roles: []string{"broker", "controller"},
					},
				},
				{
					Id: int32(50),
					BrokerConfig: &v1beta1.BrokerConfig{
						Roles: []string{"broker"},
					},
				},
				{
					Id: int32(80),
					BrokerConfig: &v1beta1.BrokerConfig{
						Roles: []string{"controller"},
					},
				},
				{
					Id: int32(30),
					BrokerConfig: &v1beta1.BrokerConfig{
						Roles: []string{"broker"},
					},
				},
				{
					Id: int32(90),
					BrokerConfig: &v1beta1.BrokerConfig{
						Roles: []string{"controller"},
					},
				},
				{
					Id: int32(40),
					BrokerConfig: &v1beta1.BrokerConfig{
						Roles: []string{"broker"},
					},
				},
				{
					Id: int32(60),
					BrokerConfig: &v1beta1.BrokerConfig{
						Roles: []string{"controller"},
					},
				},
			},
			listenersStatuses: map[string]v1beta1.ListenerStatusList{
				"test-listener": {
					{
						Name:    "broker-30",
						Address: "fakeKafka-30.fakeKafka-headless.default.svc.cluster.local:29093",
					},
					{
						Name:    "broker-50",
						Address: "fakeKafka-50.fakeKafka-headless.default.svc.cluster.local:29093",
					},
					{
						Name:    "broker-60",
						Address: "fakeKafka-60.fakeKafka-headless.default.svc.cluster.local:29093",
					},
					{
						Name:    "broker-100",
						Address: "fakeKafka-100.fakeKafka-headless.default.svc.cluster.local:29093",
					},
					{
						Name:    "broker-80",
						Address: "fakeKafka-80.fakeKafka-headless.default.svc.cluster.local:29093",
					},
					{
						Name:    "broker-90",
						Address: "fakeKafka-90.fakeKafka-headless.default.svc.cluster.local:29093",
					},
					{
						Name:    "broker-40",
						Address: "fakeKafka-40.fakeKafka-headless.default.svc.cluster.local:29093",
					},
				},
			},
			expectedQuorumVoters: []string{
				"60@fakeKafka-60.fakeKafka-headless.default.svc.cluster.local:29093",
				"80@fakeKafka-80.fakeKafka-headless.default.svc.cluster.local:29093",
				"90@fakeKafka-90.fakeKafka-headless.default.svc.cluster.local:29093",
				"100@fakeKafka-100.fakeKafka-headless.default.svc.cluster.local:29093",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.testName, func(t *testing.T) {
			kafkaCluster.Spec.Brokers = test.brokers
			gotQuorumVoters, err := generateQuorumVoters(kafkaCluster, test.listenersStatuses)
			if err != nil {
				t.Error(err)
			}
			if !reflect.DeepEqual(gotQuorumVoters, test.expectedQuorumVoters) {
				t.Error("Expected:", test.expectedQuorumVoters, "Got:", gotQuorumVoters)
			}
		})
	}
}

// --- podSpecIntentChanged & tainted-broker restart ---

func baseKafkaPod() *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "broker-0",
			Namespace: "default",
			Labels:    map[string]string{"brokerId": "0"},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "kafka",
					Image: "kafka:3.6",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("1"),
							corev1.ResourceMemory: resource.MustParse("4Gi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("4"),
							corev1.ResourceMemory: resource.MustParse("4Gi"),
						},
					},
				},
			},
		},
	}
}

func scaleOpsNodePreferred() *corev1.NodeAffinity {
	return &corev1.NodeAffinity{
		PreferredDuringSchedulingIgnoredDuringExecution: []corev1.PreferredSchedulingTerm{
			{
				Weight: 100,
				Preference: corev1.NodeSelectorTerm{
					MatchExpressions: []corev1.NodeSelectorRequirement{
						{Key: "scaleops.sh/node-packing", Operator: corev1.NodeSelectorOpIn, Values: []string{"true"}},
					},
				},
			},
		},
	}
}

func scaleOpsPodPreferred() *corev1.PodAffinity {
	return &corev1.PodAffinity{
		PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
			{
				Weight: 50,
				PodAffinityTerm: corev1.PodAffinityTerm{
					LabelSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"scaleops.sh/managed": "true"},
					},
					TopologyKey: "kubernetes.io/hostname",
				},
			},
		},
	}
}

// setAnnotationFromDesired stamps currentPod with the last-applied annotation
// derived from desiredPod, simulating what koperator does at pod creation time
// (before any admission webhook runs and mutates the live pod).
func setAnnotationFromDesired(t *testing.T, desiredPod, currentPod *corev1.Pod) {
	t.Helper()
	tmp := desiredPod.DeepCopy()
	if err := patch.DefaultAnnotator.SetLastAppliedAnnotation(tmp); err != nil {
		t.Fatalf("SetLastAppliedAnnotation: %v", err)
	}
	if currentPod.Annotations == nil {
		currentPod.Annotations = make(map[string]string)
	}
	currentPod.Annotations[patch.LastAppliedConfig] = tmp.Annotations[patch.LastAppliedConfig]
}

// TestPodSpecIntentChanged verifies the intent-aware diff: a rolling upgrade is
// triggered only when koperator's own desired spec differs from what it last
// applied. Mutations made to the live pod by admission controllers must never
// register as a change; intentional CR edits always must.
func TestPodSpecIntentChanged(t *testing.T) {
	// scaleOpsMutateResources simulates a VPA/admission controller rewriting the
	// live pod's kafka request after admission.
	scaleOpsMutateResources := func(p *corev1.Pod) {
		p.Spec.Containers[0].Resources.Requests[corev1.ResourceCPU] = resource.MustParse("392m")
	}

	t.Run("in sync, nothing changed -> false", func(t *testing.T) {
		desired := baseKafkaPod()
		current := baseKafkaPod()
		setAnnotationFromDesired(t, desired, current)

		changed, _, err := podSpecIntentChanged(current, desired)
		if err != nil {
			t.Fatalf("podSpecIntentChanged: %v", err)
		}
		if changed {
			t.Error("expected no change when current matches last-applied and the CR is unchanged")
		}
	})

	t.Run("admission controller rewrote resources, CR unchanged -> false", func(t *testing.T) {
		desired := baseKafkaPod()
		current := baseKafkaPod()
		setAnnotationFromDesired(t, desired, current)
		scaleOpsMutateResources(current)

		changed, _, err := podSpecIntentChanged(current, desired)
		if err != nil {
			t.Fatalf("podSpecIntentChanged: %v", err)
		}
		if changed {
			t.Error("expected no change: a resource mutation on the live pod must not trigger a restart")
		}
	})

	t.Run("admission controller injected preferred affinity, CR unchanged -> false", func(t *testing.T) {
		desired := baseKafkaPod()
		current := baseKafkaPod()
		setAnnotationFromDesired(t, desired, current)
		current.Spec.Affinity = &corev1.Affinity{
			NodeAffinity: scaleOpsNodePreferred(),
			PodAffinity:  scaleOpsPodPreferred(),
		}

		changed, _, err := podSpecIntentChanged(current, desired)
		if err != nil {
			t.Fatalf("podSpecIntentChanged: %v", err)
		}
		if changed {
			t.Error("expected no change: webhook-injected preferred affinity must not trigger a restart")
		}
	})

	t.Run("operator changed resources in the CR -> true", func(t *testing.T) {
		// last-applied reflects the old CR (cpu=1); a VPA then mutated the live pod.
		lastApplied := baseKafkaPod()
		current := baseKafkaPod()
		setAnnotationFromDesired(t, lastApplied, current)
		scaleOpsMutateResources(current)

		// operator bumps the request in the CR.
		desired := baseKafkaPod()
		desired.Spec.Containers[0].Resources.Requests[corev1.ResourceCPU] = resource.MustParse("2")

		changed, _, err := podSpecIntentChanged(current, desired)
		if err != nil {
			t.Fatalf("podSpecIntentChanged: %v", err)
		}
		if !changed {
			t.Error("expected change: the operator edited resources in the CR")
		}
	})

	t.Run("operator changed a soft (preferred) affinity in the CR -> true", func(t *testing.T) {
		// koperator emits a soft nodeAffinity; last-applied records it, and the
		// live pod carries it too.
		lastApplied := baseKafkaPod()
		lastApplied.Spec.Affinity = &corev1.Affinity{NodeAffinity: scaleOpsNodePreferred()}
		current := baseKafkaPod()
		setAnnotationFromDesired(t, lastApplied, current)
		current.Spec.Affinity = &corev1.Affinity{NodeAffinity: scaleOpsNodePreferred()}

		// operator changes the preferred weight in the CR.
		desired := baseKafkaPod()
		np := scaleOpsNodePreferred()
		np.PreferredDuringSchedulingIgnoredDuringExecution[0].Weight = 10
		desired.Spec.Affinity = &corev1.Affinity{NodeAffinity: np}

		changed, _, err := podSpecIntentChanged(current, desired)
		if err != nil {
			t.Fatalf("podSpecIntentChanged: %v", err)
		}
		if !changed {
			t.Error("expected change: the operator edited a soft affinity in the CR (the old strip approach swallowed this)")
		}
	})
}

// TestParkedBrokerRestartsIndependentOfIntent verifies that the shredder
// park mechanism (label on the live pod matched by TaintedBrokersSelector) still
// triggers a restart under the intent-aware diff. The label lives only on the
// live pod, so podSpecIntentChanged does not see it; isPodTainted does.
func TestParkedBrokerRestartsIndependentOfIntent(t *testing.T) {
	const parkLabel = "shredder.ethos.adobe.net/upgrade-status"

	r := &Reconciler{
		Reconciler: resources.Reconciler{
			KafkaCluster: &v1beta1.KafkaCluster{
				Spec: v1beta1.KafkaClusterSpec{
					TaintedBrokersSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{parkLabel: "parked"},
					},
				},
			},
		},
	}

	desired := baseKafkaPod()
	current := baseKafkaPod()
	setAnnotationFromDesired(t, desired, current)
	// Shredder parks the broker by labeling the live pod.
	current.Labels[parkLabel] = "parked"

	// The intent diff sees no change: the label is only on the live pod.
	changed, _, err := podSpecIntentChanged(current, desired)
	if err != nil {
		t.Fatalf("podSpecIntentChanged: %v", err)
	}
	if changed {
		t.Error("a parked label on the live pod must not register as a CR intent change")
	}
	// But the tainted-broker selector still selects it for restart.
	if !r.isPodTainted(logr.Discard(), current) {
		t.Error("expected a parked broker to be tainted and therefore restarted")
	}

	// Sanity: a non-parked broker is neither changed nor tainted.
	cleanCur := baseKafkaPod()
	setAnnotationFromDesired(t, baseKafkaPod(), cleanCur)
	if r.isPodTainted(logr.Discard(), cleanCur) {
		t.Error("expected a non-parked broker to not be tainted")
	}
}
