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

func TestCheckKRaftConfig(t *testing.T) {
	interBrokerListener := v1beta1.InternalListenerConfig{
		CommonListenerSpec: v1beta1.CommonListenerSpec{
			Name:                            "internal",
			ContainerPort:                   29092,
			UsedForInnerBrokerCommunication: true,
		},
	}
	controllerListener := v1beta1.InternalListenerConfig{
		CommonListenerSpec:             v1beta1.CommonListenerSpec{Name: "controller", ContainerPort: 29093},
		UsedForControllerCommunication: true,
	}
	// A controller's storage config must provision exactly one PVC (matches the reconcile-time constraint).
	onePVCStorage := []v1beta1.StorageConfig{{MountPath: "/kafka-logs", PvcSpec: &corev1.PersistentVolumeClaimSpec{}}}
	// One PVC plus an emptyDir is still a single PVC-backed volume, so it is valid for a controller.
	onePVCPlusEmptyDir := []v1beta1.StorageConfig{
		{MountPath: "/kafka-logs", PvcSpec: &corev1.PersistentVolumeClaimSpec{}},
		{MountPath: "/ephemeral", EmptyDir: &corev1.EmptyDirVolumeSource{}},
	}
	twoPVCStorage := []v1beta1.StorageConfig{
		{MountPath: "/kafka-logs", PvcSpec: &corev1.PersistentVolumeClaimSpec{}},
		{MountPath: "/kafka-logs-2", PvcSpec: &corev1.PersistentVolumeClaimSpec{}},
	}

	validListeners := v1beta1.ListenersConfig{
		InternalListeners: []v1beta1.InternalListenerConfig{interBrokerListener, controllerListener},
	}

	testCases := []struct {
		testName         string
		kafkaClusterSpec v1beta1.KafkaClusterSpec
		expectedErrCount int
	}{
		{
			testName: "not KRaft mode is a no-op even with an otherwise invalid layout",
			kafkaClusterSpec: v1beta1.KafkaClusterSpec{
				KRaftMode:       false,
				ListenersConfig: v1beta1.ListenersConfig{},
				Brokers:         []v1beta1.Broker{{Id: 0, BrokerConfig: &v1beta1.BrokerConfig{}}},
			},
			expectedErrCount: 0,
		},
		{
			testName: "valid KRaft cluster: broker-only, controller-only and combined nodes",
			kafkaClusterSpec: v1beta1.KafkaClusterSpec{
				KRaftMode:       true,
				ListenersConfig: validListeners,
				Brokers: []v1beta1.Broker{
					{Id: 0, BrokerConfig: &v1beta1.BrokerConfig{Roles: []string{"broker"}, StorageConfigs: twoPVCStorage}},
					{Id: 1, BrokerConfig: &v1beta1.BrokerConfig{Roles: []string{"controller"}, StorageConfigs: onePVCStorage}},
					{Id: 2, BrokerConfig: &v1beta1.BrokerConfig{Roles: []string{"broker", "controller"}, StorageConfigs: onePVCPlusEmptyDir}},
				},
			},
			expectedErrCount: 0,
		},
		{
			testName: "missing controller listener",
			kafkaClusterSpec: v1beta1.KafkaClusterSpec{
				KRaftMode:       true,
				ListenersConfig: v1beta1.ListenersConfig{InternalListeners: []v1beta1.InternalListenerConfig{interBrokerListener}},
				Brokers: []v1beta1.Broker{
					{Id: 1, BrokerConfig: &v1beta1.BrokerConfig{Roles: []string{"controller"}, StorageConfigs: onePVCStorage}},
				},
			},
			expectedErrCount: 1,
		},
		{
			testName: "more than one controller listener",
			kafkaClusterSpec: v1beta1.KafkaClusterSpec{
				KRaftMode: true,
				ListenersConfig: v1beta1.ListenersConfig{InternalListeners: []v1beta1.InternalListenerConfig{
					controllerListener,
					{CommonListenerSpec: v1beta1.CommonListenerSpec{Name: "controller2", ContainerPort: 29094}, UsedForControllerCommunication: true},
				}},
				Brokers: []v1beta1.Broker{
					{Id: 1, BrokerConfig: &v1beta1.BrokerConfig{Roles: []string{"controller"}, StorageConfigs: onePVCStorage}},
				},
			},
			expectedErrCount: 1,
		},
		{
			testName: "no controller node",
			kafkaClusterSpec: v1beta1.KafkaClusterSpec{
				KRaftMode:       true,
				ListenersConfig: validListeners,
				Brokers: []v1beta1.Broker{
					{Id: 0, BrokerConfig: &v1beta1.BrokerConfig{Roles: []string{"broker"}, StorageConfigs: onePVCStorage}},
				},
			},
			expectedErrCount: 1,
		},
		{
			testName: "broker with no process role",
			kafkaClusterSpec: v1beta1.KafkaClusterSpec{
				KRaftMode:       true,
				ListenersConfig: validListeners,
				Brokers: []v1beta1.Broker{
					{Id: 0, BrokerConfig: &v1beta1.BrokerConfig{Roles: []string{}, StorageConfigs: onePVCStorage}},
					{Id: 1, BrokerConfig: &v1beta1.BrokerConfig{Roles: []string{"controller"}, StorageConfigs: onePVCStorage}},
				},
			},
			expectedErrCount: 1,
		},
		{
			testName: "controller node with more than one storage config",
			kafkaClusterSpec: v1beta1.KafkaClusterSpec{
				KRaftMode:       true,
				ListenersConfig: validListeners,
				Brokers: []v1beta1.Broker{
					{Id: 1, BrokerConfig: &v1beta1.BrokerConfig{Roles: []string{"controller"}, StorageConfigs: twoPVCStorage}},
				},
			},
			expectedErrCount: 1,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.testName, func(t *testing.T) {
			got := checkKRaftConfig(&testCase.kafkaClusterSpec)
			require.Len(t, got, testCase.expectedErrCount, "unexpected errors: %v", got)
		})
	}
}
