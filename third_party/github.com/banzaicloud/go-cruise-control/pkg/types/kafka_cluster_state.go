/*
Copyright © 2021 Cisco and/or its affiliates. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package types

type KafkaClusterState struct {
	Version

	KafkaBrokerState    KafkaBrokerState    `json:"KafkaBrokerState"`
	KafkaPartitionState KafkaPartitionState `json:"KafkaPartitionState"`
}

type KafkaBrokerState struct {
	LeaderCountByBrokerID         map[string]int32    `json:"LeaderCountByBrokerId"`
	OutOfSyncCountByBrokerID      map[string]int32    `json:"OutOfSyncCountByBrokerId"`
	ReplicaCountByBrokerID        map[string]int32    `json:"ReplicaCountByBrokerId"`
	OfflineReplicaCountByBrokerID map[string]int32    `json:"OfflineReplicaCountByBrokerId"`
	IsController                  map[string]bool     `json:"IsController"`
	OnlineLogDirsByBrokerID       map[string][]string `json:"OnlineLogDirsByBrokerId"`
	OfflineLogDirsByBrokerID      map[string][]string `json:"OfflineLogDirsByBrokerId"`
	Summary                       KafkaClusterStats   `json:"Summary"`
	BrokerSetByBrokerID           map[string]string   `json:"BrokerSetByBrokerId"`
}

type KafkaClusterStats struct {
	Brokers  int32 `json:"Brokers"`
	Topics   int32 `json:"Topics"`
	Replicas int32 `json:"Replicas"`
	Leaders  int32 `json:"Leaders"`

	AvgReplicationFactor float64 `json:"AvgReplicationFactor"`
	AvgReplicasPerBroker float64 `json:"AvgReplicasPerBroker"`
	AvgLeadersPerBroker  float64 `json:"AvgLeadersPerBroker"`
	MaxReplicasPerBroker float64 `json:"MaxReplicasPerBroker"`
	MaxLeadersPerBroker  float64 `json:"MaxLeadersPerBroker"`
	StdReplicasPerBroker float64 `json:"StdReplicasPerBroker"`
	StdLeadersPerBroker  float64 `json:"StdLeadersPerBroker"`
}

type KafkaPartitionState struct {
	Offline                   []PartitionState `json:"offline"`
	WithOfflineReplicas       []PartitionState `json:"with-offline-replicas"`
	UnderReplicatedPartitions []PartitionState `json:"urp"`
	UnderMinISR               []PartitionState `json:"under-min-isr"`
	Other                     []PartitionState `json:"other"`
}

type PartitionState struct {
	Topic                string  `json:"topic"`
	Partition            int32   `json:"partition"`
	Leader               int32   `json:"leader"`
	Replicas             []int32 `json:"replicas"`
	InSyncReplicas       []int32 `json:"in-sync"`
	OutOfSyncReplicas    []int32 `json:"out-of-sync"`
	OfflineReplicas      []int32 `json:"offline"`
	MinISRReplicas       int32   `json:"min-isr"`
	RemoteStorageEnabled bool    `json:"remote-storage-enabled"`
}
