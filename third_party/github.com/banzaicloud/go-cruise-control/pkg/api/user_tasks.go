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

	"github.com/banzaicloud/go-cruise-control/pkg/types"
)

const (
	EndpointUserTasks types.APIEndpoint = "USER_TASKS"
)

type UserTasksRequest struct {
	types.GenericRequestWithReason

	// List of IP addresses to filter the task results Cruise Control report.
	ClientIDs []string `param:"client_ids,omitempty"`
	// List of endpoints to filter the task results Cruise Control report.
	Endpoints []types.APIEndpoint `param:"endpoints,omitempty"`
	// The number of entries to show in the response.
	Entries int32 `param:"entries,omitempty"`
	// List of user task status to filter the task results Cruise Control report.
	Types []types.UserTaskStatus `param:"types,omitempty"`
	// List of user task UUIDs to filter the task results Cruise Control report.
	UserTaskIDs []string `param:"user_task_ids,omitempty"`
	// Whether return the original request's final response.
	FetchCompletedTasks bool `param:"fetch_completed_task,omitempty"`
}

func (s UserTasksRequest) Validate() error {
	return nil
}

func UserTasksRequestWithDefaults() *UserTasksRequest {
	return &UserTasksRequest{
		Entries: math.MaxInt32,
	}
}

type UserTasksResponse struct {
	types.GenericResponse

	Result *types.UserTaskState
}

func (r *UserTasksResponse) UnmarshalResponse(resp *http.Response) error {
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
		r.Result = &types.UserTaskState{}
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
