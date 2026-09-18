// Copyright © 2020 Cisco Systems, Inc. and/or its affiliates
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

package cruisecontrol

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/banzaicloud/koperator/api/v1beta1"
)

//nolint:funlen
func TestGenerateCapacityConfig_JBOD(t *testing.T) {
	quantity, _ := resource.ParseQuantity("10Gi")
	oneMiQuantity, _ := resource.ParseQuantity("1Mi")
	cpuQuantity, _ := resource.ParseQuantity("2000m")

	testCases := []struct {
		testName              string
		kafkaCluster          v1beta1.KafkaCluster
		expectedConfiguration string
	}{
		{
			testName: "if config is set manually then use that one",
			kafkaCluster: v1beta1.KafkaCluster{
				Spec: v1beta1.KafkaClusterSpec{
					Brokers: []v1beta1.Broker{
						{
							Id: 0,
							BrokerConfig: &v1beta1.BrokerConfig{
								Resources: &v1.ResourceRequirements{
									Limits: v1.ResourceList{
										"cpu": cpuQuantity,
									}},
							},
						},
						{
							Id: 1,
						},
						{
							Id: 2,
						},
						{
							Id: 4,
						},
					},
					CruiseControlConfig: v1beta1.CruiseControlConfig{
						CapacityConfig: `
                  {
				  "brokerCapacities":[
					{
					  "brokerId": "-1",
					  "capacity": {
						"DISK": {"/tmp/kafka-logs-1": "100000", "/tmp/kafka-logs-2": "100000", "/tmp/kafka-logs-3": "50000",
						  "/tmp/kafka-logs-4": "50000", "/tmp/kafka-logs-5": "150000", "/tmp/kafka-logs-6": "50000"},
						"CPU": "100",
						"NW_IN": "10000",
						"NW_OUT": "10000"
					  },
					  "doc": "The default capacity for a broker with multiple logDirs each on a separate heterogeneous disk."
					},
					{
					  "brokerId": "0",
					  "capacity": {
						"DISK": {"/tmp/kafka-logs": "500000"},
						"CPU": "100",
						"NW_IN": "50000",
						"NW_OUT": "50000"
					  },
					  "doc": "This overrides the capacity for broker 0. This broker is not a JBOD broker."
					},
					{
					  "brokerId": "1",
					  "capacity": {
						"DISK": {"/tmp/kafka-logs-1": "250000", "/tmp/kafka-logs-2": "250000"},
						"CPU": "100",
						"NW_IN": "50000",
						"NW_OUT": "50000"
					  },
					  "doc": "This overrides the capacity for broker 1. This broker is a JBOD broker."
					}
				  ]
				}`,
					},
				},
			},
			expectedConfiguration: `
                  {
				  "brokerCapacities":[
					{
					  "brokerId": "-1",
					  "capacity": {
						"DISK": {"/tmp/kafka-logs-1": "100000", "/tmp/kafka-logs-2": "100000", "/tmp/kafka-logs-3": "50000",
						  "/tmp/kafka-logs-4": "50000", "/tmp/kafka-logs-5": "150000", "/tmp/kafka-logs-6": "50000"},
						"CPU": "100",
						"NW_IN": "10000",
						"NW_OUT": "10000"
					  },
					  "doc": "The default capacity for a broker with multiple logDirs each on a separate heterogeneous disk."
					},
					{
					  "brokerId": "0",
					  "capacity": {
						"DISK": {"/tmp/kafka-logs": "500000"},
						"CPU": "100",
						"NW_IN": "50000",
						"NW_OUT": "50000"
					  },
					  "doc": "This overrides the capacity for broker 0. This broker is not a JBOD broker."
					},
					{
					  "brokerId": "1",
					  "capacity": {
						"DISK": {"/tmp/kafka-logs-1": "250000", "/tmp/kafka-logs-2": "250000"},
						"CPU": "100",
						"NW_IN": "50000",
						"NW_OUT": "50000"
					  },
					  "doc": "This overrides the capacity for broker 1. This broker is a JBOD broker."
					}
				  ]
				}`,
		},
		{
			testName: "generate correct capacity config when there is a broker config group",
			kafkaCluster: v1beta1.KafkaCluster{
				Spec: v1beta1.KafkaClusterSpec{
					BrokerConfigGroups: map[string]v1beta1.BrokerConfig{
						"default": {
							StorageConfigs: []v1beta1.StorageConfig{
								{
									MountPath: "/path-from-default",
									PvcSpec: &v1.PersistentVolumeClaimSpec{
										Resources: v1.VolumeResourceRequirements{
											Requests: v1.ResourceList{
												v1.ResourceStorage: quantity,
											},
										},
									},
								},
							},
						},
					},
					Brokers: []v1beta1.Broker{
						{
							Id:                0,
							BrokerConfigGroup: "default",
							BrokerConfig: &v1beta1.BrokerConfig{
								Resources: &v1.ResourceRequirements{
									Limits: v1.ResourceList{
										"cpu": cpuQuantity,
									}},
							},
						},
						{
							Id:                1,
							BrokerConfigGroup: "default",
						},
						{
							Id:                2,
							BrokerConfigGroup: "default",
						},
						{
							Id:                3,
							BrokerConfigGroup: "default",
							BrokerConfig: &v1beta1.BrokerConfig{
								StorageConfigs: []v1beta1.StorageConfig{
									{
										MountPath: "/path1",
										PvcSpec: &v1.PersistentVolumeClaimSpec{
											Resources: v1.VolumeResourceRequirements{
												Requests: v1.ResourceList{
													v1.ResourceStorage: quantity,
												},
											},
										},
									},
								},
							},
						},
					},
				},
				Status: v1beta1.KafkaClusterStatus{
					BrokersState: map[string]v1beta1.BrokerState{
						"0": {},
						"1": {},
						"2": {},
						"3": {},
					},
				},
			},
			expectedConfiguration: `
				  {
					"brokerCapacities": [
                      {
					  "brokerId": "0",
					  "capacity": {
					   "DISK": {
						"/path-from-default/kafka": "10737"
					   },
					   "CPU": "200",
					   "NW_IN": "125000",
					   "NW_OUT": "125000"
					  },
					  "doc": "Capacity unit used for disk is in MB, cpu is in percentage, network throughput is in KB."
					 },
                     {
					  "brokerId": "1",
					  "capacity": {
					   "DISK": {
						"/path-from-default/kafka": "10737"
					   },
					   "CPU": "150",
					   "NW_IN": "125000",
					   "NW_OUT": "125000"
					  },
					  "doc": "Capacity unit used for disk is in MB, cpu is in percentage, network throughput is in KB."
					 },
                      {
					  "brokerId": "2",
					  "capacity": {
					   "DISK": {
						"/path-from-default/kafka": "10737"
					   },
					   "CPU": "150",
					   "NW_IN": "125000",
					   "NW_OUT": "125000"
					  },
					  "doc": "Capacity unit used for disk is in MB, cpu is in percentage, network throughput is in KB."
					 },
					 {
					  "brokerId": "3",
					  "capacity": {
					   "DISK": {
						"/path1/kafka": "10737",
						"/path-from-default/kafka": "10737"
					   },
					   "CPU": "150",
					   "NW_IN": "125000",
					   "NW_OUT": "125000"
					  },
					  "doc": "Capacity unit used for disk is in MB, cpu is in percentage, network throughput is in KB."
					 }
					]
                  }`,
		},
		{
			testName: "generate correct capacity config when there is a broker missing from spec but present in status",
			kafkaCluster: v1beta1.KafkaCluster{
				Spec: v1beta1.KafkaClusterSpec{
					BrokerConfigGroups: map[string]v1beta1.BrokerConfig{
						"default": {
							StorageConfigs: []v1beta1.StorageConfig{
								{
									MountPath: "/path-from-default",
									PvcSpec: &v1.PersistentVolumeClaimSpec{
										Resources: v1.VolumeResourceRequirements{
											Requests: v1.ResourceList{
												v1.ResourceStorage: quantity,
											},
										},
									},
								},
							},
						},
					},
					Brokers: []v1beta1.Broker{
						{
							Id:                0,
							BrokerConfigGroup: "default",
							BrokerConfig: &v1beta1.BrokerConfig{
								Resources: &v1.ResourceRequirements{
									Limits: v1.ResourceList{
										"cpu": cpuQuantity,
									}},
							},
						},
						{
							Id:                1,
							BrokerConfigGroup: "default",
						},
						{
							Id:                2,
							BrokerConfigGroup: "default",
						},
						{
							Id:                3,
							BrokerConfigGroup: "default",
						},
					},
				},
				Status: v1beta1.KafkaClusterStatus{
					BrokersState: map[string]v1beta1.BrokerState{
						"0": {},
						"1": {},
						"2": {},
						"3": {},
						"4": {},
					},
				},
			},
			expectedConfiguration: `
				  {
					"brokerCapacities": [
                      {
					  "brokerId": "0",
					  "capacity": {
					   "DISK": {
						"/path-from-default/kafka": "10737"
					   },
					   "CPU": "200",
					   "NW_IN": "125000",
					   "NW_OUT": "125000"
					  },
					  "doc": "Capacity unit used for disk is in MB, cpu is in percentage, network throughput is in KB."
					 },
                     {
					  "brokerId": "1",
					  "capacity": {
					   "DISK": {
						"/path-from-default/kafka": "10737"
					   },
					   "CPU": "150",
					   "NW_IN": "125000",
					   "NW_OUT": "125000"
					  },
					  "doc": "Capacity unit used for disk is in MB, cpu is in percentage, network throughput is in KB."
					 },
                     {
					  "brokerId": "2",
					  "capacity": {
					   "DISK": {
						"/path-from-default/kafka": "10737"
					   },
					   "CPU": "150",
					   "NW_IN": "125000",
					   "NW_OUT": "125000"
					  },
					  "doc": "Capacity unit used for disk is in MB, cpu is in percentage, network throughput is in KB."
					 },
                     {
					   "brokerId": "3",
					   "capacity": {
					    "DISK": {
					     "/path-from-default/kafka": "10737"
					    },
					    "CPU": "150",
					    "NW_IN": "125000",
					    "NW_OUT": "125000"
					   },
					   "doc": "Capacity unit used for disk is in MB, cpu is in percentage, network throughput is in KB."
					 },
					 {
					   "brokerId": "4",
					   "capacity": {
					    "DISK": {
							"/kafka-logs/kafka": "10737"
					    },
					    "CPU": "100",
					    "NW_IN": "125000",
					    "NW_OUT": "125000"
					   },
					   "doc": "Capacity unit used for disk is in MB, cpu is in percentage, network throughput is in KB."
					 }
					]
                  }`,
		},
		{
			testName: "generate correct capacity config when storage config is specified as 1Mi ",
			kafkaCluster: v1beta1.KafkaCluster{
				Spec: v1beta1.KafkaClusterSpec{
					BrokerConfigGroups: map[string]v1beta1.BrokerConfig{
						"default": {
							StorageConfigs: []v1beta1.StorageConfig{
								{
									MountPath: "/path-from-default",
									PvcSpec: &v1.PersistentVolumeClaimSpec{
										Resources: v1.VolumeResourceRequirements{
											Requests: v1.ResourceList{
												v1.ResourceStorage: oneMiQuantity,
											},
										},
									},
								},
							},
						},
					},
					Brokers: []v1beta1.Broker{
						{
							Id:                0,
							BrokerConfigGroup: "default",
						},
					},
				},
				Status: v1beta1.KafkaClusterStatus{
					BrokersState: map[string]v1beta1.BrokerState{
						"0": {},
					},
				},
			},
			expectedConfiguration: `
				  {
					"brokerCapacities": [
                      {
					  "brokerId": "0",
					  "capacity": {
					   "DISK": {
						"/path-from-default/kafka": "1"
					   },
					   "CPU": "150",
					   "NW_IN": "125000",
					   "NW_OUT": "125000"
					  },
					  "doc": "Capacity unit used for disk is in MB, cpu is in percentage, network throughput is in KB."
					 }
					]
                  }`,
		},
		{
			testName: "generate correct capacity config when there is no broker config group on last broker",
			kafkaCluster: v1beta1.KafkaCluster{
				Spec: v1beta1.KafkaClusterSpec{
					BrokerConfigGroups: map[string]v1beta1.BrokerConfig{
						"default": {
							StorageConfigs: []v1beta1.StorageConfig{
								{
									MountPath: "/path-from-default",
									PvcSpec: &v1.PersistentVolumeClaimSpec{
										Resources: v1.VolumeResourceRequirements{
											Requests: v1.ResourceList{
												v1.ResourceStorage: quantity,
											},
										},
									},
								},
							},
						},
					},
					Brokers: []v1beta1.Broker{
						{
							Id:                0,
							BrokerConfigGroup: "default",
						},
						{
							Id:                1,
							BrokerConfigGroup: "default",
						},
						{
							Id:                2,
							BrokerConfigGroup: "default",
						},
						{
							Id: 3,
							BrokerConfig: &v1beta1.BrokerConfig{
								NetworkConfig: &v1beta1.NetworkConfig{
									IncomingNetworkThroughPut: "200",
									OutgoingNetworkThroughPut: "200",
								},
								StorageConfigs: []v1beta1.StorageConfig{
									{
										MountPath: "/path1",
										PvcSpec: &v1.PersistentVolumeClaimSpec{
											Resources: v1.VolumeResourceRequirements{
												Requests: v1.ResourceList{
													v1.ResourceStorage: quantity,
												},
											},
										},
									},
								},
							},
						},
					},
				},
				Status: v1beta1.KafkaClusterStatus{
					BrokersState: map[string]v1beta1.BrokerState{
						"0": {},
						"1": {},
						"2": {},
						"3": {},
					},
				},
			},
			expectedConfiguration: `{
					"brokerCapacities": [
                      {
					  "brokerId": "0",
					  "capacity": {
					   "DISK": {
						"/path-from-default/kafka": "10737"
					   },
					   "CPU": "150",
					   "NW_IN": "125000",
					   "NW_OUT": "125000"
					  },
					  "doc": "Capacity unit used for disk is in MB, cpu is in percentage, network throughput is in KB."
					 },
                     {
					  "brokerId": "1",
					  "capacity": {
					   "DISK": {
						"/path-from-default/kafka": "10737"
					   },
					   "CPU": "150",
					   "NW_IN": "125000",
					   "NW_OUT": "125000"
					  },
					  "doc": "Capacity unit used for disk is in MB, cpu is in percentage, network throughput is in KB."
					 },
                      {
					  "brokerId": "2",
					  "capacity": {
					   "DISK": {
						"/path-from-default/kafka": "10737"
					   },
					   "CPU": "150",
					   "NW_IN": "125000",
					   "NW_OUT": "125000"
					  },
					  "doc": "Capacity unit used for disk is in MB, cpu is in percentage, network throughput is in KB."
					 },
					 {
					  "brokerId": "3",
					  "capacity": {
					   "DISK": {
						"/path1/kafka": "10737"
					   },
					   "CPU": "150",
					   "NW_IN": "200",
					   "NW_OUT": "200"
					  },
					  "doc": "Capacity unit used for disk is in MB, cpu is in percentage, network throughput is in KB."
					 }
					]
                  }`,
		},
	}

	t.Parallel()

	for _, test := range testCases {
		test := test

		t.Run(test.testName, func(t *testing.T) {
			var actual CapacityConfig
			rawStringActual, _ := GenerateCapacityConfig(&test.kafkaCluster, logr.Discard(), nil)
			err := json.Unmarshal([]byte(rawStringActual), &actual)
			if err != nil {
				t.Error(err, "could not unmarshal actual json")
			}

			var expected CapacityConfig
			err = json.Unmarshal([]byte(test.expectedConfiguration), &expected)
			if err != nil {
				t.Error(err, "could not unmarshal expected json")
			}

			if !reflect.DeepEqual(actual, expected) {
				t.Error("Expected:", expected, ", got:", actual)
			}
		})
	}
}

//nolint:funlen
func TestReturnErrorStorageConfigLessThan1MB(t *testing.T) {
	// return error when storage config is specified as 500Ki

	fiveHundredKiQuantity, _ := resource.ParseQuantity("500Ki")
	kafkaCluster := v1beta1.KafkaCluster{
		Spec: v1beta1.KafkaClusterSpec{
			BrokerConfigGroups: map[string]v1beta1.BrokerConfig{
				"default": {
					StorageConfigs: []v1beta1.StorageConfig{
						{
							MountPath: "/path-from-default",
							PvcSpec: &v1.PersistentVolumeClaimSpec{
								Resources: v1.VolumeResourceRequirements{
									Requests: v1.ResourceList{
										v1.ResourceStorage: fiveHundredKiQuantity,
									},
								},
							},
						},
					},
				},
			},
			Brokers: []v1beta1.Broker{
				{
					Id:                0,
					BrokerConfigGroup: "default",
				},
			},
		},
		Status: v1beta1.KafkaClusterStatus{
			BrokersState: map[string]v1beta1.BrokerState{
				"0": {},
			},
		},
	}

	_, err := GenerateCapacityConfig(&kafkaCluster, logr.Discard(), nil)

	if err == nil {
		t.Error("Expected error to be thrown when storage config < 1MB")
	}
}

// TestGenerateCapacityConfigKeepsDiskPendingRemoval reproduces the bug where a disk removed from the
// KafkaCluster spec disappeared from Cruise Control's capacity.json immediately, even though the
// broker's log.dirs and PVC mount still kept it around while Cruise Control's own disk
// removal/rebalance for that mount path was still in progress. This caused Cruise Control to throw a
// permanent "Missing disk information" error because its model never had a Disk entry for the still
// actively used log dir. The fix additively keeps such disks in capacity.json (recovering their size
// from the previous capacity.json) until the removal/rebalance is confirmed to have succeeded, and
// mirrors the same keep/drop contract as kafkautils.KeepRemovedVolume for every CruiseControlVolumeState.
//
//nolint:funlen
func TestGenerateCapacityConfigKeepsDiskPendingRemoval(t *testing.T) {
	quantity, _ := resource.ParseQuantity("10Gi")

	testCases := []struct {
		volumeState v1beta1.CruiseControlVolumeState
		expectKeep  bool
	}{
		{v1beta1.GracefulDiskRemovalRequired, true},
		{v1beta1.GracefulDiskRemovalRunning, true},
		{v1beta1.GracefulDiskRemovalScheduled, true},
		{v1beta1.GracefulDiskRemovalCompletedWithError, true},
		{v1beta1.GracefulDiskRemovalPaused, true},
		{v1beta1.GracefulDiskRemovalSucceeded, false},
		{v1beta1.GracefulDiskRebalanceRequired, true},
		{v1beta1.GracefulDiskRebalanceRunning, true},
		{v1beta1.GracefulDiskRebalanceScheduled, true},
		{v1beta1.GracefulDiskRebalanceCompletedWithError, true},
		{v1beta1.GracefulDiskRebalancePaused, true},
		{v1beta1.GracefulDiskRebalanceSucceeded, false},
	}

	for _, test := range testCases {
		test := test
		t.Run(string(test.volumeState), func(t *testing.T) {
			kafkaCluster := v1beta1.KafkaCluster{
				Spec: v1beta1.KafkaClusterSpec{
					Brokers: []v1beta1.Broker{
						{
							Id: 0,
							BrokerConfig: &v1beta1.BrokerConfig{
								StorageConfigs: []v1beta1.StorageConfig{
									{
										MountPath: "/kafka-logs1",
										PvcSpec: &v1.PersistentVolumeClaimSpec{
											Resources: v1.VolumeResourceRequirements{
												Requests: v1.ResourceList{
													v1.ResourceStorage: quantity,
												},
											},
										},
									},
									// Note: /kafka-logs3 has already been removed from the desired spec.
								},
							},
						},
					},
				},
				Status: v1beta1.KafkaClusterStatus{
					BrokersState: map[string]v1beta1.BrokerState{
						"0": {
							GracefulActionState: v1beta1.GracefulActionState{
								VolumeStates: map[string]v1beta1.VolumeState{
									"/kafka-logs3": {
										CruiseControlVolumeState: test.volumeState,
									},
								},
							},
						},
					},
				},
			}

			oldConfig := &v1.ConfigMap{
				Data: map[string]string{
					"capacity.json": `{
						"brokerCapacities": [
							{
								"brokerId": "0",
								"capacity": {
									"DISK": {
										"/kafka-logs1/kafka": "10737",
										"/kafka-logs3/kafka": "20000"
									},
									"CPU": "100",
									"NW_IN": "125000",
									"NW_OUT": "125000"
								},
								"doc": "Capacity unit used for disk is in MB, cpu is in percentage, network throughput is in KB."
							}
						]
					}`,
				},
			}

			rawStringActual, err := GenerateCapacityConfig(&kafkaCluster, logr.Discard(), oldConfig)
			if err != nil {
				t.Fatal("unexpected error:", err)
			}

			var actual JBODInvariantCapacityConfig
			if err := json.Unmarshal([]byte(rawStringActual), &actual); err != nil {
				t.Fatal("could not unmarshal actual json:", err)
			}

			if len(actual.Capacities) != 1 {
				t.Fatalf("expected 1 broker capacity, got %d", len(actual.Capacities))
			}
			brokerCapacityMap, ok := actual.Capacities[0].(map[string]interface{})
			if !ok {
				t.Fatal("could not cast broker capacity to map")
			}
			diskMap, ok := brokerCapacityMap["capacity"].(map[string]interface{})["DISK"].(map[string]interface{})
			if !ok {
				t.Fatal("could not cast DISK to map")
			}

			if _, found := diskMap["/kafka-logs1/kafka"]; !found {
				t.Error("expected desired disk /kafka-logs1/kafka to be present")
			}
			size, found := diskMap["/kafka-logs3/kafka"]
			if test.expectKeep {
				if !found {
					t.Fatal("expected disk pending removal /kafka-logs3/kafka to still be present in capacity.json")
				}
				if size != "20000" {
					t.Errorf("expected disk /kafka-logs3/kafka to keep its recovered size 20000, got %v", size)
				}
			} else if found {
				t.Errorf("expected disk /kafka-logs3/kafka to have been dropped once removal succeeded, got size %v", size)
			}
		})
	}
}

//nolint:funlen
func TestGenerateCapacityConfigRequiresExactCapacityForPendingDisk(t *testing.T) {
	quantity, _ := resource.ParseQuantity("10Gi")
	kafkaCluster := v1beta1.KafkaCluster{
		Spec: v1beta1.KafkaClusterSpec{
			Brokers: []v1beta1.Broker{
				{
					Id: 0,
					BrokerConfig: &v1beta1.BrokerConfig{
						StorageConfigs: []v1beta1.StorageConfig{
							{
								MountPath: "/kafka-logs1",
								PvcSpec: &v1.PersistentVolumeClaimSpec{
									Resources: v1.VolumeResourceRequirements{
										Requests: v1.ResourceList{
											v1.ResourceStorage: quantity,
										},
									},
								},
							},
						},
					},
				},
			},
		},
		Status: v1beta1.KafkaClusterStatus{
			BrokersState: map[string]v1beta1.BrokerState{
				"0": {
					GracefulActionState: v1beta1.GracefulActionState{
						VolumeStates: map[string]v1beta1.VolumeState{
							"/kafka-logs3": {
								CruiseControlVolumeState: v1beta1.GracefulDiskRemovalRunning,
							},
						},
					},
				},
			},
		},
	}

	testCases := []struct {
		name      string
		oldConfig *v1.ConfigMap
	}{
		{
			name: "configmap unavailable",
		},
		{
			name:      "capacity json unavailable",
			oldConfig: &v1.ConfigMap{Data: map[string]string{}},
		},
		{
			name: "capacity json malformed",
			oldConfig: &v1.ConfigMap{
				Data: map[string]string{"capacity.json": "{"},
			},
		},
		{
			name: "pending disk missing",
			oldConfig: &v1.ConfigMap{
				Data: map[string]string{
					"capacity.json": `{
						"brokerCapacities": [{
							"brokerId": "0",
							"capacity": {
								"DISK": {"/kafka-logs1/kafka": "10737"},
								"CPU": "100",
								"NW_IN": "125000",
								"NW_OUT": "125000"
							},
							"doc": "capacity"
						}]
					}`,
				},
			},
		},
	}

	for _, test := range testCases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			_, err := GenerateCapacityConfig(&kafkaCluster, logr.Discard(), test.oldConfig)
			if err == nil {
				t.Fatal("expected an error when exact prior capacity for pending disk is unavailable")
			}
		})
	}
}

//nolint:funlen
func TestGenerateCapacityConfigParsesOldBrokerEntriesIndependently(t *testing.T) {
	quantity, _ := resource.ParseQuantity("10Gi")
	kafkaCluster := v1beta1.KafkaCluster{
		Spec: v1beta1.KafkaClusterSpec{
			Brokers: []v1beta1.Broker{
				{
					Id: 0,
					BrokerConfig: &v1beta1.BrokerConfig{
						StorageConfigs: []v1beta1.StorageConfig{
							{
								MountPath: "/kafka-logs1",
								PvcSpec: &v1.PersistentVolumeClaimSpec{
									Resources: v1.VolumeResourceRequirements{
										Requests: v1.ResourceList{
											v1.ResourceStorage: quantity,
										},
									},
								},
							},
						},
					},
				},
			},
		},
		Status: v1beta1.KafkaClusterStatus{
			BrokersState: map[string]v1beta1.BrokerState{
				"0": {
					GracefulActionState: v1beta1.GracefulActionState{
						VolumeStates: map[string]v1beta1.VolumeState{
							"/kafka-logs3": {
								CruiseControlVolumeState: v1beta1.GracefulDiskRemovalRunning,
							},
						},
					},
				},
				"1": {
					GracefulActionState: v1beta1.GracefulActionState{
						CruiseControlState: v1beta1.GracefulDownscaleRunning,
					},
				},
			},
		},
	}
	oldConfig := &v1.ConfigMap{
		Data: map[string]string{
			"capacity.json": `{
				"brokerCapacities": [
					{
						"brokerId": "1",
						"capacity": {
							"DISK": "500000",
							"CPU": "200",
							"NW_IN": "250000",
							"NW_OUT": "250000"
						},
						"doc": "scalar disk capacity"
					},
					{
						"brokerId": "0",
						"capacity": {
							"DISK": {
								"/kafka-logs1/kafka": "10737",
								"/kafka-logs3/kafka": "20000"
							},
							"CPU": "100",
							"NW_IN": "125000",
							"NW_OUT": "125000"
						},
						"doc": "JBOD capacity"
					}
				]
			}`,
		},
	}

	actualJSON, err := GenerateCapacityConfig(&kafkaCluster, logr.Discard(), oldConfig)
	if err != nil {
		t.Fatal("unexpected error:", err)
	}

	var actual struct {
		BrokerCapacities []struct {
			BrokerID string `json:"brokerId"`
			Capacity struct {
				DISK json.RawMessage `json:"DISK"`
			} `json:"capacity"`
		} `json:"brokerCapacities"`
	}
	if err := json.Unmarshal([]byte(actualJSON), &actual); err != nil {
		t.Fatal("could not unmarshal generated capacity config:", err)
	}

	disksByBroker := make(map[string]json.RawMessage, len(actual.BrokerCapacities))
	for _, brokerCapacity := range actual.BrokerCapacities {
		disksByBroker[brokerCapacity.BrokerID] = brokerCapacity.Capacity.DISK
	}

	var scalarDisk string
	if err := json.Unmarshal(disksByBroker["1"], &scalarDisk); err != nil {
		t.Fatal("expected removed broker's scalar DISK entry to be preserved:", err)
	}
	if scalarDisk != "500000" {
		t.Errorf("expected removed broker's scalar DISK capacity 500000, got %q", scalarDisk)
	}

	var jbodDisks map[string]string
	if err := json.Unmarshal(disksByBroker["0"], &jbodDisks); err != nil {
		t.Fatal("expected active broker's JBOD DISK entry:", err)
	}
	if jbodDisks["/kafka-logs3/kafka"] != "20000" {
		t.Errorf("expected pending disk capacity 20000, got %q", jbodDisks["/kafka-logs3/kafka"])
	}
}

// TestGenerateCapacityConfigIsIdempotentAcrossReconciles guards against a regression class where
// GenerateCapacityConfig would produce a different capacity.json on every reconcile even though nothing
// relevant changed (e.g. re-deriving CPU/NW values for still-in-spec brokers, or re-deriving the
// removed broker's placeholder capacity differently each time). Such churn would change the Cruise
// Control ConfigMap's content hash on every reconcile, forcing an unnecessary Cruise Control pod
// restart during an in-progress downscale/disk-removal/rebalance -- the same failure class this fix
// addresses. Regenerating the config from its own previous output (round-tripping it as "config") must
// be a no-op both for the broker-deletion-in-progress case (broker missing from spec) and for the
// disk-removal/rebalance-in-progress case (disk missing from spec).
func TestGenerateCapacityConfigIsIdempotentAcrossReconciles(t *testing.T) {
	quantity, _ := resource.ParseQuantity("10Gi")
	cpuQuantity, _ := resource.ParseQuantity("2000m")

	kafkaCluster := v1beta1.KafkaCluster{
		Spec: v1beta1.KafkaClusterSpec{
			BrokerConfigGroups: map[string]v1beta1.BrokerConfig{
				"default": {
					StorageConfigs: []v1beta1.StorageConfig{
						{
							MountPath: "/path-from-default",
							PvcSpec: &v1.PersistentVolumeClaimSpec{
								Resources: v1.VolumeResourceRequirements{
									Requests: v1.ResourceList{
										v1.ResourceStorage: quantity,
									},
								},
							},
						},
					},
				},
			},
			// Broker "1" has been removed from the spec (downscale/deletion in progress) and
			// /kafka-logs3 has been removed from broker "0"'s storage configs (disk removal in
			// progress), both while still present in Status.BrokersState below.
			Brokers: []v1beta1.Broker{
				{
					Id:                0,
					BrokerConfigGroup: "default",
					BrokerConfig: &v1beta1.BrokerConfig{
						Resources: &v1.ResourceRequirements{
							Limits: v1.ResourceList{
								"cpu": cpuQuantity,
							},
						},
					},
				},
			},
		},
		Status: v1beta1.KafkaClusterStatus{
			BrokersState: map[string]v1beta1.BrokerState{
				"0": {
					GracefulActionState: v1beta1.GracefulActionState{
						VolumeStates: map[string]v1beta1.VolumeState{
							"/kafka-logs3": {
								CruiseControlVolumeState: v1beta1.GracefulDiskRemovalRunning,
							},
						},
					},
				},
				"1": {
					GracefulActionState: v1beta1.GracefulActionState{
						CruiseControlState: v1beta1.GracefulDownscaleRunning,
					},
				},
			},
		},
	}
	// Broker "1" is marked as being downscaled (GracefulDownscaleRunning) so isBrokerDeletionInProgress
	// (exercised via the reconciler, not directly here) would trigger fetching the old config;
	// GenerateCapacityConfig itself only needs the broker to be missing from Spec.Brokers to hit the
	// "reuse old entry" branch.

	// Broker "0"'s CPU/NW_IN/NW_OUT deliberately mismatch what generateBrokerCPU/NetworkIn/NetworkOut
	// would derive from the spec above, so the first pass is forced to actually recompute them from
	// spec rather than trivially reproducing an already-correct seed value. This proves real
	// convergence (stale-seed -> spec-derived value, stable thereafter), not just stability at a
	// pre-matched fixed point.
	seedConfig := &v1.ConfigMap{
		Data: map[string]string{
			"capacity.json": `{
				"brokerCapacities": [
					{
						"brokerId": "0",
						"capacity": {
							"DISK": {
								"/path-from-default/kafka": "10737",
								"/kafka-logs3/kafka": "20000"
							},
							"CPU": "999",
							"NW_IN": "1",
							"NW_OUT": "1"
						},
						"doc": "active broker capacity"
					},
					{
						"brokerId": "1",
						"capacity": {
							"DISK": "500000",
							"CPU": "100",
							"NW_IN": "125000",
							"NW_OUT": "125000"
						},
						"doc": "removed broker capacity"
					}
				]
			}`,
		},
	}

	firstPass, err := GenerateCapacityConfig(&kafkaCluster, logr.Discard(), seedConfig)
	if err != nil {
		t.Fatal("unexpected error on first pass:", err)
	}

	if strings.Contains(firstPass, `"CPU": "999"`) || strings.Contains(firstPass, `"NW_IN": "1"`) {
		t.Fatalf("expected first pass to recompute broker 0's CPU/NW_IN from spec rather than reuse the stale seed value, got:\n%s", firstPass)
	}

	oldConfig := &v1.ConfigMap{Data: map[string]string{"capacity.json": firstPass}}
	secondPass, err := GenerateCapacityConfig(&kafkaCluster, logr.Discard(), oldConfig)
	if err != nil {
		t.Fatal("unexpected error on second pass:", err)
	}

	if firstPass != secondPass {
		t.Errorf("expected capacity config to be idempotent across reconciles, but it changed:\nfirst:\n%s\nsecond:\n%s", firstPass, secondPass)
	}

	// A third pass, reusing the second pass's output, must also be stable (guards against slow drift).
	oldConfig2 := &v1.ConfigMap{Data: map[string]string{"capacity.json": secondPass}}
	thirdPass, err := GenerateCapacityConfig(&kafkaCluster, logr.Discard(), oldConfig2)
	if err != nil {
		t.Fatal("unexpected error on third pass:", err)
	}
	if secondPass != thirdPass {
		t.Errorf("expected capacity config to remain stable on a third reconcile, but it changed:\nsecond:\n%s\nthird:\n%s", secondPass, thirdPass)
	}
}

//nolint:funlen
func TestGenerateCapacityConfigWithUserProvidedInput(t *testing.T) {
	cpuQuantity, _ := resource.ParseQuantity("2000m")
	testCases := []struct {
		testName              string
		capacityConfig        string
		expectedConfiguration string
	}{
		{
			testName: "JBOD case, without default broker",
			capacityConfig: `
                  {
				  "brokerCapacities":[
					{
					  "brokerId": "0",
					  "capacity": {
						"DISK": {"/tmp/kafka-logs": "500000"},
						"CPU": "100",
						"NW_IN": "50000",
						"NW_OUT": "50000"
					  },
					  "doc": "This overrides the capacity for broker 0. This broker is not a JBOD broker."
					},
					{
					  "brokerId": "1",
					  "capacity": {
						"DISK": {"/tmp/kafka-logs-1": "250000", "/tmp/kafka-logs-2": "250000"},
						"CPU": "100",
						"NW_IN": "50000",
						"NW_OUT": "50000"
					  },
					  "doc": "This overrides the capacity for broker 1. This broker is a JBOD broker."
					}
				  ]
				}`,
			expectedConfiguration: `
                  {
				  "brokerCapacities":[
					{
					  "brokerId": "0",
					  "capacity": {
						"DISK": {"/tmp/kafka-logs": "500000"},
						"CPU": "100",
						"NW_IN": "50000",
						"NW_OUT": "50000"
					  },
					  "doc": "This overrides the capacity for broker 0. This broker is not a JBOD broker."
					},
					{
					  "brokerId": "1",
					  "capacity": {
						"DISK": {"/tmp/kafka-logs-1": "250000", "/tmp/kafka-logs-2": "250000"},
						"CPU": "100",
						"NW_IN": "50000",
						"NW_OUT": "50000"
					  },
					  "doc": "This overrides the capacity for broker 1. This broker is a JBOD broker."
					},
					{
						"brokerId": "2",
						"capacity": {
						  "DISK": {},
						  "CPU": "200",
						  "NW_IN": "125000",
						  "NW_OUT": "125000"
						},
						"doc": "Capacity unit used for disk is in MB, cpu is in percentage, network throughput is in KB."
					},
					{
						"brokerId": "4",
						"capacity": {
						  "DISK": {"/path1/kafka": "100"},
						  "CPU": "200",
						  "NW_IN": "125000",
						  "NW_OUT": "125000"
						},
						"doc": "Capacity unit used for disk is in MB, cpu is in percentage, network throughput is in KB."
					}
				  ]
				}`,
		},
		{
			testName: "JBOD case, with default broker",
			capacityConfig: `
                  {
				  "brokerCapacities":[
					{
					  "brokerId": "-1",
					  "capacity": {
						"DISK": {"/tmp/kafka-logs-1": "100000", "/tmp/kafka-logs-2": "100000", "/tmp/kafka-logs-3": "50000",
						  "/tmp/kafka-logs-4": "50000", "/tmp/kafka-logs-5": "150000", "/tmp/kafka-logs-6": "50000"},
						"CPU": "100",
						"NW_IN": "10000",
						"NW_OUT": "10000"
					  },
					  "doc": "The default capacity for a broker with multiple logDirs each on a separate heterogeneous disk."
					},
					{
					  "brokerId": "0",
					  "capacity": {
						"DISK": {"/tmp/kafka-logs": "500000"},
						"CPU": "100",
						"NW_IN": "50000",
						"NW_OUT": "50000"
					  },
					  "doc": "This overrides the capacity for broker 0. This broker is not a JBOD broker."
					},
					{
					  "brokerId": "1",
					  "capacity": {
						"DISK": {"/tmp/kafka-logs-1": "250000", "/tmp/kafka-logs-2": "250000"},
						"CPU": "100",
						"NW_IN": "50000",
						"NW_OUT": "50000"
					  },
					  "doc": "This overrides the capacity for broker 1. This broker is a JBOD broker."
					}
				  ]
				}`,
			expectedConfiguration: `
                  {
				  "brokerCapacities":[
					{
					  "brokerId": "-1",
					  "capacity": {
						"DISK": {"/tmp/kafka-logs-1": "100000", "/tmp/kafka-logs-2": "100000", "/tmp/kafka-logs-3": "50000",
						  "/tmp/kafka-logs-4": "50000", "/tmp/kafka-logs-5": "150000", "/tmp/kafka-logs-6": "50000"},
						"CPU": "100",
						"NW_IN": "10000",
						"NW_OUT": "10000"
					  },
					  "doc": "The default capacity for a broker with multiple logDirs each on a separate heterogeneous disk."
					},
					{
					  "brokerId": "0",
					  "capacity": {
						"DISK": {"/tmp/kafka-logs": "500000"},
						"CPU": "100",
						"NW_IN": "50000",
						"NW_OUT": "50000"
					  },
					  "doc": "This overrides the capacity for broker 0. This broker is not a JBOD broker."
					},
					{
					  "brokerId": "1",
					  "capacity": {
						"DISK": {"/tmp/kafka-logs-1": "250000", "/tmp/kafka-logs-2": "250000"},
						"CPU": "100",
						"NW_IN": "50000",
						"NW_OUT": "50000"
					  },
					  "doc": "This overrides the capacity for broker 1. This broker is a JBOD broker."
					}
				  ]
				}`,
		},
		{
			testName: "without JBOD and default broker",
			capacityConfig: `
                  {
				  "brokerCapacities":[
					{
					  "brokerId": "0",
					  "capacity": {
						"DISK": "500000",
						"CPU": "100",
						"NW_IN": "50000",
						"NW_OUT": "50000"
					  },
					  "doc": "This overrides the capacity for broker 0. This broker is not a JBOD broker."
					},
					{
					  "brokerId": "1",
					  "capacity": {
						"DISK": "250000",
						"CPU": "100",
						"NW_IN": "50000",
						"NW_OUT": "50000"
					  },
					  "doc": "This overrides the capacity for broker 1. This broker is a JBOD broker."
					}
				  ]
				}`,
			expectedConfiguration: `
                  {
				  "brokerCapacities":[
					{
					  "brokerId": "0",
					  "capacity": {
						"DISK": "500000",
						"CPU": "100",
						"NW_IN": "50000",
						"NW_OUT": "50000"
					  },
					  "doc": "This overrides the capacity for broker 0. This broker is not a JBOD broker."
					},
					{
					  "brokerId": "1",
					  "capacity": {
						"DISK": "250000",
						"CPU": "100",
						"NW_IN": "50000",
						"NW_OUT": "50000"
					  },
					  "doc": "This overrides the capacity for broker 1. This broker is a JBOD broker."
					},
					{
						"brokerId": "2",
						"capacity": {
						  "DISK": {},
						  "CPU": "200",
						  "NW_IN": "125000",
						  "NW_OUT": "125000"
						},
						"doc": "Capacity unit used for disk is in MB, cpu is in percentage, network throughput is in KB."
					},
					{
						"brokerId": "4",
						"capacity": {
						  "DISK": {"/path1/kafka": "100"},
						  "CPU": "200",
						  "NW_IN": "125000",
						  "NW_OUT": "125000"
						},
						"doc": "Capacity unit used for disk is in MB, cpu is in percentage, network throughput is in KB."
					}
				  ]
				}`,
		},
		{
			testName: "without JBOD case, but with default broker",
			capacityConfig: `
                  {
				  "brokerCapacities":[
					{
					  "brokerId": "-1",
					  "capacity": {
						"DISK": "100000",
						"CPU": "100",
						"NW_IN": "10000",
						"NW_OUT": "10000"
					  },
					  "doc": "The default capacity for a broker with multiple logDirs each on a separate heterogeneous disk."
					},
					{
					  "brokerId": "0",
					  "capacity": {
						"DISK": "500000",
						"CPU": "100",
						"NW_IN": "50000",
						"NW_OUT": "50000"
					  },
					  "doc": "This overrides the capacity for broker 0. This broker is not a JBOD broker."
					},
					{
					  "brokerId": "1",
					  "capacity": {
						"DISK": "250000",
						"CPU": "100",
						"NW_IN": "50000",
						"NW_OUT": "50000"
					  },
					  "doc": "This overrides the capacity for broker 1. This broker is a JBOD broker."
					}
				  ]
				}`,
			expectedConfiguration: `
                  {
				  "brokerCapacities":[
					{
					  "brokerId": "-1",
					  "capacity": {
						"DISK": "100000",
						"CPU": "100",
						"NW_IN": "10000",
						"NW_OUT": "10000"
					  },
					  "doc": "The default capacity for a broker with multiple logDirs each on a separate heterogeneous disk."
					},
					{
					  "brokerId": "0",
					  "capacity": {
						"DISK": "500000",
						"CPU": "100",
						"NW_IN": "50000",
						"NW_OUT": "50000"
					  },
					  "doc": "This overrides the capacity for broker 0. This broker is not a JBOD broker."
					},
					{
					  "brokerId": "1",
					  "capacity": {
						"DISK": "250000",
						"CPU": "100",
						"NW_IN": "50000",
						"NW_OUT": "50000"
					  },
					  "doc": "This overrides the capacity for broker 1. This broker is a JBOD broker."
					}
				  ]
				}`,
		},
	}

	t.Parallel()

	for _, test := range testCases {
		test := test

		t.Run(test.testName, func(t *testing.T) {
			kafkaCluster := v1beta1.KafkaCluster{
				Spec: v1beta1.KafkaClusterSpec{
					Brokers: []v1beta1.Broker{
						{
							Id: 0,
							BrokerConfig: &v1beta1.BrokerConfig{
								Resources: &v1.ResourceRequirements{
									Limits: v1.ResourceList{
										"cpu": cpuQuantity,
									}},
							},
						},
						{
							Id: 1,
							BrokerConfig: &v1beta1.BrokerConfig{
								Resources: &v1.ResourceRequirements{
									Limits: v1.ResourceList{
										"cpu": cpuQuantity,
									}},
							},
						},
						{
							Id: 2,
							BrokerConfig: &v1beta1.BrokerConfig{
								Resources: &v1.ResourceRequirements{
									Limits: v1.ResourceList{
										"cpu": cpuQuantity,
									}},
							},
						},
						{
							Id: 4,
							BrokerConfig: &v1beta1.BrokerConfig{
								Resources: &v1.ResourceRequirements{
									Limits: v1.ResourceList{
										"cpu": cpuQuantity,
									}},
								StorageConfigs: []v1beta1.StorageConfig{
									{
										MountPath: "/path1",
										PvcSpec: &v1.PersistentVolumeClaimSpec{
											Resources: v1.VolumeResourceRequirements{
												Requests: v1.ResourceList{
													v1.ResourceStorage: resource.MustParse("100M"),
												},
											},
										},
									},
								},
							},
						},
					},
					CruiseControlConfig: v1beta1.CruiseControlConfig{
						CapacityConfig: test.capacityConfig,
					},
				},
				Status: v1beta1.KafkaClusterStatus{
					BrokersState: map[string]v1beta1.BrokerState{
						"0": {},
						"1": {},
						"2": {},
						"4": {},
					},
				},
			}
			var actual JBODInvariantCapacityConfig
			rawStringActual, _ := GenerateCapacityConfig(&kafkaCluster, logr.Discard(), nil)
			err := json.Unmarshal([]byte(rawStringActual), &actual)
			if err != nil {
				t.Error(err, "could not unmarshal actual json")
			}

			var expected JBODInvariantCapacityConfig
			err = json.Unmarshal([]byte(test.expectedConfiguration), &expected)
			if err != nil {
				t.Error(err, "could not unmarshal expected json")
			}

			if !reflect.DeepEqual(actual, expected) {
				t.Error("Expected:", expected, ", got:", actual)
			}
		})
	}
}
