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

type ReviewResult struct {
	Version

	RequestInfo RequestInfo `json:"RequestInfo"`
}

type RequestInfo struct {
	ID                 int32  `json:"Id"`
	SubmitterAddress   string `json:"SubmitterAddress"`
	SubmissionTimeMs   int64  `json:"SubmissionTimeMs"`
	EndpointWithParams string `json:"EndpointWithParams"`
	Reason             string `json:"reason"`

	Status RequestStatus `json:"Status"`
}

const (
	RequestStatusUndefined RequestStatus = iota
	RequestStatusPendingReview
	RequestStatusApproved
	RequestStatusSubmitted
	RequestStatusDiscarded
)

type RequestStatus int8

func (s RequestStatus) String() string {
	switch s {
	case RequestStatusPendingReview:
		return "PENDING_REVIEW"
	case RequestStatusApproved:
		return "APPROVED"
	case RequestStatusSubmitted:
		return "SUBMITTED"
	case RequestStatusDiscarded:
		return "DISCARDED"
	case RequestStatusUndefined:
		fallthrough
	default:
		return Undefined
	}
}

func (s *RequestStatus) MarshalJSON() ([]byte, error) {
	return []byte(addQuotes(s.String())), nil
}

func (s *RequestStatus) UnmarshalJSON(data []byte) error {
	switch removeQuotes(string(data)) {
	case RequestStatusPendingReview.String():
		*s = RequestStatusPendingReview
	case RequestStatusApproved.String():
		*s = RequestStatusApproved
	case RequestStatusSubmitted.String():
		*s = RequestStatusSubmitted
	case RequestStatusDiscarded.String():
		*s = RequestStatusDiscarded
	case ResourceTypeUndefined.String():
		fallthrough
	default:
		*s = RequestStatusUndefined
	}

	return nil
}

func (s *RequestStatus) UnmarshalText(data []byte) error {
	return s.UnmarshalJSON(data)
}
