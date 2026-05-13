// Copyright © 2022 Cisco Systems, Inc. and/or its affiliates
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

package webhooks

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/banzaicloud/koperator/pkg/util"

	"github.com/banzaicloud/koperator/api/v1beta1"
)

func TestCheckUniqueListenerContainerPort(t *testing.T) {
	testCases := []struct {
		testName  string
		listeners v1beta1.ListenersConfig
		expected  field.ErrorList
	}{
		{
			testName: "unique values",
			listeners: v1beta1.ListenersConfig{
				InternalListeners: []v1beta1.InternalListenerConfig{
					{
						CommonListenerSpec: v1beta1.CommonListenerSpec{Name: "test-internal1", ContainerPort: 29092},
					},
					{
						CommonListenerSpec: v1beta1.CommonListenerSpec{Name: "test-internal2", ContainerPort: 29093},
					},
				},
				ExternalListeners: []v1beta1.ExternalListenerConfig{
					{
						CommonListenerSpec: v1beta1.CommonListenerSpec{Name: "test-external1", ContainerPort: 9094},
					},
					{
						CommonListenerSpec: v1beta1.CommonListenerSpec{Name: "test-external2", ContainerPort: 9095},
					},
				},
			},
			expected: nil,
		},
		{
			testName: "non-unique containerPorts with only internalListeners",
			listeners: v1beta1.ListenersConfig{
				InternalListeners: []v1beta1.InternalListenerConfig{
					{
						CommonListenerSpec: v1beta1.CommonListenerSpec{Name: "test-internal1", ContainerPort: 29092},
					},
					{
						CommonListenerSpec: v1beta1.CommonListenerSpec{Name: "test-internal2", ContainerPort: 29092},
					},
				},
			},
			expected: append(field.ErrorList{},
				field.Duplicate(field.NewPath("spec").Child("listenersConfig").Child("internalListeners").Index(1).Child("containerPort"), int32(29092))),
		},
		{
			testName: "non-unique containerPorts with only externalListeners",
			listeners: v1beta1.ListenersConfig{
				ExternalListeners: []v1beta1.ExternalListenerConfig{
					{
						CommonListenerSpec: v1beta1.CommonListenerSpec{Name: "test-external1", ContainerPort: 9094},
					},
					{
						CommonListenerSpec: v1beta1.CommonListenerSpec{Name: "test-external2", ContainerPort: 9094},
					},
				},
			},
			expected: append(field.ErrorList{},
				field.Duplicate(field.NewPath("spec").Child("listenersConfig").Child("externalListeners").Index(1).Child("containerPort"), int32(9094))),
		},
		{
			testName: "non-unique containerPorts across both listener types single error",
			listeners: v1beta1.ListenersConfig{
				InternalListeners: []v1beta1.InternalListenerConfig{
					{
						CommonListenerSpec: v1beta1.CommonListenerSpec{Name: "test-internal1", ContainerPort: 39098},
					},
				},
				ExternalListeners: []v1beta1.ExternalListenerConfig{
					{
						CommonListenerSpec: v1beta1.CommonListenerSpec{Name: "test-external1", ContainerPort: 39098},
					},
				},
			},
			expected: append(field.ErrorList{},
				field.Duplicate(field.NewPath("spec").Child("listenersConfig").Child("externalListeners").Index(0).Child("containerPort"), int32(39098))),
		},
		{
			testName: "non-unique containerPorts across both listener types two errors",
			listeners: v1beta1.ListenersConfig{
				InternalListeners: []v1beta1.InternalListenerConfig{
					{
						CommonListenerSpec: v1beta1.CommonListenerSpec{Name: "test-internal1", ContainerPort: 39098},
					},
					{
						CommonListenerSpec: v1beta1.CommonListenerSpec{Name: "test-internal2", ContainerPort: 39098},
					},
				},
				ExternalListeners: []v1beta1.ExternalListenerConfig{
					{
						CommonListenerSpec: v1beta1.CommonListenerSpec{Name: "test-external1", ContainerPort: 39098},
					},
				},
			},
			expected: append(field.ErrorList{},
				field.Duplicate(field.NewPath("spec").Child("listenersConfig").Child("internalListeners").Index(1).Child("containerPort"), int32(39098)),
				field.Duplicate(field.NewPath("spec").Child("listenersConfig").Child("externalListeners").Index(0).Child("containerPort"), int32(39098)),
			),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.testName, func(t *testing.T) {
			got := checkUniqueListenerContainerPort(testCase.listeners)
			require.Equal(t, testCase.expected, got)
		})
	}
}

func TestCheckExternalListenerStartingPort(t *testing.T) {
	testCases := []struct {
		testName         string
		kafkaClusterSpec v1beta1.KafkaClusterSpec
		expected         field.ErrorList
	}{
		{
			// In this test case, all resulting external port numbers should be valid
			testName: "valid config: 3 brokers with 2 externalListeners",
			kafkaClusterSpec: v1beta1.KafkaClusterSpec{
				Brokers: []v1beta1.Broker{{Id: 900}, {Id: 901}, {Id: 902}},
				ListenersConfig: v1beta1.ListenersConfig{
					ExternalListeners: []v1beta1.ExternalListenerConfig{
						{
							CommonListenerSpec:   v1beta1.CommonListenerSpec{Name: "test-external1"},
							ExternalStartingPort: 19090,
						},
						{
							CommonListenerSpec:   v1beta1.CommonListenerSpec{Name: "test-external2"},
							ExternalStartingPort: 29090,
						},
					},
				},
			},
			expected: nil,
		},
		{
			// In this test case, both externalListeners have an externalStartinPort that is already >65535
			// so both should generate field.Error's for all brokers/brokerIDs
			testName: "invalid config: 3 brokers with 2 out-of-range externalListeners",
			kafkaClusterSpec: v1beta1.KafkaClusterSpec{
				Brokers: []v1beta1.Broker{{Id: 900}, {Id: 901}, {Id: 902}},
				ListenersConfig: v1beta1.ListenersConfig{
					ExternalListeners: []v1beta1.ExternalListenerConfig{
						{
							CommonListenerSpec:   v1beta1.CommonListenerSpec{Name: "test-external1"},
							ExternalStartingPort: 79090,
						},
						{
							CommonListenerSpec:   v1beta1.CommonListenerSpec{Name: "test-external2"},
							ExternalStartingPort: 89090,
						},
					},
				},
			},
			expected: append(field.ErrorList{},
				field.Invalid(field.NewPath("spec").Child("listenersConfig").Child("externalListeners").Index(0).Child("externalStartingPort"), int32(79090),
					invalidExternalListenerStartingPortErrMsg+": "+fmt.Sprintf("ExternalListener '%s' would generate external access port numbers (externalStartingPort + Broker ID) that are out of range (not between 1 and 65535) for brokers %v",
						"test-external1", []int32{900, 901, 902})),
				field.Invalid(field.NewPath("spec").Child("listenersConfig").Child("externalListeners").Index(1).Child("externalStartingPort"), int32(89090),
					invalidExternalListenerStartingPortErrMsg+": "+fmt.Sprintf("ExternalListener '%s' would generate external access port numbers (externalStartingPort + Broker ID) that are out of range (not between 1 and 65535) for brokers %v",
						"test-external2", []int32{900, 901, 902})),
			),
		},
		{
			// In this test case:
			// - external1 should be invalid for brokers [11, 102] but not [0] (sum is not >65535)
			// - external2 should be invalid for brokers [102] but not [0, 11]
			testName: "invalid config: 3 brokers with 2 at-the-limit externalListeners",
			kafkaClusterSpec: v1beta1.KafkaClusterSpec{
				Brokers: []v1beta1.Broker{{Id: 0}, {Id: 11}, {Id: 102}},
				ListenersConfig: v1beta1.ListenersConfig{
					ExternalListeners: []v1beta1.ExternalListenerConfig{
						{
							CommonListenerSpec:   v1beta1.CommonListenerSpec{Name: "test-external1"},
							ExternalStartingPort: 65535,
						},
						{
							CommonListenerSpec:   v1beta1.CommonListenerSpec{Name: "test-external2"},
							ExternalStartingPort: 65434,
						},
					},
				},
			},
			expected: append(field.ErrorList{},
				field.Invalid(field.NewPath("spec").Child("listenersConfig").Child("externalListeners").Index(0).Child("externalStartingPort"), int32(65535),
					invalidExternalListenerStartingPortErrMsg+": "+fmt.Sprintf("ExternalListener '%s' would generate external access port numbers (externalStartingPort + Broker ID) that are out of range (not between 1 and 65535) for brokers %v",
						"test-external1", []int32{11, 102})),
				field.Invalid(field.NewPath("spec").Child("listenersConfig").Child("externalListeners").Index(1).Child("externalStartingPort"), int32(65434),
					invalidExternalListenerStartingPortErrMsg+": "+fmt.Sprintf("ExternalListener '%s' would generate external access port numbers (externalStartingPort + Broker ID) that are out of range (not between 1 and 65535) for brokers %v",
						"test-external2", []int32{102})),
			),
		},
		{
			testName: "invalid config: brokers with in-range external port numbers, but they collide with the envoy ports",
			kafkaClusterSpec: v1beta1.KafkaClusterSpec{
				Brokers: []v1beta1.Broker{{Id: 0}, {Id: 11}, {Id: 102}},
				ListenersConfig: v1beta1.ListenersConfig{
					ExternalListeners: []v1beta1.ExternalListenerConfig{
						{
							CommonListenerSpec:   v1beta1.CommonListenerSpec{Name: "test-external1"},
							ExternalStartingPort: 8080,
						},
						{
							CommonListenerSpec:   v1beta1.CommonListenerSpec{Name: "test-external2"},
							ExternalStartingPort: 8070,
						},
					},
				},
			},
			expected: append(field.ErrorList{},
				field.Invalid(field.NewPath("spec").Child("listenersConfig").Child("externalListeners").Index(0).Child("externalStartingPort"), int32(8080),
					invalidExternalListenerStartingPortErrMsg+": "+fmt.Sprintf("ExternalListener '%s' would generate external access port numbers ("+
						"externalStartingPort + Broker ID) that collide with either the envoy admin port ('%d'), the envoy health-check port ('%d'), "+
						"or the ingressControllerTargetPort ('%d') for brokers %v",
						"test-external1", int32(8081), int32(8080), int32(29092), []int32{0})),
				field.Invalid(field.NewPath("spec").Child("listenersConfig").Child("externalListeners").Index(1).Child("externalStartingPort"), int32(8070),
					invalidExternalListenerStartingPortErrMsg+": "+fmt.Sprintf("ExternalListener '%s' would generate external access port numbers ("+
						"externalStartingPort + Broker ID) that collide with either the envoy admin port ('%d'), the envoy health-check port ('%d'), "+
						"or the ingressControllerTargetPort ('%d') for brokers %v",
						"test-external2", int32(8081), int32(8080), int32(29092), []int32{11})),
			),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.testName, func(t *testing.T) {
			got := checkExternalListenerStartingPort(&testCase.kafkaClusterSpec)
			require.Equal(t, testCase.expected, got)
		})
	}
}

func TestCheckTieredStorageCacheImmutability(t *testing.T) {
	sc := func(path string, tc bool) v1beta1.StorageConfig {
		return v1beta1.StorageConfig{MountPath: path, TieredStorageCache: tc}
	}
	// committed returns a KafkaCluster whose status records the given tieredCache volumes for broker 0.
	// Only cache PVCs are tracked (TieredCacheVolumeActive or TieredCacheVolumePendingDeletion);
	// non-cache PVCs have no entry.
	committed := func(vols map[string]v1beta1.TieredCacheVolumeState) *v1beta1.KafkaCluster {
		return &v1beta1.KafkaCluster{
			Status: v1beta1.KafkaClusterStatus{
				BrokersState: map[string]v1beta1.BrokerState{
					"0": {TieredCacheVolumes: vols},
				},
			},
		}
	}

	testCases := []struct {
		testName   string
		oldCluster *v1beta1.KafkaCluster
		newCluster *v1beta1.KafkaCluster
		expected   field.ErrorList
	}{
		{
			// No committed state — new cluster, any spec is valid.
			testName:   "no committed state — any spec change allowed",
			oldCluster: &v1beta1.KafkaCluster{},
			newCluster: &v1beta1.KafkaCluster{Spec: v1beta1.KafkaClusterSpec{
				Brokers: []v1beta1.Broker{{Id: 0, BrokerConfig: &v1beta1.BrokerConfig{
					StorageConfigs: []v1beta1.StorageConfig{sc("/data", true)},
				}}},
			}},
			expected: nil,
		},
		{
			// Committed status says /data IS a cache volume; inline spec now marks it as non-cache.
			testName:   "in-place inline flip true→false rejected",
			oldCluster: committed(map[string]v1beta1.TieredCacheVolumeState{"/data": v1beta1.TieredCacheVolumeActive}),
			newCluster: &v1beta1.KafkaCluster{Spec: v1beta1.KafkaClusterSpec{
				Brokers: []v1beta1.Broker{{Id: 0, BrokerConfig: &v1beta1.BrokerConfig{
					StorageConfigs: []v1beta1.StorageConfig{sc("/data", false)},
				}}},
			}},
			expected: append(field.ErrorList{},
				field.Forbidden(
					field.NewPath("spec").Child("brokers").Index(0).Child("brokerConfig").Child("storageConfigs").Index(0).Child("tieredStorageCache"),
					immutableTieredStorageCacheErrMsg,
				),
			),
		},
		{
			// Committed status says /cache IS a cache volume; group spec now marks it as non-cache.
			testName:   "in-place group flip true→false rejected",
			oldCluster: committed(map[string]v1beta1.TieredCacheVolumeState{"/cache": v1beta1.TieredCacheVolumeActive}),
			newCluster: &v1beta1.KafkaCluster{Spec: v1beta1.KafkaClusterSpec{
				Brokers: []v1beta1.Broker{{Id: 0, BrokerConfigGroup: "default"}},
				BrokerConfigGroups: map[string]v1beta1.BrokerConfig{
					"default": {StorageConfigs: []v1beta1.StorageConfig{sc("/cache", false)}},
				},
			}},
			expected: append(field.ErrorList{},
				field.Forbidden(
					field.NewPath("spec").Child("brokerConfigGroups").Key("default").Child("storageConfigs").Index(0).Child("tieredStorageCache"),
					immutableTieredStorageCacheErrMsg,
				),
			),
		},
		{
			// Bypass attempt: broker switches from groupA (where /cache=true) to groupB (where /cache=false).
			// The raw-spec old→new comparison would miss this since groupA is unchanged.
			// Status-based check catches it because committed status records /cache=active for broker 0.
			testName:   "group-switch bypass rejected",
			oldCluster: committed(map[string]v1beta1.TieredCacheVolumeState{"/cache": v1beta1.TieredCacheVolumeActive}),
			newCluster: &v1beta1.KafkaCluster{Spec: v1beta1.KafkaClusterSpec{
				Brokers: []v1beta1.Broker{{Id: 0, BrokerConfigGroup: "groupB"}},
				BrokerConfigGroups: map[string]v1beta1.BrokerConfig{
					"groupA": {StorageConfigs: []v1beta1.StorageConfig{sc("/cache", true)}},
					"groupB": {StorageConfigs: []v1beta1.StorageConfig{sc("/cache", false)}},
				},
			}},
			expected: append(field.ErrorList{},
				field.Forbidden(
					field.NewPath("spec").Child("brokerConfigGroups").Key("groupB").Child("storageConfigs").Index(0).Child("tieredStorageCache"),
					immutableTieredStorageCacheErrMsg,
				),
			),
		},
		{
			// Bypass attempt: /cache was provisioned as a cache volume (via group); now an inline entry
			// overrides it with TieredStorageCache=false. Inline takes priority in GetBrokerConfig so
			// the effective value would flip — the check must reject the inline entry.
			testName:   "inline-shadow bypass rejected",
			oldCluster: committed(map[string]v1beta1.TieredCacheVolumeState{"/cache": v1beta1.TieredCacheVolumeActive}),
			newCluster: &v1beta1.KafkaCluster{Spec: v1beta1.KafkaClusterSpec{
				Brokers: []v1beta1.Broker{{
					Id:                0,
					BrokerConfigGroup: "default",
					BrokerConfig: &v1beta1.BrokerConfig{
						StorageConfigs: []v1beta1.StorageConfig{sc("/cache", false)},
					},
				}},
				BrokerConfigGroups: map[string]v1beta1.BrokerConfig{
					"default": {StorageConfigs: []v1beta1.StorageConfig{sc("/cache", true)}},
				},
			}},
			expected: append(field.ErrorList{},
				field.Forbidden(
					field.NewPath("spec").Child("brokers").Index(0).Child("brokerConfig").Child("storageConfigs").Index(0).Child("tieredStorageCache"),
					immutableTieredStorageCacheErrMsg,
				),
			),
		},
		{
			// mountPath is removed from spec entirely — remove-and-re-add path is intentionally allowed.
			testName:   "remove mountPath from spec — allowed",
			oldCluster: committed(map[string]v1beta1.TieredCacheVolumeState{"/cache": v1beta1.TieredCacheVolumeActive}),
			newCluster: &v1beta1.KafkaCluster{Spec: v1beta1.KafkaClusterSpec{
				Brokers: []v1beta1.Broker{{Id: 0, BrokerConfig: &v1beta1.BrokerConfig{
					StorageConfigs: []v1beta1.StorageConfig{sc("/data", false)},
				}}},
			}},
			expected: nil,
		},
		{
			// New mountPath with no committed state — any value is allowed.
			testName:   "new mountPath not in committed state — allowed",
			oldCluster: committed(map[string]v1beta1.TieredCacheVolumeState{"/data": v1beta1.TieredCacheVolumeActive}),
			newCluster: &v1beta1.KafkaCluster{Spec: v1beta1.KafkaClusterSpec{
				Brokers: []v1beta1.Broker{{Id: 0, BrokerConfig: &v1beta1.BrokerConfig{
					StorageConfigs: []v1beta1.StorageConfig{sc("/data", true), sc("/cache", true)},
				}}},
			}},
			expected: nil,
		},
		{
			// Value unchanged — no error.
			testName:   "unchanged value — no error",
			oldCluster: committed(map[string]v1beta1.TieredCacheVolumeState{"/cache": v1beta1.TieredCacheVolumeActive}),
			newCluster: &v1beta1.KafkaCluster{Spec: v1beta1.KafkaClusterSpec{
				Brokers: []v1beta1.Broker{{Id: 0, BrokerConfig: &v1beta1.BrokerConfig{
					StorageConfigs: []v1beta1.StorageConfig{sc("/cache", true)},
				}}},
			}},
			expected: nil,
		},
		{
			// Existing log-dir volume flipped to cache via inline config.
			// The old spec has tieredStorageCache=false, the new spec flips it to true.
			// The broker has no TieredCacheVolumes status entry (it is a log dir, not a cache),
			// so only the spec-based check catches this.
			testName: "false→true flip on existing log-dir volume rejected (inline)",
			oldCluster: &v1beta1.KafkaCluster{
				Spec: v1beta1.KafkaClusterSpec{
					Brokers: []v1beta1.Broker{{Id: 0, BrokerConfig: &v1beta1.BrokerConfig{
						StorageConfigs: []v1beta1.StorageConfig{sc("/data", false)},
					}}},
				},
			},
			newCluster: &v1beta1.KafkaCluster{Spec: v1beta1.KafkaClusterSpec{
				Brokers: []v1beta1.Broker{{Id: 0, BrokerConfig: &v1beta1.BrokerConfig{
					StorageConfigs: []v1beta1.StorageConfig{sc("/data", true)},
				}}},
			}},
			expected: append(field.ErrorList{},
				field.Forbidden(
					field.NewPath("spec").Child("brokers").Index(0).Child("brokerConfig").Child("storageConfigs").Index(0).Child("tieredStorageCache"),
					immutableTieredStorageCacheErrMsg,
				),
			),
		},
		{
			// Existing log-dir volume flipped to cache via brokerConfigGroup.
			testName: "false→true flip on existing log-dir volume rejected (group)",
			oldCluster: &v1beta1.KafkaCluster{
				Spec: v1beta1.KafkaClusterSpec{
					Brokers: []v1beta1.Broker{{Id: 0, BrokerConfigGroup: "default"}},
					BrokerConfigGroups: map[string]v1beta1.BrokerConfig{
						"default": {StorageConfigs: []v1beta1.StorageConfig{sc("/data", false)}},
					},
				},
			},
			newCluster: &v1beta1.KafkaCluster{Spec: v1beta1.KafkaClusterSpec{
				Brokers: []v1beta1.Broker{{Id: 0, BrokerConfigGroup: "default"}},
				BrokerConfigGroups: map[string]v1beta1.BrokerConfig{
					"default": {StorageConfigs: []v1beta1.StorageConfig{sc("/data", true)}},
				},
			}},
			expected: append(field.ErrorList{},
				field.Forbidden(
					field.NewPath("spec").Child("brokerConfigGroups").Key("default").Child("storageConfigs").Index(0).Child("tieredStorageCache"),
					immutableTieredStorageCacheErrMsg,
				),
			),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.testName, func(t *testing.T) {
			got := checkTieredStorageCacheImmutability(testCase.oldCluster, testCase.newCluster)
			require.Equal(t, testCase.expected, got)
		})
	}
}

func TestCheckTargetPortsCollisionForEnvoy(t *testing.T) {
	testCases := []struct {
		testName         string
		kafkaClusterSpec v1beta1.KafkaClusterSpec
		expected         field.ErrorList
	}{
		{
			testName: "valid config: envoy admin port, envoy health-check port, and ingress controller target port are not defined",
			kafkaClusterSpec: v1beta1.KafkaClusterSpec{
				ListenersConfig: v1beta1.ListenersConfig{
					ExternalListeners: []v1beta1.ExternalListenerConfig{
						{
							CommonListenerSpec: v1beta1.CommonListenerSpec{Name: "test-external1"},
						},
						{
							CommonListenerSpec: v1beta1.CommonListenerSpec{Name: "test-external2"},
						},
					},
				},
			},
			expected: nil,
		},
		{
			testName: "valid config: external listeners use non-LoadBalancer access method",
			kafkaClusterSpec: v1beta1.KafkaClusterSpec{
				ListenersConfig: v1beta1.ListenersConfig{
					ExternalListeners: []v1beta1.ExternalListenerConfig{
						{
							AccessMethod:                corev1.ServiceTypeNodePort,
							CommonListenerSpec:          v1beta1.CommonListenerSpec{Name: "test-external1"},
							IngressControllerTargetPort: util.Int32Pointer(29000),
						},
						{
							AccessMethod:       corev1.ServiceTypeNodePort,
							CommonListenerSpec: v1beta1.CommonListenerSpec{Name: "test-external2"},
						},
					},
				},
			},
			expected: nil,
		},
		{
			testName: "invalid config: user-specified envoy admin port collides with default envoy health-check port",
			kafkaClusterSpec: v1beta1.KafkaClusterSpec{
				EnvoyConfig: v1beta1.EnvoyConfig{
					AdminPort: util.Int32Pointer(8080),
				},
			},
			expected: append(field.ErrorList{},
				field.Invalid(field.NewPath("spec").Child("envoyConfig").Child("adminPort"), int32(8080),
					invalidContainerPortForIngressControllerErrMsg+": The envoy configuration uses an admin port number that collides with the health-check port number"),
			),
		},
		{
			testName: "invalid config: default envoy admin port collides with user-specified envoy health-check port",
			kafkaClusterSpec: v1beta1.KafkaClusterSpec{
				EnvoyConfig: v1beta1.EnvoyConfig{
					HealthCheckPort: util.Int32Pointer(8081),
				},
			},
			expected: append(field.ErrorList{},
				field.Invalid(field.NewPath("spec").Child("envoyConfig").Child("adminPort"), int32(8081),
					invalidContainerPortForIngressControllerErrMsg+": The envoy configuration uses an admin port number that collides with the health-check port number"),
			),
		},
		{
			testName: "invalid config: user-specified ingress controller target port collided with user-specified envoy admin port and default health-check port",
			kafkaClusterSpec: v1beta1.KafkaClusterSpec{
				EnvoyConfig: v1beta1.EnvoyConfig{
					AdminPort: util.Int32Pointer(29000),
				},
				ListenersConfig: v1beta1.ListenersConfig{
					ExternalListeners: []v1beta1.ExternalListenerConfig{
						{
							CommonListenerSpec:          v1beta1.CommonListenerSpec{Name: "test-external1"},
							IngressControllerTargetPort: util.Int32Pointer(29000),
						},
						{
							CommonListenerSpec:          v1beta1.CommonListenerSpec{Name: "test-external2"},
							IngressControllerTargetPort: util.Int32Pointer(8080),
						},
					},
				},
			},
			expected: append(field.ErrorList{},
				field.Invalid(field.NewPath("spec").Child("listenersConfig").Child("externalListeners").Index(0).Child("ingressControllerTargetPort"), int32(29000),
					invalidContainerPortForIngressControllerErrMsg+": ExternalListener 'test-external1' uses an ingress controller target port number that collides with the envoy's admin port"),
				field.Invalid(field.NewPath("spec").Child("listenersConfig").Child("externalListeners").Index(1).Child("ingressControllerTargetPort"), int32(8080),
					invalidContainerPortForIngressControllerErrMsg+": ExternalListener 'test-external2' uses an ingress controller target port number that collides with the envoy's health-check"+
						" port"),
			),
		},
		{
			testName: "invalid config: user-specified ingress controller target port collided with default envoy admin port and user-specified health-check port",
			kafkaClusterSpec: v1beta1.KafkaClusterSpec{
				EnvoyConfig: v1beta1.EnvoyConfig{
					HealthCheckPort: util.Int32Pointer(19090),
				},
				ListenersConfig: v1beta1.ListenersConfig{
					ExternalListeners: []v1beta1.ExternalListenerConfig{
						{
							CommonListenerSpec:          v1beta1.CommonListenerSpec{Name: "test-external1"},
							IngressControllerTargetPort: util.Int32Pointer(19090),
						},
						{
							CommonListenerSpec:          v1beta1.CommonListenerSpec{Name: "test-external2"},
							IngressControllerTargetPort: util.Int32Pointer(8081),
						},
					},
				},
			},
			expected: append(field.ErrorList{},
				field.Invalid(field.NewPath("spec").Child("listenersConfig").Child("externalListeners").Index(0).Child("ingressControllerTargetPort"), int32(19090),
					invalidContainerPortForIngressControllerErrMsg+": ExternalListener 'test-external1' uses an ingress controller target port number that collides with the envoy's health-check port"),
				field.Invalid(field.NewPath("spec").Child("listenersConfig").Child("externalListeners").Index(1).Child("ingressControllerTargetPort"), int32(8081),
					invalidContainerPortForIngressControllerErrMsg+": ExternalListener 'test-external2' uses an ingress controller target port number that collides with the envoy's admin port"),
			),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.testName, func(t *testing.T) {
			got := checkTargetPortsCollisionForEnvoy(&testCase.kafkaClusterSpec)
			require.Equal(t, testCase.expected, got)
		})
	}
}
