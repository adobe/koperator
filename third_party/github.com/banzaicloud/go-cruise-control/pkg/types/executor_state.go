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

type ExecutorState struct {
	TriggeredUserTaskID string `json:"triggeredUserTaskId,omitempty"`
	TriggeredTaskReason string `json:"triggeredTaskReason,omitempty"`

	TriggeredSelfHealingTaskID string `json:"triggeredSelfHealingTaskId,omitempty"`

	State ExecutorStateType `json:"state"`

	RecentlyDemotedBrokers []int32 `json:"recentlyDemotedBrokers,omitempty"`
	RecentlyRemovedBrokers []int32 `json:"recentlyRemovedBrokers,omitempty"`

	NumTotalLeadershipMovements     int32 `json:"numTotalLeadershipMovements"`
	NumCancelledLeadershipMovements int32 `json:"numCancelledLeadershipMovements"`
	NumPendingLeadershipMovements   int32 `json:"numPendingLeadershipMovements"`
	NumFinishedLeadershipMovements  int32 `json:"numFinishedLeadershipMovements"`

	MaximumConcurrentLeaderMovements int32   `json:"maximumConcurrentLeaderMovements,omitempty"`
	MinimumConcurrentLeaderMovements int32   `json:"minimumConcurrentLeaderMovements,omitempty"`
	AverageConcurrentLeaderMovements float64 `json:"averageConcurrentLeaderMovements,omitempty"`
	NumTotalPartitionMovements       int32   `json:"numTotalPartitionMovements,omitempty"`
	NumPendingPartitionMovements     int32   `json:"numPendingPartitionMovements,omitempty"`
	NumCancelledPartitionMovements   int32   `json:"numCancelledPartitionMovements,omitempty"`
	NumInProgressPartitionMovements  int32   `json:"numInProgressPartitionMovements,omitempty"`

	AbortingPartitions            int32 `json:"abortingPartitions,omitempty"`
	NumFinishedPartitionMovements int32 `json:"numFinishedPartitionMovements,omitempty"`

	CancelledLeadershipMovement []ExecutionTask `json:"cancelledLeadershipMovement,omitempty"`
	InProgressPartitionMovement []ExecutionTask `json:"inProgressPartitionMovement,omitempty"`
	PendingPartitionMovement    []ExecutionTask `json:"pendingPartitionMovement,omitempty"`
	CancelledPartitionMovement  []ExecutionTask `json:"cancelledPartitionMovement,omitempty"`
	DeadPartitionMovement       []ExecutionTask `json:"deadPartitionMovement,omitempty"`
	CompletedPartitionMovement  []ExecutionTask `json:"completedPartitionMovement,omitempty"`
	AbortingPartitionMovement   []ExecutionTask `json:"abortingPartitionMovement,omitempty"`
	AbortedPartitionMovement    []ExecutionTask `json:"abortedPartitionMovement,omitempty"`

	FinishedDataMovement int64 `json:"finishedDataMovement,omitempty"`
	TotalDataToMove      int64 `json:"totalDataToMove,omitempty"`

	MaximumConcurrentPartitionMovementsPerBroker int32   `json:"maximumConcurrentPartitionMovementsPerBroker,omitempty"`
	MinimumConcurrentPartitionMovementsPerBroker int32   `json:"minimumConcurrentPartitionMovementsPerBroker,omitempty"`
	AverageConcurrentPartitionMovementsPerBroker float64 `json:"averageConcurrentPartitionMovementsPerBroker,omitempty"`
	NumTotalIntraBrokerPartitionMovements        int32   `json:"numTotalIntraBrokerPartitionMovements,omitempty"`
	NumFinishedIntraBrokerPartitionMovements     int32   `json:"numFinishedIntraBrokerPartitionMovements,omitempty"`
	NumInProgressIntraBrokerPartitionMovements   int32   `json:"numInProgressIntraBrokerPartitionMovements,omitempty"`
	NumAbortingIntraBrokerPartitionMovements     int32   `json:"numAbortingIntraBrokerPartitionMovements,omitempty"`
	NumPendingIntraBrokerPartitionMovements      int32   `json:"numPendingIntraBrokerPartitionMovements,omitempty"`
	NumCancelledIntraBrokerPartitionMovements    int32   `json:"numCancelledIntraBrokerPartitionMovements,omitempty"`

	InProgressIntraBrokerPartitionMovement []ExecutionTask `json:"inProgressIntraBrokerPartitionMovement,omitempty"`
	PendingIntraBrokerPartitionMovement    []ExecutionTask `json:"pendingIntraBrokerPartitionMovement,omitempty"`
	CancelledIntraBrokerPartitionMovement  []ExecutionTask `json:"cancelledIntraBrokerPartitionMovement,omitempty"`
	DeadIntraBrokerPartitionMovement       []ExecutionTask `json:"deadIntraBrokerPartitionMovement,omitempty"`
	CompletedIntraBrokerPartitionMovement  []ExecutionTask `json:"completedIntraBrokerPartitionMovement,omitempty"`
	AbortingIntraBrokerPartitionMovement   []ExecutionTask `json:"abortingIntraBrokerPartitionMovement,omitempty"`
	AbortedIntraBrokerPartitionMovement    []ExecutionTask `json:"abortedIntraBrokerPartitionMovement,omitempty"`

	FinishedIntraBrokerDataMovement int64 `json:"finishedIntraBrokerDataMovement,omitempty"`
	TotalIntraBrokerDataToMove      int64 `json:"totalIntraBrokerDataToMove,omitempty"`

	MaximumConcurrentIntraBrokerPartitionMovementsPerBroker int32   `json:"maximumConcurrentIntraBrokerPartitionMovementsPerBroker,omitempty"` //nolint:lll
	MinimumConcurrentIntraBrokerPartitionMovementsPerBroker int32   `json:"minimumConcurrentIntraBrokerPartitionMovementsPerBroker,omitempty"` //nolint:lll
	AverageConcurrentIntraBrokerPartitionMovementsPerBroker float64 `json:"averageConcurrentIntraBrokerPartitionMovementsPerBroker,omitempty"` //nolint:lll

	Error string `json:"error,omitempty"`
}

const (
	ExecutorStateTypeUndefined ExecutorStateType = iota
	ExecutorStateTypeNoTaskInProgress
	ExecutorStateTypeStartingExecution
	ExecutorStateTypeInterBrokerReplicaMovementTaskInProgress
	ExecutorStateTypeIntraBrokerReplicaMovementTaskInProgress
	ExecutorStateTypeLeaderMovementTaskInProgress
	ExecutorStateTypeStoppingExecution
	ExecutorStateTypeInitializingProposalExecution
	ExecutorStateTypeGeneratingProposalsForExecution
)

type ExecutorStateType int8

func (t ExecutorStateType) String() string {
	switch t {
	case ExecutorStateTypeNoTaskInProgress:
		return "NO_TASK_IN_PROGRESS"
	case ExecutorStateTypeStartingExecution:
		return "STARTING_EXECUTION"
	case ExecutorStateTypeInterBrokerReplicaMovementTaskInProgress:
		return "INTER_BROKER_REPLICA_MOVEMENT_TASK_IN_PROGRESS"
	case ExecutorStateTypeIntraBrokerReplicaMovementTaskInProgress:
		return "INTRA_BROKER_REPLICA_MOVEMENT_TASK_IN_PROGRESS"
	case ExecutorStateTypeLeaderMovementTaskInProgress:
		return "LEADER_MOVEMENT_TASK_IN_PROGRESS"
	case ExecutorStateTypeStoppingExecution:
		return "STOPPING_EXECUTION"
	case ExecutorStateTypeInitializingProposalExecution:
		return "INITIALIZING_PROPOSAL_EXECUTION"
	case ExecutorStateTypeGeneratingProposalsForExecution:
		return "GENERATING_PROPOSALS_FOR_EXECUTION"
	case ExecutorStateTypeUndefined:
		fallthrough
	default:
		return Undefined
	}
}

func (t *ExecutorStateType) MarshalJSON() ([]byte, error) {
	return []byte(addQuotes(t.String())), nil
}

func (t *ExecutorStateType) UnmarshalJSON(data []byte) error {
	switch removeQuotes(string(data)) {
	case ExecutorStateTypeNoTaskInProgress.String():
		*t = ExecutorStateTypeNoTaskInProgress
	case ExecutorStateTypeStartingExecution.String():
		*t = ExecutorStateTypeStartingExecution
	case ExecutorStateTypeInterBrokerReplicaMovementTaskInProgress.String():
		*t = ExecutorStateTypeInterBrokerReplicaMovementTaskInProgress
	case ExecutorStateTypeIntraBrokerReplicaMovementTaskInProgress.String():
		*t = ExecutorStateTypeIntraBrokerReplicaMovementTaskInProgress
	case ExecutorStateTypeLeaderMovementTaskInProgress.String():
		*t = ExecutorStateTypeLeaderMovementTaskInProgress
	case ExecutorStateTypeStoppingExecution.String():
		*t = ExecutorStateTypeStoppingExecution
	case ExecutorStateTypeInitializingProposalExecution.String():
		*t = ExecutorStateTypeInitializingProposalExecution
	case ExecutorStateTypeGeneratingProposalsForExecution.String():
		*t = ExecutorStateTypeGeneratingProposalsForExecution
	case ExecutorStateTypeUndefined.String():
		fallthrough
	default:
		*t = ExecutorStateTypeUndefined
	}

	return nil
}

func (t *ExecutorStateType) UnmarshalText(data []byte) error {
	return t.UnmarshalJSON(data)
}
