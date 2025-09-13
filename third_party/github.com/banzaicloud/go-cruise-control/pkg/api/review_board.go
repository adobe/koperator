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
	EndpointReviewBoard types.APIEndpoint = "REVIEW_BOARD"
)

type ReviewBoardRequest struct {
	types.GenericRequest

	// Approve one or more pending Cruise Control requests.
	ReviewIDs []int32 `param:"review_ids,omitempty"`
}

func (s ReviewBoardRequest) Validate() error {
	return nil
}

func ReviewBoardRequestWithDefaults() *ReviewRequest {
	return &ReviewRequest{}
}

type ReviewBoardResponse struct {
	types.GenericResponse

	Result *types.ReviewResult
}

func (r *ReviewBoardResponse) UnmarshalResponse(resp *http.Response) error {
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
		r.Result = &types.ReviewResult{}
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
