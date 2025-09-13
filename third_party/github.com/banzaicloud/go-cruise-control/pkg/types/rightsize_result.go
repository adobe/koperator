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

type RightsizeResult struct {
	Version

	NumberOfBrokersToAdd int32  `json:"numBrokersToAdd"`
	PartitionCount       int32  `json:"partitionCount"`
	Topic                string `json:"topic"`

	ProvisionerState ProvisionerState `json:"provisionerState"`
}

const (
	ProvisionerStateStatusUndefined ProvisionerState = iota
	ProvisionerStateCompleted
	ProvisionerStateCompletedWithError
	ProvisionerStateInProgress
)

type ProvisionerState int8

func (s ProvisionerState) String() string {
	switch s {
	case ProvisionerStateCompleted:
		return "COMPLETED"
	case ProvisionerStateCompletedWithError:
		return "COMPLETED_WITH_ERROR"
	case ProvisionerStateInProgress:
		return "IN_PROGRESS"
	case ProvisionerStateStatusUndefined:
		fallthrough
	default:
		return Undefined
	}
}

func (s *ProvisionerState) MarshalJSON() ([]byte, error) {
	return []byte(addQuotes(s.String())), nil
}

func (s *ProvisionerState) UnmarshalJSON(data []byte) error {
	switch removeQuotes(string(data)) {
	case ProvisionerStateCompleted.String():
		*s = ProvisionerStateCompleted
	case ProvisionerStateCompletedWithError.String():
		*s = ProvisionerStateCompletedWithError
	case ProvisionerStateInProgress.String():
		*s = ProvisionerStateInProgress
	case GoalReadinessStatusUndefined.String():
		fallthrough
	default:
		*s = ProvisionerStateStatusUndefined
	}
	return nil
}

func (s *ProvisionerState) UnmarshalText(data []byte) error {
	return s.UnmarshalJSON(data)
}
