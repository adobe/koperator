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

package webhooks

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/banzaicloud/koperator/api/v1beta1"
)

func TestMetadataStorageAdmission(t *testing.T) {
	cluster := &v1beta1.KafkaCluster{Spec: v1beta1.KafkaClusterSpec{
		KRaftMode: true,
		BrokerConfigGroups: map[string]v1beta1.BrokerConfig{"brokers": {
			Roles:           []string{"broker"},
			StorageConfigs:  []v1beta1.StorageConfig{{MountPath: "/csi-kafka-logs1", PvcSpec: &corev1.PersistentVolumeClaimSpec{}}},
			MetadataStorage: &v1beta1.StorageConfig{MountPath: "/csi-kafka-metadata", PvcSpec: &corev1.PersistentVolumeClaimSpec{}},
		}},
		Brokers: []v1beta1.Broker{{Id: 1, BrokerConfigGroup: "brokers"}},
	}}
	validator := KafkaClusterValidator{Log: logr.Discard()}
	_, err := validator.ValidateCreate(context.Background(), cluster)
	require.NoError(t, err)
	for _, tc := range []struct {
		name   string
		change func(*v1beta1.KafkaCluster)
	}{
		{"removal", func(c *v1beta1.KafkaCluster) {
			b := c.Spec.BrokerConfigGroups["brokers"]
			b.MetadataStorage = nil
			c.Spec.BrokerConfigGroups["brokers"] = b
		}},
		{"relocation", func(c *v1beta1.KafkaCluster) {
			b := c.Spec.BrokerConfigGroups["brokers"]
			b.MetadataStorage.MountPath = "/new"
			c.Spec.BrokerConfigGroups["brokers"] = b
		}},
		{"role change", func(c *v1beta1.KafkaCluster) {
			b := c.Spec.BrokerConfigGroups["brokers"]
			b.Roles = []string{"controller"}
			c.Spec.BrokerConfigGroups["brokers"] = b
		}},
		{"zk mode", func(c *v1beta1.KafkaCluster) { c.Spec.KRaftMode = false }},
		{"zk migration", func(c *v1beta1.KafkaCluster) { c.Spec.Brokers[0].ReadOnlyConfig = "migration.broker.kRaftMode=false" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			next := cluster.DeepCopy()
			tc.change(next)
			_, err := validator.ValidateUpdate(context.Background(), cluster, next)
			require.Error(t, err)
		})
	}
	grown := cluster.DeepCopy()
	b := grown.Spec.BrokerConfigGroups["brokers"]
	b.MetadataStorage.PvcSpec.Resources.Requests = corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("20Gi")}
	grown.Spec.BrokerConfigGroups["brokers"] = b
	_, err = validator.ValidateUpdate(context.Background(), cluster, grown)
	require.NoError(t, err)
	old := cluster.DeepCopy()
	b = old.Spec.BrokerConfigGroups["brokers"]
	b.MetadataStorage = nil
	old.Spec.BrokerConfigGroups["brokers"] = b
	_, err = validator.ValidateUpdate(context.Background(), old, cluster)
	require.NoError(t, err)
	changedData := cluster.DeepCopy()
	b = changedData.Spec.BrokerConfigGroups["brokers"]
	b.StorageConfigs[0].MountPath = "/another-data-volume"
	changedData.Spec.BrokerConfigGroups["brokers"] = b
	_, err = validator.ValidateUpdate(context.Background(), old, changedData)
	require.Error(t, err)
}

func TestMetadataStorageDataDiskRemovalAdmission(t *testing.T) {
	cluster := &v1beta1.KafkaCluster{Spec: v1beta1.KafkaClusterSpec{
		KRaftMode: true,
		Brokers: []v1beta1.Broker{{Id: 1, BrokerConfig: &v1beta1.BrokerConfig{
			Roles: []string{"broker"},
			StorageConfigs: []v1beta1.StorageConfig{
				{MountPath: "/csi-kafka-logs1", PvcSpec: &corev1.PersistentVolumeClaimSpec{}},
				{MountPath: "/csi-kafka-logs2", PvcSpec: &corev1.PersistentVolumeClaimSpec{}},
			},
			MetadataStorage: &v1beta1.StorageConfig{MountPath: "/csi-kafka-metadata", PvcSpec: &corev1.PersistentVolumeClaimSpec{}},
		}}},
	}}
	validator := KafkaClusterValidator{Log: logr.Discard()}
	removed := cluster.DeepCopy()
	removed.Spec.Brokers[0].BrokerConfig.StorageConfigs = removed.Spec.Brokers[0].BrokerConfig.StorageConfigs[1:]
	added := cluster.DeepCopy()
	added.Spec.Brokers[0].BrokerConfig.StorageConfigs = append(added.Spec.Brokers[0].BrokerConfig.StorageConfigs,
		v1beta1.StorageConfig{MountPath: "/csi-kafka-logs3", PvcSpec: &corev1.PersistentVolumeClaimSpec{}})

	_, err := validator.ValidateUpdate(context.Background(), cluster, removed)
	require.ErrorContains(t, err, "metadataStorageState")
	_, err = validator.ValidateUpdate(context.Background(), cluster, added)
	require.NoError(t, err)

	ready := cluster.DeepCopy()
	ready.Status.BrokersState = map[string]v1beta1.BrokerState{"1": {MetadataStorageState: v1beta1.MetadataStorageReady}}
	_, err = validator.ValidateUpdate(context.Background(), ready, removed)
	require.NoError(t, err)

	otherReady := cluster.DeepCopy()
	otherReady.Status.BrokersState = map[string]v1beta1.BrokerState{"2": {MetadataStorageState: v1beta1.MetadataStorageReady}}
	_, err = validator.ValidateUpdate(context.Background(), otherReady, removed)
	require.Error(t, err)

	_, err = validator.ValidateCreate(context.Background(), &v1beta1.KafkaCluster{Spec: v1beta1.KafkaClusterSpec{
		KRaftMode: true,
		Brokers: []v1beta1.Broker{{Id: 1, BrokerConfig: &v1beta1.BrokerConfig{
			Roles:           []string{"broker"},
			StorageConfigs:  []v1beta1.StorageConfig{{MountPath: "/kafka-logs", EmptyDir: &corev1.EmptyDirVolumeSource{}}},
			MetadataStorage: &v1beta1.StorageConfig{MountPath: "/csi-kafka-metadata", PvcSpec: &corev1.PersistentVolumeClaimSpec{}},
		}}},
	}})
	require.ErrorContains(t, err, "PVC-backed")
}
