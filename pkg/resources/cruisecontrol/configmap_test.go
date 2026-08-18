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
	"fmt"
	"reflect"
	"sort"
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

// TestGenerateCapacityConfigReuseAndMerge covers the config-reuse path taken during a scaling operation
// (config != nil): a departing broker's deployed entry must be preserved verbatim (so capacity.json does
// not change and roll Cruise Control mid-removal, see #301), while a newly added broker must get a freshly
// generated entry so add_broker has capacity data even in a mixed add+remove edit.
//
//nolint:funlen
func TestGenerateCapacityConfigReuseAndMerge(t *testing.T) {
	tenGiQuantity, _ := resource.ParseQuantity("10Gi")

	// The already-deployed capacity.json. Broker 100 (the one being removed) carries deliberately unusual
	// values so we can prove it is preserved verbatim rather than rewritten to the fallback default.
	deployedCapacityConfig := `{
    "brokerCapacities": [
        {"brokerId": "0", "capacity": {"DISK": {"/kafka-logs-broker/kafka": "10240"}, "CPU": "100", "NW_IN": "125000", "NW_OUT": "125000"}, "doc": "d"},
        {"brokerId": "1", "capacity": {"DISK": {"/kafka-logs-broker/kafka": "10240"}, "CPU": "100", "NW_IN": "125000", "NW_OUT": "125000"}, "doc": "d"},
        {"brokerId": "2", "capacity": {"DISK": {"/kafka-logs-broker/kafka": "10240"}, "CPU": "100", "NW_IN": "125000", "NW_OUT": "125000"}, "doc": "d"},
        {"brokerId": "100", "capacity": {"DISK": {"/kafka-logs-broker/kafka": "99999"}, "CPU": "777", "NW_IN": "111", "NW_OUT": "222"}, "doc": "departing-verbatim"}
    ]
}`
	deployedConfigMap := &v1.ConfigMap{Data: map[string]string{CapacityConfigMapKey: deployedCapacityConfig}}

	brokerConfigGroups := map[string]v1beta1.BrokerConfig{
		"broker": {
			StorageConfigs: []v1beta1.StorageConfig{
				{
					MountPath: "/kafka-logs-broker",
					PvcSpec: &v1.PersistentVolumeClaimSpec{
						Resources: v1.VolumeResourceRequirements{
							Requests: v1.ResourceList{v1.ResourceStorage: tenGiQuantity},
						},
					},
				},
			},
		},
	}

	brokers := func(ids ...int32) []v1beta1.Broker {
		out := make([]v1beta1.Broker, 0, len(ids))
		for _, id := range ids {
			out = append(out, v1beta1.Broker{Id: id, BrokerConfigGroup: "broker"})
		}
		return out
	}
	statusState := func(ids ...string) map[string]v1beta1.BrokerState {
		out := map[string]v1beta1.BrokerState{}
		for _, id := range ids {
			out[id] = v1beta1.BrokerState{}
		}
		return out
	}

	t.Run("pure removal returns the deployed config verbatim (no roll)", func(t *testing.T) {
		// Broker 100 dropped from the spec but still in the status: nothing new to add, so the deployed
		// capacity.json must be returned byte-for-byte unchanged.
		kc := &v1beta1.KafkaCluster{
			Spec: v1beta1.KafkaClusterSpec{
				BrokerConfigGroups: brokerConfigGroups,
				Brokers:            brokers(0, 1, 2),
			},
			Status: v1beta1.KafkaClusterStatus{BrokersState: statusState("0", "1", "2", "100")},
		}

		actual, err := GenerateCapacityConfig(kc, logr.Discard(), deployedConfigMap)
		if err != nil {
			t.Fatal(err, "unexpected error")
		}
		if actual != deployedCapacityConfig {
			t.Errorf("expected the deployed capacity.json to be reused verbatim.\nExpected:\n%s\nGot:\n%s", deployedCapacityConfig, actual)
		}
	})

	t.Run("mixed add+remove preserves the departing broker and adds the new one", func(t *testing.T) {
		// Broker 100 removed and broker 101 added in one edit; both are still/already in the status.
		kc := &v1beta1.KafkaCluster{
			Spec: v1beta1.KafkaClusterSpec{
				BrokerConfigGroups: brokerConfigGroups,
				Brokers:            brokers(0, 1, 2, 101),
			},
			Status: v1beta1.KafkaClusterStatus{BrokersState: statusState("0", "1", "2", "100", "101")},
		}

		actual, err := GenerateCapacityConfig(kc, logr.Discard(), deployedConfigMap)
		if err != nil {
			t.Fatal(err, "unexpected error")
		}

		var merged CapacityConfig
		if err := json.Unmarshal([]byte(actual), &merged); err != nil {
			t.Fatal(err, "could not unmarshal merged json")
		}

		byID := map[string]BrokerCapacity{}
		for _, bc := range merged.BrokerCapacities {
			byID[bc.BrokerID] = bc
		}

		gotIDs := make([]string, 0, len(byID))
		for id := range byID {
			gotIDs = append(gotIDs, id)
		}
		sort.Strings(gotIDs)
		if !reflect.DeepEqual(gotIDs, []string{"0", "1", "100", "101", "2"}) {
			t.Errorf("unexpected broker ids in merged config: %v", gotIDs)
		}

		// Departing broker 100 must be preserved verbatim (its unusual deployed values, not the fallback).
		departing := byID["100"]
		if departing.Capacity.CPU != "777" || departing.Capacity.DISK["/kafka-logs-broker/kafka"] != "99999" {
			t.Errorf("departing broker 100 capacity was not preserved verbatim: %+v", departing)
		}

		// Newly added broker 101 must have a generated entry so add_broker has capacity data.
		added, ok := byID["101"]
		if !ok {
			t.Fatal("newly added broker 101 is missing from the merged capacity config")
		}
		if len(added.Capacity.DISK) == 0 {
			t.Errorf("newly added broker 101 has no generated disk capacity: %+v", added)
		}
	})

	for _, state := range []v1beta1.CruiseControlState{v1beta1.GracefulDownscaleRequired, v1beta1.GracefulDownscaleScheduled} {
		state := state
		t.Run(fmt.Sprintf("mixed add+remove with downscale %s still adds the new broker (no deadlock)", state), func(t *testing.T) {
			// Broker 100's downscale has not started executing in Cruise Control yet (Required: no
			// CruiseControlOperation created; Scheduled: one exists but has not been submitted to CC) - there
			// is no in-flight CC-side task for a capacity.json roll to disrupt. Reusing the deployed config
			// verbatim here (as isBrokerDeletionInProgress used to do for every IsDownscale() state) would
			// never write broker 101's capacity, so add_broker's roll gate
			// (CapacityConfigContainsBrokers) would defer it forever; and because the task controller
			// prioritizes add_broker over remove_broker, broker 100's downscale would never advance past this
			// state either - a permanent deadlock. The merge must proceed instead.
			kc := &v1beta1.KafkaCluster{
				Spec: v1beta1.KafkaClusterSpec{
					BrokerConfigGroups: brokerConfigGroups,
					Brokers:            brokers(0, 1, 2, 101),
				},
				Status: v1beta1.KafkaClusterStatus{BrokersState: map[string]v1beta1.BrokerState{
					"0": {}, "1": {}, "2": {},
					"100": {GracefulActionState: v1beta1.GracefulActionState{CruiseControlState: state}},
					"101": {},
				}},
			}

			actual, err := GenerateCapacityConfig(kc, logr.Discard(), deployedConfigMap)
			if err != nil {
				t.Fatal(err, "unexpected error")
			}

			var merged CapacityConfig
			if err := json.Unmarshal([]byte(actual), &merged); err != nil {
				t.Fatal(err, "could not unmarshal merged json")
			}
			byID := map[string]BrokerCapacity{}
			for _, bc := range merged.BrokerCapacities {
				byID[bc.BrokerID] = bc
			}

			added, ok := byID["101"]
			if !ok {
				t.Fatalf("newly added broker 101 is missing from the capacity config while broker 100 is %s - this deadlocks add_broker", state)
			}
			if len(added.Capacity.DISK) == 0 {
				t.Errorf("newly added broker 101 has no generated disk capacity: %+v", added)
			}

			// Departing broker 100 must still be preserved verbatim.
			if got := byID["100"].Capacity.CPU; got != "777" {
				t.Errorf("departing broker 100 was not preserved verbatim: CPU=%q", got)
			}
		})
	}

	t.Run("mixed add+remove keeps the new broker's user-provided capacity (not generated, not dropped)", func(t *testing.T) {
		// The CR provides explicit per-broker capacity (no "-1" universal default) for the newly added
		// broker 101. During the removal-pending window the merge must carry that user-provided entry over
		// verbatim rather than dropping it (and rather than generating a different one).
		kc := &v1beta1.KafkaCluster{
			Spec: v1beta1.KafkaClusterSpec{
				BrokerConfigGroups: brokerConfigGroups,
				Brokers:            brokers(0, 1, 2, 101),
				CruiseControlConfig: v1beta1.CruiseControlConfig{
					CapacityConfig: `{
    "brokerCapacities": [
        {"brokerId": "101", "capacity": {"DISK": {"/kafka-logs-broker/kafka": "424242"}, "CPU": "314", "NW_IN": "271", "NW_OUT": "161"}, "doc": "user-provided-101"}
    ]
}`,
				},
			},
			Status: v1beta1.KafkaClusterStatus{BrokersState: statusState("0", "1", "2", "100", "101")},
		}

		actual, err := GenerateCapacityConfig(kc, logr.Discard(), deployedConfigMap)
		if err != nil {
			t.Fatal(err, "unexpected error")
		}

		var merged CapacityConfig
		if err := json.Unmarshal([]byte(actual), &merged); err != nil {
			t.Fatal(err, "could not unmarshal merged json")
		}
		byID := map[string]BrokerCapacity{}
		for _, bc := range merged.BrokerCapacities {
			byID[bc.BrokerID] = bc
		}

		// Broker 100 (departing) preserved verbatim, broker 101 present with the user-provided values.
		if got := byID["100"].Capacity.CPU; got != "777" {
			t.Errorf("departing broker 100 was not preserved verbatim: CPU=%q", got)
		}
		added, ok := byID["101"]
		if !ok {
			t.Fatal("newly added user-configured broker 101 is missing from the merged capacity config")
		}
		if added.Capacity.CPU != "314" || added.Capacity.DISK["/kafka-logs-broker/kafka"] != "424242" {
			t.Errorf("broker 101 did not keep its user-provided capacity: %+v", added)
		}
	})

	t.Run("unparseable deployed capacity.json is reused verbatim (fail-safe, no roll)", func(t *testing.T) {
		// A corrupt deployed capacity.json must not crash or regenerate from scratch (which would roll CC
		// mid-scaling); mergeCapacityConfig logs and returns it unchanged.
		malformed := `{ this is not valid capacity json`
		cm := &v1.ConfigMap{Data: map[string]string{CapacityConfigMapKey: malformed}}
		kc := &v1beta1.KafkaCluster{
			Spec: v1beta1.KafkaClusterSpec{
				BrokerConfigGroups: brokerConfigGroups,
				Brokers:            brokers(0, 1, 2),
			},
			Status: v1beta1.KafkaClusterStatus{BrokersState: statusState("0", "1", "2", "100")},
		}

		actual, err := GenerateCapacityConfig(kc, logr.Discard(), cm)
		if err != nil {
			t.Fatal(err, "unexpected error")
		}
		if actual != malformed {
			t.Errorf("expected the unparseable capacity.json to be reused verbatim.\nExpected:\n%s\nGot:\n%s", malformed, actual)
		}
	})

	t.Run("active downscale reuses verbatim and does NOT append a concurrently added broker (would roll CC)", func(t *testing.T) {
		// Broker 100 is actively downscaling (a remove_broker task is in flight); broker 103 was added
		// concurrently and is already in the status. Appending 103 would change capacity.json and roll CC,
		// killing the in-flight removal (the #301 failure class), so the deployed config must be reused
		// verbatim - 103's capacity is written by the next full regeneration once the downscale completes.
		kc := &v1beta1.KafkaCluster{
			Spec: v1beta1.KafkaClusterSpec{
				BrokerConfigGroups: brokerConfigGroups,
				Brokers:            brokers(0, 1, 2, 103),
			},
			Status: v1beta1.KafkaClusterStatus{BrokersState: map[string]v1beta1.BrokerState{
				"0": {}, "1": {}, "2": {},
				"100": {GracefulActionState: v1beta1.GracefulActionState{CruiseControlState: v1beta1.GracefulDownscaleRunning}},
				"103": {},
			}},
		}

		actual, err := GenerateCapacityConfig(kc, logr.Discard(), deployedConfigMap)
		if err != nil {
			t.Fatal(err, "unexpected error")
		}
		if actual != deployedCapacityConfig {
			t.Errorf("expected verbatim reuse during an active downscale (no 103 appended).\nExpected:\n%s\nGot:\n%s", deployedCapacityConfig, actual)
		}
	})

	t.Run("deployed config with a -1 universal default is reused verbatim (no redundant append/roll)", func(t *testing.T) {
		// A "-1" default covers every broker, so a newly added broker needs no per-broker entry; appending one
		// would only change capacity.json and roll CC. (Latent - not reachable via the normal flow.)
		deployedWithMinus1 := `{
    "brokerCapacities": [
        {"brokerId": "-1", "capacity": {"DISK": {"/kafka-logs-broker/kafka": "10240"}, "CPU": "100", "NW_IN": "125000", "NW_OUT": "125000"}, "doc": "default"}
    ]
}`
		cm := &v1.ConfigMap{Data: map[string]string{CapacityConfigMapKey: deployedWithMinus1}}
		kc := &v1beta1.KafkaCluster{
			Spec: v1beta1.KafkaClusterSpec{
				BrokerConfigGroups: brokerConfigGroups,
				Brokers:            brokers(0, 1, 2, 103),
			},
			// No downscale in progress, so the merge path runs and must hit the -1 guard.
			Status: v1beta1.KafkaClusterStatus{BrokersState: statusState("0", "1", "2", "103")},
		}

		actual, err := GenerateCapacityConfig(kc, logr.Discard(), cm)
		if err != nil {
			t.Fatal(err, "unexpected error")
		}
		if actual != deployedWithMinus1 {
			t.Errorf("expected verbatim reuse when a -1 universal default is present.\nExpected:\n%s\nGot:\n%s", deployedWithMinus1, actual)
		}
	})
}

func TestCapacityConfigContainsBrokers(t *testing.T) {
	perBroker := `{"brokerCapacities":[
		{"brokerId":"0","capacity":{"DISK":{"/k":"1"},"CPU":"1","NW_IN":"1","NW_OUT":"1"},"doc":"d"},
		{"brokerId":"103","capacity":{"DISK":{"/k":"1"},"CPU":"1","NW_IN":"1","NW_OUT":"1"},"doc":"d"}
	]}`
	universal := `{"brokerCapacities":[
		{"brokerId":"-1","capacity":{"DISK":{"/k":"1"},"CPU":"1","NW_IN":"1","NW_OUT":"1"},"doc":"d"}
	]}`

	tests := []struct {
		name        string
		capacity    string
		brokerIDs   []string
		expected    bool
		expectError bool
	}{
		{name: "all present", capacity: perBroker, brokerIDs: []string{"0", "103"}, expected: true},
		{name: "one missing", capacity: perBroker, brokerIDs: []string{"103", "104"}, expected: false},
		{name: "no brokers requested", capacity: perBroker, brokerIDs: nil, expected: true},
		{name: "universal default covers any broker", capacity: universal, brokerIDs: []string{"999"}, expected: true},
		{name: "invalid json", capacity: "{not json", brokerIDs: []string{"0"}, expectError: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := CapacityConfigContainsBrokers(test.capacity, test.brokerIDs)
			if test.expectError {
				if err == nil {
					t.Fatal("expected an error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatal(err, "unexpected error")
			}
			if got != test.expected {
				t.Errorf("expected %v, got %v", test.expected, got)
			}
		})
	}
}

func TestCapacityConfigHash(t *testing.T) {
	// Deterministic and sensitive to content; must equal what GeneratePodAnnotations stamps into the pod
	// template for the same capacity.json.
	a := CapacityConfigHash(`{"brokerCapacities":[]}`)
	if CapacityConfigHash(`{"brokerCapacities":[]}`) != a {
		t.Error("hash is not deterministic")
	}
	if CapacityConfigHash(`{"brokerCapacities":[{"brokerId":"0"}]}`) == a {
		t.Error("hash did not change for different content")
	}

	capacityJSON := `{"brokerCapacities":[{"brokerId":"0"}]}`
	viaAnnotations := GeneratePodAnnotations(nil, map[string]string{CapacityConfigMapKey: capacityJSON})
	if viaAnnotations[CapacityConfigHashAnnotationKey] != CapacityConfigHash(capacityJSON) {
		t.Errorf("CapacityConfigHash (%s) does not match GeneratePodAnnotations (%s)",
			CapacityConfigHash(capacityJSON), viaAnnotations[CapacityConfigHashAnnotationKey])
	}
}
