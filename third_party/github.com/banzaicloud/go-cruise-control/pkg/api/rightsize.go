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

	"github.com/banzaicloud/go-cruise-control/pkg/types"
)

const (
	EndpointRightsize types.APIEndpoint = "RIGHTSIZE"
)

type RightsizeRequest struct {
	types.GenericRequestWithReason

	// NumberOfBrokersToAdd is the difference in broker count to rightsize towards. Minimum value is 1.
	NumberOfBrokersToAdd int32 `param:"num_brokers_to_add,omitempty"`
	// PartitionCount is the target number of partitions to rightsize towards. Minimum value is 1.
	PartitionCount int32 `param:"partition_count,omitempty"`
	// Topic is a regular expression to specify subject topics.
	Topic string `param:"topic,omitempty"`
}

func (s RightsizeRequest) Validate() error {
	return nil
}

func RightsizeRequestWithDefaults() *RightsizeRequest {
	return &RightsizeRequest{}
}

type RightsizeResponse struct {
	types.GenericResponse

	Result *types.RightsizeResult
}

func (r *RightsizeResponse) UnmarshalResponse(resp *http.Response) error {
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
		r.Result = &types.RightsizeResult{}
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
