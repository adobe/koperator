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
