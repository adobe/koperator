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

type ExecutionTask struct {
	ExecutionID int64 `json:"executionId"`

	Type  ExecutionTaskType  `json:"type"`
	State ExecutionTaskState `json:"state"`

	Proposal ExecutionProposal `json:"proposal"`
	BrokerID int32             `json:"brokerId"`
}

const (
	ExecutionTaskTypeUndefined ExecutionTaskType = iota
	ExecutionTaskTypeInterBrokerReplicaAction
	ExecutionTaskTypeIntraBrokerReplicaAction
	ExecutionTaskTypeLeaderAction
)

type ExecutionTaskType int8

func (t ExecutionTaskType) String() string {
	switch t {
	case ExecutionTaskTypeInterBrokerReplicaAction:
		return "INTER_BROKER_REPLICA_ACTION"
	case ExecutionTaskTypeIntraBrokerReplicaAction:
		return "INTRA_BROKER_REPLICA_ACTION"
	case ExecutionTaskTypeLeaderAction:
		return "LEADER_ACTION"
	case ExecutionTaskTypeUndefined:
		fallthrough
	default:
		return Undefined
	}
}

func (t *ExecutionTaskType) MarshalJSON() ([]byte, error) {
	return []byte(addQuotes(t.String())), nil
}

func (t *ExecutionTaskType) UnmarshalJSON(data []byte) error {
	switch removeQuotes(string(data)) {
	case ExecutionTaskTypeInterBrokerReplicaAction.String():
		*t = ExecutionTaskTypeInterBrokerReplicaAction
	case ExecutionTaskTypeIntraBrokerReplicaAction.String():
		*t = ExecutionTaskTypeIntraBrokerReplicaAction
	case ExecutionTaskTypeLeaderAction.String():
		*t = ExecutionTaskTypeLeaderAction
	case ExecutionTaskTypeUndefined.String():
		fallthrough
	default:
		*t = ExecutionTaskTypeUndefined
	}
	return nil
}

func (t *ExecutionTaskType) UnmarshalText(data []byte) error {
	return t.UnmarshalJSON(data)
}

const (
	ExecutionTaskStateUndefined ExecutionTaskState = iota
	ExecutionTaskStatePending
	ExecutionTaskStateInProgress
	ExecutionTaskStateAborting
	ExecutionTaskStateAborted
	ExecutionTaskStateDead
	ExecutionTaskStateCompleted
)

type ExecutionTaskState int8

func (s ExecutionTaskState) String() string {
	switch s {
	case ExecutionTaskStatePending:
		return "PENDING"
	case ExecutionTaskStateInProgress:
		return "IN_PROGRESS"
	case ExecutionTaskStateAborting:
		return "ABORTING"
	case ExecutionTaskStateAborted:
		return "ABORTED"
	case ExecutionTaskStateDead:
		return "DEAD"
	case ExecutionTaskStateCompleted:
		return "COMPLETED"
	case ExecutionTaskStateUndefined:
		fallthrough
	default:
		return Undefined
	}
}

func (s *ExecutionTaskState) MarshalJSON() ([]byte, error) {
	return []byte(s.String()), nil
}

func (s *ExecutionTaskState) UnmarshalJSON(data []byte) error {
	switch removeQuotes(string(data)) {
	case ExecutionTaskStatePending.String():
		*s = ExecutionTaskStatePending
	case ExecutionTaskStateInProgress.String():
		*s = ExecutionTaskStateInProgress
	case ExecutionTaskStateAborting.String():
		*s = ExecutionTaskStateAborting
	case ExecutionTaskStateAborted.String():
		*s = ExecutionTaskStateAborted
	case ExecutionTaskStateDead.String():
		*s = ExecutionTaskStateDead
	case ExecutionTaskStateCompleted.String():
		*s = ExecutionTaskStateCompleted
	case ExecutionTaskStateUndefined.String():
		fallthrough
	default:
		*s = ExecutionTaskStateUndefined
	}
	return nil
}

func (s *ExecutionTaskState) UnmarshalText(data []byte) error {
	return s.UnmarshalJSON(data)
}
