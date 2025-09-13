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
	EndpointBootstrap types.APIEndpoint = "BOOTSTRAP"
)

type BootstrapRequest struct {
	types.GenericRequest

	// Whether to clear the collected metric samples during bootstrap.
	ClearMetrics bool `param:"clearmetrics,omitempty"`
	// Whether to run the request in developer mode.
	DeveloperMode bool `param:"developer_mode,omitempty"`
	// Timestamp in millisecond of the latest metrics sample to load during bootstrap, current time will be used
	// if this parameter is not specified.
	End int64 `param:"end,omitempty"`
	// Timestamp in millisecond of the earliest metrics sample to load during bootstrap.
	Start int64 `param:"start"`
}

func (s BootstrapRequest) Validate() error {
	if s.Start < 1 {
		return errors.New("timestamp for bootstrap start must be bigger then 0")
	}

	if s.End < 1 {
		return errors.New("timestamp for bootstrap end must be bigger then 0")
	}

	return nil
}

func BootstrapRequestWithDefaults() *BootstrapRequest {
	return &BootstrapRequest{
		DeveloperMode: true,
	}
}

type BootstrapResponse struct {
	types.GenericResponse

	Result *types.BootstrapResult
}

func (r *BootstrapResponse) UnmarshalResponse(resp *http.Response) error {
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
		r.Result = &types.BootstrapResult{}
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
