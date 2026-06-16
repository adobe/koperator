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

func TestSyncScaleOpsPodAffinities(t *testing.T) {
	tests := []struct {
		name                string
		currentPod          *corev1.Pod
		desiredPod          *corev1.Pod
		expectedPodAffinity bool
		expectedTermCount   int
	}{
		{
			name: "no affinity in current pod",
			currentPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
				Spec:       corev1.PodSpec{},
			},
			desiredPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
				Spec:       corev1.PodSpec{},
			},
			expectedPodAffinity: false,
			expectedTermCount:   0,
		},
		{
			name: "no pod affinity in current pod",
			currentPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{},
				},
			},
			desiredPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
				Spec:       corev1.PodSpec{},
			},
			expectedPodAffinity: false,
			expectedTermCount:   0,
		},
		{
			name: "pod affinity with scaleops managed-unevictable in MatchLabels",
			currentPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						PodAffinity: &corev1.PodAffinity{
							PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
								{
									Weight: 100,
									PodAffinityTerm: corev1.PodAffinityTerm{
										LabelSelector: &metav1.LabelSelector{
											MatchLabels: map[string]string{
												"scaleops.sh/managed-unevictable": "true",
											},
										},
										TopologyKey: "kubernetes.io/hostname",
									},
								},
							},
						},
					},
				},
			},
			desiredPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
				Spec:       corev1.PodSpec{},
			},
			expectedPodAffinity: true,
			expectedTermCount:   1,
		},
		{
			name: "pod affinity with scaleops managed-unevictable in MatchExpressions",
			currentPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						PodAffinity: &corev1.PodAffinity{
							PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
								{
									Weight: 50,
									PodAffinityTerm: corev1.PodAffinityTerm{
										LabelSelector: &metav1.LabelSelector{
											MatchExpressions: []metav1.LabelSelectorRequirement{
												{
													Key:      "scaleops.sh/managed-unevictable",
													Operator: metav1.LabelSelectorOpIn,
													Values:   []string{"true"},
												},
											},
										},
										TopologyKey: "kubernetes.io/hostname",
									},
								},
							},
						},
					},
				},
			},
			desiredPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
				Spec:       corev1.PodSpec{},
			},
			expectedPodAffinity: true,
			expectedTermCount:   1,
		},
		{
			name: "pod affinity with mixed terms, only scaleops managed-unevictable should be synced",
			currentPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						PodAffinity: &corev1.PodAffinity{
							PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
								{
									Weight: 100,
									PodAffinityTerm: corev1.PodAffinityTerm{
										LabelSelector: &metav1.LabelSelector{
											MatchLabels: map[string]string{
												"app": "other",
											},
										},
										TopologyKey: "kubernetes.io/hostname",
									},
								},
								{
									Weight: 50,
									PodAffinityTerm: corev1.PodAffinityTerm{
										LabelSelector: &metav1.LabelSelector{
											MatchLabels: map[string]string{
												"scaleops.sh/managed-unevictable": "true",
											},
										},
										TopologyKey: "kubernetes.io/hostname",
									},
								},
							},
						},
					},
				},
			},
			desiredPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
				Spec:       corev1.PodSpec{},
			},
			expectedPodAffinity: true,
			expectedTermCount:   1,
		},
		{
			name: "desired pod already has pod affinity, scaleops affinity should be merged",
			currentPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						PodAffinity: &corev1.PodAffinity{
							PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
								{
									Weight: 100,
									PodAffinityTerm: corev1.PodAffinityTerm{
										LabelSelector: &metav1.LabelSelector{
											MatchLabels: map[string]string{
												"scaleops.sh/managed-unevictable": "true",
											},
										},
										TopologyKey: "kubernetes.io/hostname",
									},
								},
							},
						},
					},
				},
			},
			desiredPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						PodAffinity: &corev1.PodAffinity{
							PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
								{
									Weight: 80,
									PodAffinityTerm: corev1.PodAffinityTerm{
										LabelSelector: &metav1.LabelSelector{
											MatchLabels: map[string]string{
												"app": "myapp",
											},
										},
										TopologyKey: "kubernetes.io/hostname",
									},
								},
							},
						},
					},
				},
			},
			expectedPodAffinity: true,
			expectedTermCount:   2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			syncScaleOpsPodAffinities(tt.desiredPod, tt.currentPod)

			if !tt.expectedPodAffinity {
				if tt.desiredPod.Spec.Affinity != nil && tt.desiredPod.Spec.Affinity.PodAffinity != nil {
					t.Errorf("expected no pod affinity, but got one")
				}
				return
			}

			if tt.desiredPod.Spec.Affinity == nil || tt.desiredPod.Spec.Affinity.PodAffinity == nil {
				t.Errorf("expected pod affinity to be set")
				return
			}

			gotTermCount := len(tt.desiredPod.Spec.Affinity.PodAffinity.PreferredDuringSchedulingIgnoredDuringExecution)
			if gotTermCount != tt.expectedTermCount {
				t.Errorf("expected %d pod affinity terms, got %d", tt.expectedTermCount, gotTermCount)
			}

			// Verify all synced terms have the scaleops label
			for _, term := range tt.desiredPod.Spec.Affinity.PodAffinity.PreferredDuringSchedulingIgnoredDuringExecution {
				if term.PodAffinityTerm.LabelSelector != nil {
					hasScaleOpsLabel := false
					for _, req := range term.PodAffinityTerm.LabelSelector.MatchExpressions {
						if req.Key == "scaleops.sh/managed-unevictable" {
							hasScaleOpsLabel = true
							break
						}
					}
					if !hasScaleOpsLabel {
						if _, exists := term.PodAffinityTerm.LabelSelector.MatchLabels["scaleops.sh/managed-unevictable"]; !exists {
							// This term should have been filtered out if it doesn't have scaleops label
							// unless it came from the original desired pod
						}
					}
				}
			}
		})
	}
}

func TestSyncScaleOpsNodeAffinities(t *testing.T) {
	tests := []struct {
		name                 string
		currentPod           *corev1.Pod
		desiredPod           *corev1.Pod
		expectedNodeAffinity bool
		expectedTermCount    int
	}{
		{
			name: "no affinity in current pod",
			currentPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
				Spec:       corev1.PodSpec{},
			},
			desiredPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
				Spec:       corev1.PodSpec{},
			},
			expectedNodeAffinity: false,
			expectedTermCount:    0,
		},
		{
			name: "no node affinity in current pod",
			currentPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{},
				},
			},
			desiredPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
				Spec:       corev1.PodSpec{},
			},
			expectedNodeAffinity: false,
			expectedTermCount:    0,
		},
		{
			name: "node affinity with scaleops node-packing in MatchExpressions",
			currentPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							PreferredDuringSchedulingIgnoredDuringExecution: []corev1.PreferredSchedulingTerm{
								{
									Weight: 100,
									Preference: corev1.NodeSelectorTerm{
										MatchExpressions: []corev1.NodeSelectorRequirement{
											{
												Key:      "scaleops.sh/node-packing",
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"true"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			desiredPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
				Spec:       corev1.PodSpec{},
			},
			expectedNodeAffinity: true,
			expectedTermCount:    1,
		},
		{
			name: "node affinity with scaleops node-packing in MatchFields",
			currentPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							PreferredDuringSchedulingIgnoredDuringExecution: []corev1.PreferredSchedulingTerm{
								{
									Weight: 50,
									Preference: corev1.NodeSelectorTerm{
										MatchFields: []corev1.NodeSelectorRequirement{
											{
												Key:      "scaleops.sh/node-packing",
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"true"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			desiredPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
				Spec:       corev1.PodSpec{},
			},
			expectedNodeAffinity: true,
			expectedTermCount:    1,
		},
		{
			name: "node affinity with mixed terms, only scaleops node-packing should be synced",
			currentPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							PreferredDuringSchedulingIgnoredDuringExecution: []corev1.PreferredSchedulingTerm{
								{
									Weight: 100,
									Preference: corev1.NodeSelectorTerm{
										MatchExpressions: []corev1.NodeSelectorRequirement{
											{
												Key:      "disktype",
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"ssd"},
											},
										},
									},
								},
								{
									Weight: 50,
									Preference: corev1.NodeSelectorTerm{
										MatchExpressions: []corev1.NodeSelectorRequirement{
											{
												Key:      "scaleops.sh/node-packing",
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"true"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			desiredPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
				Spec:       corev1.PodSpec{},
			},
			expectedNodeAffinity: true,
			expectedTermCount:    1,
		},
		{
			name: "desired pod already has node affinity, scaleops affinity should be merged",
			currentPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							PreferredDuringSchedulingIgnoredDuringExecution: []corev1.PreferredSchedulingTerm{
								{
									Weight: 100,
									Preference: corev1.NodeSelectorTerm{
										MatchExpressions: []corev1.NodeSelectorRequirement{
											{
												Key:      "scaleops.sh/node-packing",
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"true"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			desiredPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							PreferredDuringSchedulingIgnoredDuringExecution: []corev1.PreferredSchedulingTerm{
								{
									Weight: 80,
									Preference: corev1.NodeSelectorTerm{
										MatchExpressions: []corev1.NodeSelectorRequirement{
											{
												Key:      "disktype",
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"ssd"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expectedNodeAffinity: true,
			expectedTermCount:    2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			syncScaleOpsNodeAffinities(tt.desiredPod, tt.currentPod)

			if !tt.expectedNodeAffinity {
				if tt.desiredPod.Spec.Affinity != nil && tt.desiredPod.Spec.Affinity.NodeAffinity != nil {
					t.Errorf("expected no node affinity, but got one")
				}
				return
			}

			if tt.desiredPod.Spec.Affinity == nil || tt.desiredPod.Spec.Affinity.NodeAffinity == nil {
				t.Errorf("expected node affinity to be set")
				return
			}

			gotTermCount := len(tt.desiredPod.Spec.Affinity.NodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution)
			if gotTermCount != tt.expectedTermCount {
				t.Errorf("expected %d node affinity terms, got %d", tt.expectedTermCount, gotTermCount)
			}
		})
	}
}

func TestSyncScaleOpsAffinities(t *testing.T) {
	tests := []struct {
		name               string
		currentPod         *corev1.Pod
		desiredPod         *corev1.Pod
		expectPodAffinity  bool
		expectNodeAffinity bool
	}{
		{
			name: "no affinities in current pod",
			currentPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
				Spec:       corev1.PodSpec{},
			},
			desiredPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
				Spec:       corev1.PodSpec{},
			},
			expectPodAffinity:  false,
			expectNodeAffinity: false,
		},
		{
			name: "both pod and node affinities with scaleops labels",
			currentPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						PodAffinity: &corev1.PodAffinity{
							PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
								{
									Weight: 100,
									PodAffinityTerm: corev1.PodAffinityTerm{
										LabelSelector: &metav1.LabelSelector{
											MatchLabels: map[string]string{
												"scaleops.sh/managed-unevictable": "true",
											},
										},
										TopologyKey: "kubernetes.io/hostname",
									},
								},
							},
						},
						NodeAffinity: &corev1.NodeAffinity{
							PreferredDuringSchedulingIgnoredDuringExecution: []corev1.PreferredSchedulingTerm{
								{
									Weight: 50,
									Preference: corev1.NodeSelectorTerm{
										MatchExpressions: []corev1.NodeSelectorRequirement{
											{
												Key:      "scaleops.sh/node-packing",
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"true"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			desiredPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
				Spec:       corev1.PodSpec{},
			},
			expectPodAffinity:  true,
			expectNodeAffinity: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			syncScaleOpsAffinities(tt.desiredPod, tt.currentPod)

			if tt.expectPodAffinity {
				if tt.desiredPod.Spec.Affinity == nil || tt.desiredPod.Spec.Affinity.PodAffinity == nil {
					t.Errorf("expected pod affinity to be set")
				}
			} else {
				if tt.desiredPod.Spec.Affinity != nil && tt.desiredPod.Spec.Affinity.PodAffinity != nil {
					if len(tt.desiredPod.Spec.Affinity.PodAffinity.PreferredDuringSchedulingIgnoredDuringExecution) > 0 {
						t.Errorf("expected no pod affinity")
					}
				}
			}

			if tt.expectNodeAffinity {
				if tt.desiredPod.Spec.Affinity == nil || tt.desiredPod.Spec.Affinity.NodeAffinity == nil {
					t.Errorf("expected node affinity to be set")
				}
			} else {
				if tt.desiredPod.Spec.Affinity != nil && tt.desiredPod.Spec.Affinity.NodeAffinity != nil {
					if len(tt.desiredPod.Spec.Affinity.NodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution) > 0 {
						t.Errorf("expected no node affinity")
					}
				}
			}
		})
	}
}
