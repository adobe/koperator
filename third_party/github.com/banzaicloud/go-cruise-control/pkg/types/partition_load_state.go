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

type PartitionLoadState struct {
	Version

	Records []PartitionLoad `json:"records"`
}

type PartitionLoad struct {
	Topic      string  `json:"topic"`
	Partition  int32   `json:"partition"`
	Leader     int32   `json:"leader"`
	Followers  []int32 `json:"followers"`
	CPU        float64 `json:"cpu"`
	NetworkIn  float64 `json:"networkInbound"`
	NetworkOut float64 `json:"networkOutbound"`
	Disk       float64 `json:"disk"`
	MessageIn  float64 `json:"msg_in"`
}
