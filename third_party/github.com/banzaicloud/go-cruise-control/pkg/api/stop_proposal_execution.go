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
	EndpointStopProposalExecution types.APIEndpoint = "STOP_PROPOSAL_EXECUTION"
)

type StopProposalExecutionRequest struct {
	types.GenericRequestWithReason

	// If ForceStop is set to true then it stops execution forcefully by deleting the /admin/partition_reassignemt,
	// /preferred_replica_election and /controller zNodes in Zookeeper. Default is false.
	ForceStop bool `param:"force_stop,omitempty"`
	// Review id for 2-step verification.
	ReviewID int32 `param:"review_id,omitempty"`
	// If StopExternalAgent is set to true then it stops any ongoing execution even if it is started by an external agent.
	// If false, only stop execution started by the current CC instance.
	// This parameter would only be honored with Kafka 2.4 or above.
	StopExternalAgent bool `param:"stop_external_agent,omitempty"`
}

func (s StopProposalExecutionRequest) Validate() error {
	return nil
}

func StopProposalExecutionRequestWithDefaults() *StopProposalExecutionRequest {
	return &StopProposalExecutionRequest{}
}

type StopProposalExecutionResponse struct {
	types.GenericResponse

	Result *types.StopProposalResult
}

func (r *StopProposalExecutionResponse) UnmarshalResponse(resp *http.Response) error {
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
		r.Result = &types.StopProposalResult{}
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
