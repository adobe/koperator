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
	"encoding/json"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/banzaicloud/k8s-objectmatcher/patch"

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

// --- deletePreferredAffinities ---

func TestDeletePreferredAffinities(t *testing.T) {
	preferred := []interface{}{
		map[string]interface{}{
			"weight": float64(100),
			"preference": map[string]interface{}{
				"matchExpressions": []interface{}{
					map[string]interface{}{
						"key":      "scaleops.sh/node-packing",
						"operator": "In",
						"values":   []interface{}{"true"},
					},
				},
			},
		},
	}
	required := map[string]interface{}{
		"nodeSelectorTerms": []interface{}{
			map[string]interface{}{
				"matchExpressions": []interface{}{
					map[string]interface{}{
						"key":      "topology.kubernetes.io/zone",
						"operator": "In",
						"values":   []interface{}{"us-east-1a"},
					},
				},
			},
		},
	}

	makePod := func(affinity map[string]interface{}) []byte {
		pod := map[string]interface{}{
			"spec": map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{"name": "kafka", "image": "kafka:3.6"},
				},
			},
		}
		if affinity != nil {
			pod["spec"].(map[string]interface{})["affinity"] = affinity
		}
		b, _ := json.Marshal(pod)
		return b
	}

	getAffinity := func(result []byte) map[string]interface{} {
		var pod map[string]interface{}
		_ = json.Unmarshal(result, &pod)
		spec, _ := pod["spec"].(map[string]interface{})
		if spec == nil {
			return nil
		}
		aff, _ := spec["affinity"].(map[string]interface{})
		return aff
	}

	tests := []struct {
		name            string
		affinity        map[string]interface{}
		wantErr         bool
		checkAffinityFn func(t *testing.T, aff map[string]interface{})
	}{
		{
			name:     "no affinity field",
			affinity: nil,
			checkAffinityFn: func(t *testing.T, aff map[string]interface{}) {
				t.Helper()
				if aff != nil {
					t.Errorf("expected no affinity, got %v", aff)
				}
			},
		},
		{
			name: "only required node affinity kept",
			affinity: map[string]interface{}{
				"nodeAffinity": map[string]interface{}{
					"requiredDuringSchedulingIgnoredDuringExecution": required,
				},
			},
			checkAffinityFn: func(t *testing.T, aff map[string]interface{}) {
				t.Helper()
				na, _ := aff["nodeAffinity"].(map[string]interface{})
				if na == nil {
					t.Fatal("nodeAffinity missing")
				}
				if _, hasPreferred := na["preferredDuringSchedulingIgnoredDuringExecution"]; hasPreferred {
					t.Error("preferred should be absent")
				}
				if na["requiredDuringSchedulingIgnoredDuringExecution"] == nil {
					t.Error("required should be present")
				}
			},
		},
		{
			name: "node affinity preferred removed",
			affinity: map[string]interface{}{
				"nodeAffinity": map[string]interface{}{
					"preferredDuringSchedulingIgnoredDuringExecution": preferred,
				},
			},
			checkAffinityFn: func(t *testing.T, aff map[string]interface{}) {
				t.Helper()
				na, _ := aff["nodeAffinity"].(map[string]interface{})
				if _, ok := na["preferredDuringSchedulingIgnoredDuringExecution"]; ok {
					t.Error("preferred node affinity should be removed")
				}
			},
		},
		{
			name: "pod affinity preferred removed",
			affinity: map[string]interface{}{
				"podAffinity": map[string]interface{}{
					"preferredDuringSchedulingIgnoredDuringExecution": preferred,
				},
			},
			checkAffinityFn: func(t *testing.T, aff map[string]interface{}) {
				t.Helper()
				pa, _ := aff["podAffinity"].(map[string]interface{})
				if _, ok := pa["preferredDuringSchedulingIgnoredDuringExecution"]; ok {
					t.Error("preferred pod affinity should be removed")
				}
			},
		},
		{
			name: "pod anti-affinity preferred removed",
			affinity: map[string]interface{}{
				"podAntiAffinity": map[string]interface{}{
					"preferredDuringSchedulingIgnoredDuringExecution": preferred,
				},
			},
			checkAffinityFn: func(t *testing.T, aff map[string]interface{}) {
				t.Helper()
				paa, _ := aff["podAntiAffinity"].(map[string]interface{})
				if _, ok := paa["preferredDuringSchedulingIgnoredDuringExecution"]; ok {
					t.Error("preferred pod anti-affinity should be removed")
				}
			},
		},
		{
			name: "all three types: preferred removed, required kept",
			affinity: map[string]interface{}{
				"nodeAffinity": map[string]interface{}{
					"requiredDuringSchedulingIgnoredDuringExecution":  required,
					"preferredDuringSchedulingIgnoredDuringExecution": preferred,
				},
				"podAffinity": map[string]interface{}{
					"preferredDuringSchedulingIgnoredDuringExecution": preferred,
				},
				"podAntiAffinity": map[string]interface{}{
					"preferredDuringSchedulingIgnoredDuringExecution": preferred,
				},
			},
			checkAffinityFn: func(t *testing.T, aff map[string]interface{}) {
				t.Helper()
				na, _ := aff["nodeAffinity"].(map[string]interface{})
				if _, ok := na["preferredDuringSchedulingIgnoredDuringExecution"]; ok {
					t.Error("node preferred should be removed")
				}
				if na["requiredDuringSchedulingIgnoredDuringExecution"] == nil {
					t.Error("node required should be kept")
				}
				pa, _ := aff["podAffinity"].(map[string]interface{})
				if _, ok := pa["preferredDuringSchedulingIgnoredDuringExecution"]; ok {
					t.Error("pod preferred should be removed")
				}
				paa, _ := aff["podAntiAffinity"].(map[string]interface{})
				if _, ok := paa["preferredDuringSchedulingIgnoredDuringExecution"]; ok {
					t.Error("pod anti preferred should be removed")
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			input := makePod(tc.affinity)
			result, err := deletePreferredAffinities(input)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			tc.checkAffinityFn(t, getAffinity(result))
		})
	}

	t.Run("invalid JSON returns error", func(t *testing.T) {
		_, err := deletePreferredAffinities([]byte(`{invalid`))
		if err == nil {
			t.Fatal("expected error for invalid JSON")
		}
	})
}

// --- ignorePreferredAffinities CalculateOption (diff-level) ---

func baseKafkaPod(name string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "kafka", Image: "kafka:3.6"},
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
// derived from desiredPod, simulating what Koperator does at pod creation time
// (before the admission webhook runs and mutates the live pod).
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

func TestIgnorePreferredAffinities(t *testing.T) {
	t.Run("ScaleOps injects preferred node affinity, CR unchanged → empty patch", func(t *testing.T) {
		desired := baseKafkaPod("broker-0")
		current := baseKafkaPod("broker-0")
		setAnnotationFromDesired(t, desired, current)

		// ScaleOps injects preferred node affinity into the live pod.
		current.Spec.Affinity = &corev1.Affinity{NodeAffinity: scaleOpsNodePreferred()}

		result, err := patch.DefaultPatchMaker.Calculate(current, desired, ignorePreferredAffinities())
		if err != nil {
			t.Fatalf("Calculate: %v", err)
		}
		if !result.IsEmpty() {
			t.Errorf("expected empty patch, got: %s", result.Patch)
		}
	})

	t.Run("ScaleOps injects preferred pod affinity, CR unchanged → empty patch", func(t *testing.T) {
		desired := baseKafkaPod("broker-0")
		current := baseKafkaPod("broker-0")
		setAnnotationFromDesired(t, desired, current)

		current.Spec.Affinity = &corev1.Affinity{PodAffinity: scaleOpsPodPreferred()}

		result, err := patch.DefaultPatchMaker.Calculate(current, desired, ignorePreferredAffinities())
		if err != nil {
			t.Fatalf("Calculate: %v", err)
		}
		if !result.IsEmpty() {
			t.Errorf("expected empty patch, got: %s", result.Patch)
		}
	})

	t.Run("no affinities anywhere → empty patch", func(t *testing.T) {
		desired := baseKafkaPod("broker-0")
		current := baseKafkaPod("broker-0")
		setAnnotationFromDesired(t, desired, current)

		result, err := patch.DefaultPatchMaker.Calculate(current, desired, ignorePreferredAffinities())
		if err != nil {
			t.Fatalf("Calculate: %v", err)
		}
		if !result.IsEmpty() {
			t.Errorf("expected empty patch, got: %s", result.Patch)
		}
	})

	t.Run("operator adds required node affinity to CR → non-empty patch", func(t *testing.T) {
		// Annotation reflects the original desired pod with no affinity.
		original := baseKafkaPod("broker-0")
		current := baseKafkaPod("broker-0")
		setAnnotationFromDesired(t, original, current)

		// ScaleOps has injected preferred terms into the live pod.
		current.Spec.Affinity = &corev1.Affinity{NodeAffinity: scaleOpsNodePreferred()}

		// The operator now adds a required affinity to the CR.
		desired := baseKafkaPod("broker-0")
		desired.Spec.Affinity = &corev1.Affinity{
			NodeAffinity: &corev1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{
						{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{Key: "topology.kubernetes.io/zone", Operator: corev1.NodeSelectorOpIn, Values: []string{"us-east-1a"}},
							},
						},
					},
				},
			},
		}

		result, err := patch.DefaultPatchMaker.Calculate(current, desired, ignorePreferredAffinities())
		if err != nil {
			t.Fatalf("Calculate: %v", err)
		}
		if result.IsEmpty() {
			t.Error("expected non-empty patch: operator added required affinity")
		}
	})

	t.Run("ScaleOps preferred + operator required in CR → patch contains only required", func(t *testing.T) {
		// The operator starts with a required affinity in the CR.
		desired := baseKafkaPod("broker-0")
		desired.Spec.Affinity = &corev1.Affinity{
			NodeAffinity: &corev1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{
						{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{Key: "topology.kubernetes.io/zone", Operator: corev1.NodeSelectorOpIn, Values: []string{"us-east-1a"}},
							},
						},
					},
				},
			},
		}
		current := baseKafkaPod("broker-0")
		setAnnotationFromDesired(t, desired, current)

		// ScaleOps injects preferred terms on top.
		current.Spec.Affinity = &corev1.Affinity{
			NodeAffinity: &corev1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: desired.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution,
				PreferredDuringSchedulingIgnoredDuringExecution: scaleOpsNodePreferred().PreferredDuringSchedulingIgnoredDuringExecution,
			},
		}

		// CR is unchanged: same required, no preferred.
		result, err := patch.DefaultPatchMaker.Calculate(current, desired, ignorePreferredAffinities())
		if err != nil {
			t.Fatalf("Calculate: %v", err)
		}
		// Required affinity matches → patch should be empty (only preferred differs, which is ignored).
		if !result.IsEmpty() {
			t.Errorf("expected empty patch (required affinity unchanged, preferred ignored), got: %s", result.Patch)
		}
	})
}
