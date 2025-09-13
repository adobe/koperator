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
	"net/http"

	"github.com/pkg/errors"

	"github.com/banzaicloud/go-cruise-control/pkg/types"
)

const (
	EndpointKafkaClusterLoad types.APIEndpoint = "LOAD"
)

type KafkaClusterLoadRequest struct {
	types.GenericRequestWithReason

	// Start time of the cluster load. Default is time of the earliest valid window.
	Start int64 `param:"start,omitempty"`
	// End time of the cluster load. Default is current system time.
	End int64 `param:"end,omitempty"`
	// End time of the cluster load. Default is current system time, mutually exclusive with End parameter.
	Time int64 `param:"time,omitempty"`
	// Whether to allow capacity estimation when cruise-control is unable to obtain all per-broker capacity information.
	AllowCapacityEstimation bool `param:"allow_capacity_estimation"`
	// Whether show the load of each disk of broker.
	PopulateDiskInfo bool `param:"populate_disk_info,omitempty"`
	// Whether show only the cluster capacity or the utilization, as well.
	CapacityOnly bool `param:"capacity_only,omitempty"`
}

func (s KafkaClusterLoadRequest) Validate() error {
	if s.Start < 1 {
		return errors.New("timestamp set as Start for Kafka cluster load must be bigger then 0")
	}

	if s.End < 1 {
		return errors.New("timestamp set as End for Kafka cluster load must be bigger then 0")
	}

	if s.Time < 1 {
		return errors.New("timestamp set as Time for Kafka cluster load must be bigger then 0")
	}

	if s.End > 1 && s.Time > 1 {
		return errors.New("End and Time parameters for Kafka cluster load are mutually exclusive")
	}

	return nil
}

func KafkaClusterLoadRequestWithDefaults() *KafkaClusterLoadRequest {
	return &KafkaClusterLoadRequest{
		AllowCapacityEstimation: true,
	}
}

type KafkaClusterLoadResponse struct {
	types.GenericResponse

	Result *types.BrokerStats
}

func (r *KafkaClusterLoadResponse) UnmarshalResponse(resp *http.Response) error {
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
		r.Result = &types.BrokerStats{}
		d = r.Result
	default:
		r.Error = &types.APIError{}
		d = r.Error
	}

	if err = json.Unmarshal(bodyBytes, d); err != nil {
		return fmt.Errorf("failed to parse JSON response: %w", err)
	}

	return nil
}
