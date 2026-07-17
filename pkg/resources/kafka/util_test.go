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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/banzaicloud/koperator/api/v1beta1"
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

func resourceListWithCPU(cpu string) corev1.ResourceList {
	return corev1.ResourceList{corev1.ResourceCPU: resource.MustParse(cpu)}
}

func TestSyncResourceRequests(t *testing.T) {
	cpu100m := resource.MustParse("100m")
	cpu200m := resource.MustParse("200m")
	cpu300m := resource.MustParse("300m")

	tests := []struct {
		name     string
		desired  []corev1.Container
		current  []corev1.Container
		baseline map[string]corev1.ResourceList
		want     corev1.ResourceList
	}{
		{
			name:     "no baseline recorded: CR value wins over live drift",
			desired:  []corev1.Container{{Name: "kafka", Resources: corev1.ResourceRequirements{Requests: resourceListWithCPU("100m")}}},
			current:  []corev1.Container{{Name: "kafka", Resources: corev1.ResourceRequirements{Requests: resourceListWithCPU("300m")}}},
			baseline: nil,
			want:     corev1.ResourceList{corev1.ResourceCPU: cpu100m},
		},
		{
			name:     "current matches baseline: CR hasn't changed, external drift preserved",
			desired:  []corev1.Container{{Name: "kafka", Resources: corev1.ResourceRequirements{Requests: resourceListWithCPU("100m")}}},
			current:  []corev1.Container{{Name: "kafka", Resources: corev1.ResourceRequirements{Requests: resourceListWithCPU("300m")}}},
			baseline: map[string]corev1.ResourceList{"kafka": resourceListWithCPU("100m")},
			want:     corev1.ResourceList{corev1.ResourceCPU: cpu300m},
		},
		{
			name:     "CR value differs from baseline: CR's new value wins over drift",
			desired:  []corev1.Container{{Name: "kafka", Resources: corev1.ResourceRequirements{Requests: resourceListWithCPU("200m")}}},
			current:  []corev1.Container{{Name: "kafka", Resources: corev1.ResourceRequirements{Requests: resourceListWithCPU("300m")}}},
			baseline: map[string]corev1.ResourceList{"kafka": resourceListWithCPU("100m")},
			want:     corev1.ResourceList{corev1.ResourceCPU: cpu200m},
		},
		{
			name:     "container absent from current: left unchanged",
			desired:  []corev1.Container{{Name: "kafka", Resources: corev1.ResourceRequirements{Requests: resourceListWithCPU("100m")}}},
			current:  []corev1.Container{{Name: "other", Resources: corev1.ResourceRequirements{Requests: resourceListWithCPU("300m")}}},
			baseline: nil,
			want:     corev1.ResourceList{corev1.ResourceCPU: cpu100m},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			desiredPod := &corev1.Pod{Spec: corev1.PodSpec{Containers: tt.desired}}
			currentPod := &corev1.Pod{Spec: corev1.PodSpec{Containers: tt.current}}
			var baseline *externalResourceBaseline
			if tt.baseline != nil {
				baseline = &externalResourceBaseline{ContainerRequests: tt.baseline}
			}

			syncResourceRequests(desiredPod, currentPod, baseline)

			got := desiredPod.Spec.Containers[0].Resources.Requests
			gotCPU := got[corev1.ResourceCPU]
			wantCPU := tt.want[corev1.ResourceCPU]
			if !gotCPU.Equal(wantCPU) {
				t.Errorf("expected cpu %s, got %s", wantCPU.String(), gotCPU.String())
			}
		})
	}

	t.Run("init containers synced independently from regular containers", func(t *testing.T) {
		desiredPod := &corev1.Pod{Spec: corev1.PodSpec{
			Containers:     []corev1.Container{{Name: "kafka", Resources: corev1.ResourceRequirements{Requests: resourceListWithCPU("100m")}}},
			InitContainers: []corev1.Container{{Name: "init-certs", Resources: corev1.ResourceRequirements{Requests: resourceListWithCPU("100m")}}},
		}}
		currentPod := &corev1.Pod{Spec: corev1.PodSpec{
			Containers:     []corev1.Container{{Name: "kafka", Resources: corev1.ResourceRequirements{Requests: resourceListWithCPU("300m")}}},
			InitContainers: []corev1.Container{{Name: "init-certs", Resources: corev1.ResourceRequirements{Requests: resourceListWithCPU("300m")}}},
		}}
		baseline := &externalResourceBaseline{
			ContainerRequests:     map[string]corev1.ResourceList{"kafka": resourceListWithCPU("100m")},
			InitContainerRequests: map[string]corev1.ResourceList{"init-certs": resourceListWithCPU("999m")},
		}

		syncResourceRequests(desiredPod, currentPod, baseline)

		gotContainerCPU := desiredPod.Spec.Containers[0].Resources.Requests[corev1.ResourceCPU]
		if !gotContainerCPU.Equal(cpu300m) {
			t.Errorf("expected container cpu 300m (matches baseline, drift preserved), got %s", gotContainerCPU.String())
		}
		gotInitCPU := desiredPod.Spec.InitContainers[0].Resources.Requests[corev1.ResourceCPU]
		if !gotInitCPU.Equal(cpu100m) {
			t.Errorf("expected init container cpu 100m (baseline mismatch, CR wins), got %s", gotInitCPU.String())
		}
	})
}

func newPodWithPreferredPodAffinity(terms ...corev1.WeightedPodAffinityTerm) *corev1.Pod {
	return &corev1.Pod{Spec: corev1.PodSpec{Affinity: &corev1.Affinity{
		PodAffinity: &corev1.PodAffinity{PreferredDuringSchedulingIgnoredDuringExecution: terms},
	}}}
}

func weightedPodAffinityTerm(weight int32, key string) corev1.WeightedPodAffinityTerm {
	return corev1.WeightedPodAffinityTerm{
		Weight: weight,
		PodAffinityTerm: corev1.PodAffinityTerm{
			LabelSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"key": key}},
			TopologyKey:   "kubernetes.io/hostname",
		},
	}
}

func TestSyncPodAffinities(t *testing.T) {
	crTerm := weightedPodAffinityTerm(10, "cr-declared")
	externalTerm := weightedPodAffinityTerm(20, "externally-added")

	t.Run("term added externally (absent from baseline) is preserved into desired", func(t *testing.T) {
		desiredPod := newPodWithPreferredPodAffinity(crTerm)
		currentPod := newPodWithPreferredPodAffinity(crTerm, externalTerm)
		baseline := &externalResourceBaseline{PodAffinityTerms: []corev1.WeightedPodAffinityTerm{crTerm}}

		syncPodAffinities(desiredPod, currentPod, baseline)

		got := desiredPod.Spec.Affinity.PodAffinity.PreferredDuringSchedulingIgnoredDuringExecution
		if len(got) != 2 {
			t.Fatalf("expected 2 terms after sync, got %d", len(got))
		}
	})

	t.Run("term the CR removed (present in baseline and current) is not resurrected", func(t *testing.T) {
		desiredPod := newPodWithPreferredPodAffinity() // CR no longer declares crTerm
		currentPod := newPodWithPreferredPodAffinity(crTerm)
		baseline := &externalResourceBaseline{PodAffinityTerms: []corev1.WeightedPodAffinityTerm{crTerm}}

		syncPodAffinities(desiredPod, currentPod, baseline)

		if desiredPod.Spec.Affinity != nil && desiredPod.Spec.Affinity.PodAffinity != nil &&
			len(desiredPod.Spec.Affinity.PodAffinity.PreferredDuringSchedulingIgnoredDuringExecution) != 0 {
			t.Errorf("expected crTerm to stay removed, got %+v", desiredPod.Spec.Affinity.PodAffinity.PreferredDuringSchedulingIgnoredDuringExecution)
		}
	})

	t.Run("no current affinity: no-op", func(t *testing.T) {
		desiredPod := &corev1.Pod{}
		currentPod := &corev1.Pod{}

		syncPodAffinities(desiredPod, currentPod, nil)

		if desiredPod.Spec.Affinity != nil {
			t.Errorf("expected no affinity to be set, got %+v", desiredPod.Spec.Affinity)
		}
	})
}

func newPodWithPreferredNodeAffinity(terms ...corev1.PreferredSchedulingTerm) *corev1.Pod {
	return &corev1.Pod{Spec: corev1.PodSpec{Affinity: &corev1.Affinity{
		NodeAffinity: &corev1.NodeAffinity{PreferredDuringSchedulingIgnoredDuringExecution: terms},
	}}}
}

func preferredSchedulingTerm(weight int32, key string) corev1.PreferredSchedulingTerm {
	return corev1.PreferredSchedulingTerm{
		Weight: weight,
		Preference: corev1.NodeSelectorTerm{
			MatchExpressions: []corev1.NodeSelectorRequirement{
				{Key: key, Operator: corev1.NodeSelectorOpExists},
			},
		},
	}
}

func TestSyncNodeAffinities(t *testing.T) {
	crTerm := preferredSchedulingTerm(10, "cr-declared")
	externalTerm := preferredSchedulingTerm(20, "scaleops.sh/node-packing")

	t.Run("term added externally (absent from baseline) is preserved into desired", func(t *testing.T) {
		desiredPod := newPodWithPreferredNodeAffinity(crTerm)
		currentPod := newPodWithPreferredNodeAffinity(crTerm, externalTerm)
		baseline := &externalResourceBaseline{NodeAffinityTerms: []corev1.PreferredSchedulingTerm{crTerm}}

		syncNodeAffinities(desiredPod, currentPod, baseline)

		got := desiredPod.Spec.Affinity.NodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution
		if len(got) != 2 {
			t.Fatalf("expected 2 terms after sync, got %d", len(got))
		}
	})

	t.Run("term the CR removed (present in baseline and current) is not resurrected", func(t *testing.T) {
		desiredPod := newPodWithPreferredNodeAffinity() // CR no longer declares crTerm
		currentPod := newPodWithPreferredNodeAffinity(crTerm)
		baseline := &externalResourceBaseline{NodeAffinityTerms: []corev1.PreferredSchedulingTerm{crTerm}}

		syncNodeAffinities(desiredPod, currentPod, baseline)

		if desiredPod.Spec.Affinity != nil && desiredPod.Spec.Affinity.NodeAffinity != nil &&
			len(desiredPod.Spec.Affinity.NodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution) != 0 {
			t.Errorf("expected crTerm to stay removed, got %+v", desiredPod.Spec.Affinity.NodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution)
		}
	})
}

func TestExternalResourceBaselineRoundTrip(t *testing.T) {
	original := &externalResourceBaseline{
		ContainerRequests: map[string]corev1.ResourceList{"kafka": resourceListWithCPU("100m")},
	}
	pod := &corev1.Pod{}

	if err := setExternalResourceBaseline(pod, original); err != nil {
		t.Fatalf("setExternalResourceBaseline failed: %v", err)
	}

	got, err := getExternalResourceBaseline(pod)
	if err != nil {
		t.Fatalf("getExternalResourceBaseline failed: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil baseline")
	}
	gotCPU := got.ContainerRequests["kafka"][corev1.ResourceCPU]
	wantCPU := original.ContainerRequests["kafka"][corev1.ResourceCPU]
	if !gotCPU.Equal(wantCPU) {
		t.Errorf("expected cpu %s, got %s", wantCPU.String(), gotCPU.String())
	}
}

func TestGetExternalResourceBaselineMissingAnnotation(t *testing.T) {
	pod := &corev1.Pod{}
	got, err := getExternalResourceBaseline(pod)
	if err != nil {
		t.Fatalf("expected no error for missing annotation, got %v", err)
	}
	if got != nil {
		t.Errorf("expected nil baseline when annotation is absent, got %+v", got)
	}
}
