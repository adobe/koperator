// Copyright © 2019 Cisco Systems, Inc. and/or its affiliates
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
	"testing"

	"github.com/banzaicloud/k8s-objectmatcher/patch"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestIgnoreMutationWebhookFields(t *testing.T) {
	tests := []struct {
		name        string
		currentPod  *corev1.Pod
		modifiedPod *corev1.Pod
		expectDiff  bool
		description string
	}{
		{
			name: "ignore gatekeeper mutation annotations",
			currentPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
					Annotations: map[string]string{
						"gatekeeper.sh/mutation-id": "abc123",
						"gatekeeper.sh/mutations":   "Assign//policy1:1",
						"other-annotation":          "keep-this",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "kafka",
							Image: "kafka:latest",
						},
					},
				},
			},
			modifiedPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
					Annotations: map[string]string{
						"other-annotation": "keep-this",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "kafka",
							Image: "kafka:latest",
						},
					},
				},
			},
			expectDiff:  false,
			description: "Gatekeeper mutation annotations should be ignored",
		},
		{
			name: "detect actual spec changes",
			currentPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "kafka",
							Image: "kafka:1.0",
						},
					},
				},
			},
			modifiedPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "kafka",
							Image: "kafka:2.0",
						},
					},
				},
			},
			expectDiff:  true,
			description: "Real spec changes should be detected",
		},
		{
			name: "only gatekeeper annotations differ",
			currentPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
					Annotations: map[string]string{
						"gatekeeper.sh/mutation-id": "xyz789",
						"app":                       "kafka",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "kafka",
							Image: "kafka:latest",
						},
					},
				},
			},
			modifiedPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
					Annotations: map[string]string{
						"app": "kafka",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "kafka",
							Image: "kafka:latest",
						},
					},
				},
			},
			expectDiff:  false,
			description: "Only gatekeeper annotations differ, should not trigger diff",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set last applied annotation on current pod
			if err := patch.DefaultAnnotator.SetLastAppliedAnnotation(tt.currentPod); err != nil {
				t.Fatalf("Failed to set last applied annotation: %v", err)
			}

			opts := []patch.CalculateOption{
				IgnoreMutationWebhookFields(),
			}

			patchResult, err := patch.DefaultPatchMaker.Calculate(tt.currentPod, tt.modifiedPod, opts...)
			if err != nil {
				t.Fatalf("Failed to calculate patch: %v", err)
			}

			hasDiff := !patchResult.IsEmpty()
			if hasDiff != tt.expectDiff {
				t.Errorf("%s: expected diff=%v, got diff=%v\nPatch: %s",
					tt.description, tt.expectDiff, hasDiff, string(patchResult.Patch))
			}
		})
	}
}

func TestCombinedIgnoreOptions(t *testing.T) {
	t.Run("combine mutation webhook and resource ignoring", func(t *testing.T) {
		currentPod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
				Annotations: map[string]string{
					"gatekeeper.sh/mutation-id":      "abc123",
					"gatekeeper.sh/mutations":        "Assign//policy1:1",
					"scaleops.sh/pod-owner-grouping": "kafkacluster",
				},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "kafka",
						Image: "kafka:latest",
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("2000m"),
								corev1.ResourceMemory: resource.MustParse("4Gi"),
							},
						},
					},
				},
			},
		}

		modifiedPod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
				Annotations: map[string]string{
					"scaleops.sh/pod-owner-grouping": "kafkacluster",
				},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "kafka",
						Image: "kafka:latest",
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("1000m"),
								corev1.ResourceMemory: resource.MustParse("2Gi"),
							},
						},
					},
				},
			},
		}

		if err := patch.DefaultAnnotator.SetLastAppliedAnnotation(currentPod); err != nil {
			t.Fatalf("Failed to set last applied annotation: %v", err)
		}

		opts := []patch.CalculateOption{
			IgnoreMutationWebhookFields(),
		}

		patchResult, err := patch.DefaultPatchMaker.Calculate(currentPod, modifiedPod, opts...)
		if err != nil {
			t.Fatalf("Failed to calculate patch: %v", err)
		}

		if !patchResult.IsEmpty() {
			t.Errorf("Expected no diff when both mutation webhook annotations and resources differ (ScaleOps managed), but got patch: %s",
				string(patchResult.Patch))
		}
	})
}

func TestCleanMutationWebhookFields(t *testing.T) {
	t.Run("removes gatekeeper annotations", func(t *testing.T) {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
				Annotations: map[string]string{
					"gatekeeper.sh/mutation-id": "abc123",
					"gatekeeper.sh/mutations":   "Assign//policy1:1",
					"keep-this":                 "value",
				},
			},
		}

		cleaned := cleanMutationWebhookFields(pod, false)

		if _, exists := cleaned.Annotations["gatekeeper.sh/mutation-id"]; exists {
			t.Error("gatekeeper.sh/mutation-id should be removed")
		}
		if _, exists := cleaned.Annotations["gatekeeper.sh/mutations"]; exists {
			t.Error("gatekeeper.sh/mutations should be removed")
		}
		if cleaned.Annotations["keep-this"] != "value" {
			t.Error("Other annotations should be preserved")
		}
	})

	t.Run("does not modify original pod", func(t *testing.T) {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
				Annotations: map[string]string{
					"gatekeeper.sh/mutation-id": "abc123",
				},
			},
		}

		cleanMutationWebhookFields(pod, false)

		if _, exists := pod.Annotations["gatekeeper.sh/mutation-id"]; !exists {
			t.Error("Original pod should not be modified")
		}
	})
}

// Helper functions for creating test pods
func createBasicPod() *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-pod",
			Namespace:   "default",
			Annotations: map[string]string{},
			Labels:      map[string]string{},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "kafka",
					Image: "kafka:latest",
				},
			},
		},
	}
}

func createScaleOpsAffinity() *corev1.Affinity {
	return &corev1.Affinity{
		NodeAffinity: &corev1.NodeAffinity{
			PreferredDuringSchedulingIgnoredDuringExecution: []corev1.PreferredSchedulingTerm{
				{
					Weight: 95,
					Preference: corev1.NodeSelectorTerm{
						MatchExpressions: []corev1.NodeSelectorRequirement{
							{
								Key:      "scaleops.sh/node-packing",
								Operator: corev1.NodeSelectorOpIn,
								Values:   []string{"high"},
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
								Values:   []string{"medium"},
							},
						},
					},
				},
			},
		},
		PodAffinity: &corev1.PodAffinity{
			PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
				{
					Weight: 100,
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
	}
}

func createKafkaContainer(image, cpuRequest string) corev1.Container {
	container := corev1.Container{
		Name:  "kafka",
		Image: image,
	}
	if cpuRequest != "" {
		container.Resources = corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(cpuRequest),
				corev1.ResourceMemory: resource.MustParse("4Gi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("4"),
				corev1.ResourceMemory: resource.MustParse("4Gi"),
			},
		}
	}
	return container
}

func createComplexScaleOpsPodBefore() *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pipeline-kafka-101-4s7b2",
			Namespace: "default",
			Annotations: map[string]string{
				"scaleops.sh/admission":          "true",
				"scaleops.sh/applied-policy":     "high-availability",
				"scaleops.sh/managed-containers": "{}",
				"scaleops.sh/pod-owner-grouping": "kafkabroker",
				"app":                            "kafka",
			},
			Labels: map[string]string{
				"scaleops.sh/managed":             "true",
				"scaleops.sh/managed-unevictable": "true",
				"scaleops.sh/pod-owner-grouping":  "kafkabroker",
				"app":                             "kafka",
				"brokerId":                        "101",
			},
		},
		Spec: corev1.PodSpec{
			Affinity: &corev1.Affinity{
				NodeAffinity: &corev1.NodeAffinity{
					PreferredDuringSchedulingIgnoredDuringExecution: []corev1.PreferredSchedulingTerm{
						{
							Weight: 95,
							Preference: corev1.NodeSelectorTerm{
								MatchExpressions: []corev1.NodeSelectorRequirement{
									{
										Key:      "scaleops.sh/node-packing",
										Operator: corev1.NodeSelectorOpIn,
										Values:   []string{"high"},
									},
								},
							},
						},
					},
				},
				PodAffinity: &corev1.PodAffinity{
					PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
						{
							Weight: 100,
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
			Containers: []corev1.Container{
				createKafkaContainer("kafka:3.9.1", "697m"),
				{
					Name:  "fluent-bit",
					Image: "fluent-bit:latest",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("100Mi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("256Mi"),
						},
					},
				},
			},
		},
	}
}

func createComplexScaleOpsPodAfter() *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pipeline-kafka-101-4s7b2",
			Namespace: "default",
			Annotations: map[string]string{
				"app": "kafka",
			},
			Labels: map[string]string{
				"app":      "kafka",
				"brokerId": "101",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				createKafkaContainer("kafka:3.9.1", "1"),
				{
					Name:  "fluent-bit",
					Image: "fluent-bit:latest",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("256Mi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("256Mi"),
						},
					},
				},
			},
		},
	}
}

func getScaleOpsTestCases() []struct {
	name        string
	currentPod  *corev1.Pod
	modifiedPod *corev1.Pod
	expectDiff  bool
	description string
} {
	return []struct {
		name        string
		currentPod  *corev1.Pod
		modifiedPod *corev1.Pod
		expectDiff  bool
		description string
	}{
		{
			name: "ignore scaleops annotations and labels",
			currentPod: func() *corev1.Pod {
				pod := createBasicPod()
				pod.Annotations = map[string]string{
					"scaleops.sh/admission":                 "true",
					"scaleops.sh/applied-policy":            "high-availability",
					"scaleops.sh/last-applied-resources":    "{}",
					"scaleops.sh/managed-containers":        "{}",
					"scaleops.sh/managed-keep-limit-cpu":    "true",
					"scaleops.sh/managed-keep-limit-memory": "true",
					"scaleops.sh/origin-resources":          "{}",
					"scaleops.sh/pod-owner-grouping":        "kafkabroker",
					"scaleops.sh/pod-owner-identifier":      "pipeline-kafka-123",
					"app":                                   "kafka",
				}
				pod.Labels = map[string]string{
					"scaleops.sh/applied-recommendation": "kafkabroker-pipeline-kafka-123",
					"scaleops.sh/managed":                "true",
					"scaleops.sh/managed-unevictable":    "true",
					"scaleops.sh/pod-owner-grouping":     "kafkabroker",
					"scaleops.sh/pod-owner-identifier":   "pipeline-kafka-123",
					"app":                                "kafka",
				}
				return pod
			}(),
			modifiedPod: func() *corev1.Pod {
				pod := createBasicPod()
				pod.Annotations["app"] = "kafka"
				pod.Labels["app"] = "kafka"
				return pod
			}(),
			expectDiff:  false,
			description: "ScaleOps annotations and labels should be ignored",
		},
		{
			name: "ignore scaleops-modified resources",
			currentPod: func() *corev1.Pod {
				pod := createBasicPod()
				pod.Annotations = map[string]string{
					"scaleops.sh/managed-containers": "{}",
					"scaleops.sh/pod-owner-grouping": "kafkabroker",
				}
				pod.Spec.Containers[0] = createKafkaContainer("kafka:latest", "697m")
				return pod
			}(),
			modifiedPod: func() *corev1.Pod {
				pod := createBasicPod()
				pod.Annotations = map[string]string{
					"scaleops.sh/managed-containers": "{}",
					"scaleops.sh/pod-owner-grouping": "kafkabroker",
				}
				pod.Spec.Containers[0] = createKafkaContainer("kafka:latest", "1")
				return pod
			}(),
			expectDiff:  false,
			description: "ScaleOps-modified resources should be ignored when annotations present",
		},
		{
			name: "ignore scaleops-added affinity rules",
			currentPod: func() *corev1.Pod {
				pod := createBasicPod()
				pod.Spec.Affinity = createScaleOpsAffinity()
				return pod
			}(),
			modifiedPod: createBasicPod(),
			expectDiff:  false,
			description: "ScaleOps-added affinity rules should be ignored",
		},
		{
			name: "detect image changes even with scaleops",
			currentPod: func() *corev1.Pod {
				pod := createBasicPod()
				pod.Annotations["scaleops.sh/pod-owner-grouping"] = "kafkabroker"
				pod.Labels["scaleops.sh/managed"] = "true"
				pod.Spec.Containers[0].Image = "kafka:3.6.1"
				return pod
			}(),
			modifiedPod: func() *corev1.Pod {
				pod := createBasicPod()
				pod.Annotations["scaleops.sh/pod-owner-grouping"] = "kafkabroker"
				pod.Labels["scaleops.sh/managed"] = "true"
				pod.Spec.Containers[0].Image = "kafka:3.9.1"
				return pod
			}(),
			expectDiff:  true,
			description: "Image changes should be detected even with ScaleOps annotations",
		},
		{
			name:        "complex scaleops scenario - all mutations ignored",
			currentPod:  createComplexScaleOpsPodBefore(),
			modifiedPod: createComplexScaleOpsPodAfter(),
			expectDiff:  false,
			description: "Complex ScaleOps scenario: all ScaleOps mutations should be ignored",
		},
	}
}

func TestIgnoreScaleOpsFields(t *testing.T) {
	tests := getScaleOpsTestCases()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set last applied annotation on current pod
			if err := patch.DefaultAnnotator.SetLastAppliedAnnotation(tt.currentPod); err != nil {
				t.Fatalf("Failed to set last applied annotation: %v", err)
			}

			opts := []patch.CalculateOption{
				IgnoreMutationWebhookFields(),
			}

			patchResult, err := patch.DefaultPatchMaker.Calculate(tt.currentPod, tt.modifiedPod, opts...)
			if err != nil {
				t.Fatalf("Failed to calculate patch: %v", err)
			}

			hasDiff := !patchResult.IsEmpty()
			if hasDiff != tt.expectDiff {
				t.Errorf("%s: expected diff=%v, got diff=%v\nPatch: %s\nCurrent: %s\nModified: %s",
					tt.description, tt.expectDiff, hasDiff,
					string(patchResult.Patch),
					string(patchResult.Current),
					string(patchResult.Modified))
			}
		})
	}
}
