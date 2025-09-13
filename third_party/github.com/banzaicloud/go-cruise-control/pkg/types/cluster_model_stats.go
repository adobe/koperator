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

type ClusterModelStats struct {
	Metadata   ClusterModelMetadata   `json:"metadata"`
	Statistics ClusterModelStatistics `json:"statistics"`
}

type ClusterModelMetadata struct {
	Replicas int32 `json:"replicas"`
	Topics   int32 `json:"topics"`
	Brokers  int32 `json:"brokers"`
}

type ClusterModelStatistics struct {
	Avg ClusterModelStatisticsData `json:"AVG"`
	Std ClusterModelStatisticsData `json:"STD"`
	Min ClusterModelStatisticsData `json:"MIN"`
	Max ClusterModelStatisticsData `json:"MAX"`
}

type ClusterModelStatisticsData struct {
	Disk           float64 `json:"disk"`
	Replicas       float32 `json:"replicas"`
	LeaderReplicas float32 `json:"leaderReplicas"`
	CPU            float64 `json:"cpu"`
	NetworkOut     float64 `json:"networkOutbound"`
	NetworkIn      float64 `json:"networkInbound"`
	TopicReplicas  float32 `json:"topicReplicas"`
}
