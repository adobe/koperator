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

import (
	"fmt"
	"net/http"
	"strings"
)

const (
	UserTaskIDHTTPHeader           = "User-Task-ID"
	CruiseControlVersionHTTPHeader = "Cruise-Control-Version"
	DateHTTPHeader                 = "Date"

	Undefined = "UNDEFINED"
)

type APIRequest interface {
	Validate() error
}

type GenericRequest struct {
	// Whether to return the response in JSON format or not
	JSON bool `param:"json,omitempty"`
	// Whether to return JSON schema in response header or not
	GetResponseSchema bool `param:"get_response_schema,omitempty"`
	// The user specified by a trusted proxy in that authentication model
	DoAs string `param:"doAs,omitempty"`
}

// RequestReasoner is a dummy interface to help identifying APIRequests
// which have the `Reason` field without the need for using reflection
type RequestReasoner interface {
	requestReason() string
}

type GenericRequestWithReason struct {
	GenericRequest

	// Reason for request
	Reason string `param:"reason,omitempty"`
}

func (r GenericRequestWithReason) requestReason() string {
	return r.Reason
}

type APIResponse interface {
	UnmarshalResponse(*http.Response) error
	InProgress() bool
	Failed() bool
	Err() error
}

type GenericResponse struct {
	TaskID               string
	CruiseControlVersion string
	Date                 string
	StatusCode           int
	RequestURL           string
	Progress             *ProgressResult
	Error                *APIError
}

func (r *GenericResponse) UnmarshalResponse(resp *http.Response) error {
	r.TaskID = resp.Header.Get(UserTaskIDHTTPHeader)
	r.CruiseControlVersion = resp.Header.Get(CruiseControlVersionHTTPHeader)
	r.Date = resp.Header.Get(DateHTTPHeader)
	r.StatusCode = resp.StatusCode
	r.RequestURL = resp.Request.URL.String()
	return nil
}

func (r *GenericResponse) InProgress() bool {
	return r.Progress != nil
}

func (r *GenericResponse) Failed() bool {
	return r.Error != nil
}

func (r *GenericResponse) Err() error {
	return r.Error
}

type APIError struct {
	// Cruise Control error
	StackTrace   string `json:"stackTrace,omitempty"`
	ErrorMessage string `json:"errorMessage,omitempty"`
	// Servlet error
	Message string `json:"message,omitempty"`
	Servlet string `json:"servlet,omitempty"`
	URL     string `json:"url,omitempty"`
	Status  string `json:"status,omitempty"`
}

func (r APIError) Error() string {
	if r.Message != "" {
		return fmt.Sprintf("%s - %s (url: %s, servlet: %s)", r.Status, r.Message, r.URL, r.Servlet)
	}
	return r.ErrorMessage
}

func addQuotes(s string) string {
	return fmt.Sprintf("\"%s\"", s)
}

func removeQuotes(s string) string {
	return strings.Trim(s, "\"")
}

type Version struct {
	Version int32 `json:"version"`
}

type APIEndpoint string

func (e APIEndpoint) String() string {
	return string(e)
}

func (e APIEndpoint) Path() string {
	return strings.ToLower(e.String())
}

func (e APIEndpoint) MarshalJSON() ([]byte, error) {
	return []byte(strings.ToUpper(e.String())), nil
}
