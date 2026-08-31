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
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"

	"github.com/banzaicloud/koperator/api/v1beta1"
	"github.com/banzaicloud/koperator/pkg/resources/kafka/mocks"
)

// TestGetCreatedPvcForBroker asserts that a broker pod's PVC set never includes a disk that was
// removed from the broker's storage config and whose Cruise Control removal is already confirmed
// (or whose state has been cleared). Including such a soon-to-be-deleted PVC would bake a dangling
// reference into a (re)created broker pod, wedging it in Pending ("persistentvolumeclaim ... not
// found") forever. A removed disk whose removal is still in progress must stay mounted so Cruise
// Control can drain it.
func TestGetCreatedPvcForBroker(t *testing.T) {
	t.Parallel()
	withPvc := &corev1.PersistentVolumeClaimSpec{}

	tests := []struct {
		name             string
		storageConfigs   []v1beta1.StorageConfig
		existingPvcs     []*corev1.PersistentVolumeClaim
		volumeStates     map[string]v1beta1.VolumeState
		expectedPvcNames []string
	}{
		{
			name:           "removed disk whose removal succeeded is excluded",
			storageConfigs: []v1beta1.StorageConfig{{MountPath: "/kafka-logs1", PvcSpec: withPvc}},
			existingPvcs: []*corev1.PersistentVolumeClaim{
				createPvc("kafka-0-storage-0", "0", "/kafka-logs1"),
				createPvc("kafka-0-storage-2", "0", "/kafka-logs3"),
			},
			volumeStates: map[string]v1beta1.VolumeState{
				"/kafka-logs3": {CruiseControlVolumeState: v1beta1.GracefulDiskRemovalSucceeded},
			},
			expectedPvcNames: []string{"kafka-0-storage-0"},
		},
		{
			name:           "removed disk still draining is kept mounted",
			storageConfigs: []v1beta1.StorageConfig{{MountPath: "/kafka-logs1", PvcSpec: withPvc}},
			existingPvcs: []*corev1.PersistentVolumeClaim{
				createPvc("kafka-0-storage-0", "0", "/kafka-logs1"),
				createPvc("kafka-0-storage-2", "0", "/kafka-logs3"),
			},
			volumeStates: map[string]v1beta1.VolumeState{
				"/kafka-logs3": {CruiseControlVolumeState: v1beta1.GracefulDiskRemovalRunning},
			},
			expectedPvcNames: []string{"kafka-0-storage-0", "kafka-0-storage-2"},
		},
		{
			name:           "removed disk with cleared state is excluded (leaked pvc not mounted)",
			storageConfigs: []v1beta1.StorageConfig{{MountPath: "/kafka-logs1", PvcSpec: withPvc}},
			existingPvcs: []*corev1.PersistentVolumeClaim{
				createPvc("kafka-0-storage-0", "0", "/kafka-logs1"),
				createPvc("kafka-0-storage-2", "0", "/kafka-logs3"),
			},
			volumeStates:     map[string]v1beta1.VolumeState{},
			expectedPvcNames: []string{"kafka-0-storage-0"},
		},
		{
			name: "normal broker returns all pvcs",
			storageConfigs: []v1beta1.StorageConfig{
				{MountPath: "/kafka-logs1", PvcSpec: withPvc},
				{MountPath: "/kafka-logs3", PvcSpec: withPvc},
			},
			existingPvcs: []*corev1.PersistentVolumeClaim{
				createPvc("kafka-0-storage-0", "0", "/kafka-logs1"),
				createPvc("kafka-0-storage-2", "0", "/kafka-logs3"),
			},
			volumeStates:     map[string]v1beta1.VolumeState{},
			expectedPvcNames: []string{"kafka-0-storage-0", "kafka-0-storage-2"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			mockClient := mocks.NewMockClient(mockCtrl)
			mockClient.EXPECT().List(
				context.TODO(),
				gomock.AssignableToTypeOf(&corev1.PersistentVolumeClaimList{}),
				client.InNamespace("kafka"),
				gomock.Any(),
			).Do(func(ctx context.Context, list *corev1.PersistentVolumeClaimList, opts ...client.ListOption) {
				items := make([]corev1.PersistentVolumeClaim, len(tt.existingPvcs))
				for i, p := range tt.existingPvcs {
					items[i] = *p
				}
				list.Items = items
			}).Return(nil).AnyTimes()

			pvcs, err := getCreatedPvcForBroker(context.TODO(), mockClient, tt.volumeStates, int32(0), tt.storageConfigs, "kafka", "kafka")
			assert.NoError(t, err)

			var names []string
			for _, p := range pvcs {
				names = append(names, p.Name)
			}
			assert.ElementsMatch(t, tt.expectedPvcNames, names)
		})
	}
}

// TestPodUnschedulableReferencingRemovedPVC covers the self-heal predicate: a broker pod that was
// never scheduled (still Pending, no node) because it references a PVC the desired pod no longer
// mounts must be recreatable, bypassing the rolling-upgrade health gates. A running pod, or a pod
// with no stale claim, or one already assigned to a node must NOT be force-recreated.
func TestPodUnschedulableReferencingRemovedPVC(t *testing.T) {
	t.Parallel()
	podWithClaims := func(phase corev1.PodPhase, nodeName string, claims ...string) *corev1.Pod {
		vols := make([]corev1.Volume, 0, len(claims))
		for _, c := range claims {
			vols = append(vols, corev1.Volume{
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: c},
				},
			})
		}
		return &corev1.Pod{
			Spec:   corev1.PodSpec{NodeName: nodeName, Volumes: vols},
			Status: corev1.PodStatus{Phase: phase},
		}
	}

	tests := []struct {
		name    string
		current *corev1.Pod
		desired *corev1.Pod
		want    bool
	}{
		{
			name:    "pending unscheduled pod referencing a removed pvc",
			current: podWithClaims(corev1.PodPending, "", "kafka-0-storage-0", "kafka-0-storage-2"),
			desired: podWithClaims(corev1.PodPending, "", "kafka-0-storage-0"),
			want:    true,
		},
		{
			name:    "running pod with stale claim is not force-recreated",
			current: podWithClaims(corev1.PodRunning, "node1", "kafka-0-storage-0", "kafka-0-storage-2"),
			desired: podWithClaims(corev1.PodPending, "", "kafka-0-storage-0"),
			want:    false,
		},
		{
			name:    "pending pod with no stale claim",
			current: podWithClaims(corev1.PodPending, "", "kafka-0-storage-0"),
			desired: podWithClaims(corev1.PodPending, "", "kafka-0-storage-0"),
			want:    false,
		},
		{
			name:    "scheduled pending pod (initializing) is not force-recreated",
			current: podWithClaims(corev1.PodPending, "node1", "kafka-0-storage-0", "kafka-0-storage-2"),
			desired: podWithClaims(corev1.PodPending, "", "kafka-0-storage-0"),
			want:    false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, podUnschedulableReferencingRemovedPVC(tt.current, tt.desired))
		})
	}
}
