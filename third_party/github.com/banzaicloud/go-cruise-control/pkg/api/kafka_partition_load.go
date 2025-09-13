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

package api

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"regexp"

	"github.com/pkg/errors"

	"github.com/banzaicloud/go-cruise-control/pkg/types"
)

const (
	EndpointKafkaPartitionLoad types.APIEndpoint = "PARTITION_LOAD"

	partitionRegexPattern = "^(([0-9]+)|([0-9]+-[0-9]+))$"
)

var partitionRegex = regexp.MustCompile(partitionRegexPattern)

type KafkaPartitionLoadRequest struct {
	types.GenericRequestWithReason

	// The timestamp in millisecond of the earliest metric sample used to generate load.
	Start int64 `param:"start,omitempty"`
	// The timestamp in millisecond of the latest metric sample used to generate load, current time will be used
	// if this parameter is not specified.
	End int64 `param:"end,omitempty"`
	// Whether to allow capacity estimation when cruise-control is unable to obtain all per-broker capacity information.
	AllowCapacityEstimation bool `param:"allow_capacity_estimation"`
	// The host and broker-level resource by which to sort the cruise-control response.
	SortByResource types.ResourceType `param:"resource,omitempty"`
	// The number of entries to show in the response.
	Entries int32 `param:"entries,omitempty"`
	// A regular expression used to filter the partition load returned based on topic.
	// Example: "myTopic.*"
	Topic string `param:"topic,omitempty"`
	// A single partition or partition range to filter partition load returned.
	// Example: "0" or "0-9"
	Partition string `param:"partition,omitempty"`
	// The minimum required ratio of partition load data completeness. The value must be in range of 0.0 - 1.0.
	// The default value is 0.98.
	MinValidPartitionRatio float64 `param:"min_valid_partition_ratio,omitempty"`
	// If true, the maximum load is returned.
	MaxLoad bool `param:"max_load,omitempty"`
	// If true, the average load is returned.
	AvgLoad bool `param:"avg_load,omitempty"`
	// Set of broker ids used to filter partition load returned.
	BrokerID []int32 `param:"brokerid,omitempty"`
}

func (s KafkaPartitionLoadRequest) Validate() error {
	if s.Start < 1 {
		return errors.New("timestamp set as Start for Kafka cluster load must be bigger then 0")
	}

	if s.End < 1 {
		return errors.New("timestamp set as End for Kafka cluster load must be bigger then 0")
	}

	if s.End < 1 {
		return errors.New("timestamp set as End for Kafka cluster load must be bigger then 0")
	}

	if s.Partition != "" && !partitionRegex.MatchString(s.Partition) {
		return errors.New("Partition parameter must define a single (0) or a range (0-9) of partitions")
	}

	if s.MinValidPartitionRatio < 0 || s.MinValidPartitionRatio > 1 {
		return errors.New("MinValidPartitionRatio parameter must be in range of 0.0 - 1.0")
	}

	return nil
}

func KafkaPartitionLoadRequestWithDefaults() *KafkaPartitionLoadRequest {
	return &KafkaPartitionLoadRequest{
		AllowCapacityEstimation: true,
		SortByResource:          types.ResourceTypeDisk,
		Entries:                 math.MaxInt32,
	}
}

type KafkaPartitionLoadResponse struct {
	types.GenericResponse

	Result *types.PartitionLoadState
}

func (r *KafkaPartitionLoadResponse) UnmarshalResponse(resp *http.Response) error {
	if err := r.GenericResponse.UnmarshalResponse(resp); err != nil {
		return fmt.Errorf("failed to parse HTTP response metadata: %w", err)
	}

	var bodyBytes []byte
	var err error

	bodyBytes, err = io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read HTTP response body: %w", err)
	}

	var d interface{}
	switch resp.StatusCode {
	case http.StatusOK:
		r.Result = &types.PartitionLoadState{}
		d = r.Result
	case http.StatusAccepted:
		r.Progress = &types.ProgressResult{}
		d = r.Progress
	default:
		r.Error = &types.APIError{}
		d = r.Error
	}

	if err = json.Unmarshal(bodyBytes, d); err != nil {
		return fmt.Errorf("failed to parse JSON response: %w", err)
	}

	return nil
}
