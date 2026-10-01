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
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/banzaicloud/koperator/api/v1beta1"
)

func TestMetadataStorageExcludedFromCapacity(t *testing.T) {
	pvc := &corev1.PersistentVolumeClaimSpec{Resources: corev1.VolumeResourceRequirements{
		Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("100Gi")},
	}}
	config := v1beta1.BrokerConfig{
		Roles:           []string{"broker"},
		StorageConfigs:  []v1beta1.StorageConfig{{MountPath: "/csi-kafka-logs1", PvcSpec: pvc}},
		MetadataStorage: &v1beta1.StorageConfig{MountPath: "/csi-kafka-metadata", PvcSpec: pvc.DeepCopy()},
	}
	spec := v1beta1.KafkaClusterSpec{BrokerConfigGroups: map[string]v1beta1.BrokerConfig{"brokers": config}}
	broker := v1beta1.Broker{Id: 1, BrokerConfigGroup: "brokers"}
	disks, err := generateBrokerDisks(broker, spec, nil, nil, logr.Discard())
	require.NoError(t, err)
	require.Len(t, disks, 1)
	require.Contains(t, disks, "/csi-kafka-logs1/kafka")
	require.NotContains(t, disks, "/csi-kafka-metadata/kafka")
}
