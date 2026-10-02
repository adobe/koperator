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

package v1beta1

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestMetadataStorageValidationAndMapping(t *testing.T) {
	base := &BrokerConfig{
		Roles:           []string{"broker"},
		StorageConfigs:  []StorageConfig{{MountPath: "/csi-kafka-logs1", PvcSpec: &corev1.PersistentVolumeClaimSpec{}}},
		MetadataStorage: &StorageConfig{MountPath: "/csi-kafka-metadata", PvcSpec: &corev1.PersistentVolumeClaimSpec{}},
	}
	for _, tc := range []struct {
		name   string
		kraft  bool
		change func(*BrokerConfig)
		valid  bool
	}{
		{"broker", true, func(*BrokerConfig) {}, true},
		{"default unchanged", false, func(b *BrokerConfig) { b.MetadataStorage = nil }, true},
		{"zk", false, func(*BrokerConfig) {}, false},
		{"controller", true, func(b *BrokerConfig) { b.Roles = []string{"controller"} }, false},
		{"combined", true, func(b *BrokerConfig) { b.Roles = []string{"broker", "controller"} }, false},
		{"emptydir", true, func(b *BrokerConfig) { b.MetadataStorage.EmptyDir = &corev1.EmptyDirVolumeSource{} }, false},
		{"missing pvc", true, func(b *BrokerConfig) { b.MetadataStorage.PvcSpec = nil }, false},
		{"emptydir data", true, func(b *BrokerConfig) {
			b.StorageConfigs = append(b.StorageConfigs, StorageConfig{MountPath: "/kafka-logs", EmptyDir: &corev1.EmptyDirVolumeSource{}})
		}, false},
		{"emptydir data without metadata storage", true, func(b *BrokerConfig) {
			b.MetadataStorage = nil
			b.StorageConfigs = []StorageConfig{{MountPath: "/kafka-logs", EmptyDir: &corev1.EmptyDirVolumeSource{}}}
		}, true},
		{"block pvc", true, func(b *BrokerConfig) {
			mode := corev1.PersistentVolumeBlock
			b.MetadataStorage.PvcSpec.VolumeMode = &mode
		}, false},
		{"reserved volume", true, func(b *BrokerConfig) { b.Volumes = []corev1.Volume{{Name: "kraft-metadata"}} }, false},
		{"collision", true, func(b *BrokerConfig) { b.MetadataStorage.MountPath = "/csi-kafka-logs1" }, false},
		{"nested collision", true, func(b *BrokerConfig) { b.MetadataStorage.MountPath = "/csi-kafka-logs1/metadata" }, false},
		{"reserved mount", true, func(b *BrokerConfig) { b.MetadataStorage.MountPath = "/config/metadata" }, false},
		{"custom mount", true, func(b *BrokerConfig) { b.VolumeMounts = []corev1.VolumeMount{{MountPath: "/csi-kafka-metadata/kafka"}} }, false},
		{"relative path", true, func(b *BrokerConfig) { b.MetadataStorage.MountPath = "metadata" }, false},
		{"unclean path", true, func(b *BrokerConfig) { b.MetadataStorage.MountPath = "/metadata/../logs" }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := base.DeepCopy()
			tc.change(b)
			if err := b.ValidateMetadataStorage(tc.kraft); (err == nil) != tc.valid {
				t.Fatalf("valid=%v, got %v", tc.valid, err)
			}
		})
	}
	group := base.DeepCopy()
	group.MetadataStorage.PvcSpec.StorageClassName = ptrString("group-class")
	broker := Broker{BrokerConfigGroup: "brokers"}
	spec := KafkaClusterSpec{BrokerConfigGroups: map[string]BrokerConfig{"brokers": *group}}
	inherited, err := broker.GetBrokerConfig(spec)
	if err != nil || inherited.MetadataStorage.MountPath != base.MetadataStorage.MountPath || len(inherited.StorageConfigs) != 1 {
		t.Fatalf("inheritance failed: %v, %v", inherited, err)
	}
	broker.BrokerConfig = &BrokerConfig{MetadataStorage: &StorageConfig{MountPath: "/local-metadata", PvcSpec: &corev1.PersistentVolumeClaimSpec{}}}
	local, err := broker.GetBrokerConfig(spec)
	if err != nil || local.MetadataStorage.MountPath != "/local-metadata" || local.MetadataStorage.PvcSpec.StorageClassName != nil {
		t.Fatalf("metadata override was merged rather than replaced: %v, %v", local, err)
	}
	local.MetadataStorage.MountPath = "/mutated"
	if broker.BrokerConfig.MetadataStorage.MountPath != "/local-metadata" || group.MetadataStorage.MountPath != base.MetadataStorage.MountPath {
		t.Fatal("merged config aliases its inputs")
	}
	a, b := base.MetadataStorage.DeepCopy(), base.MetadataStorage.DeepCopy()
	b.PvcSpec.Resources.Requests = corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("20Gi")}
	if !MetadataStorageLocationEqual(a, b) {
		t.Fatal("capacity change should not be a relocation")
	}
	b.MountPath = "/elsewhere"
	if MetadataStorageLocationEqual(a, b) || MetadataStorageLocationEqual(a, nil) {
		t.Fatal("removal/relocation allowed")
	}
}

func ptrString(value string) *string { return &value }
