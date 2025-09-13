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

type OptimizationResult struct {
	Proposals              []ExecutionProposal `json:"proposals"`
	LoadBeforeOptimization BrokerStats         `json:"loadBeforeOptimization"`
	Summary                OptimizerResult     `json:"summary"`
	GoalSummary            []GoalSummary       `json:"goalSummary"`
	LoadAfterOptimization  BrokerStats         `json:"loadAfterOptimization"`
	Version                int32               `json:"version"`
}

type OptimizerResult struct {
	NumReplicaMovements             int32           `json:"numReplicaMovements"`
	DataToMoveMB                    int64           `json:"dataToMoveMB"`
	NumIntraBrokerReplicaMovements  int32           `json:"numIntraBrokerReplicaMovements"`
	IntraBrokerDataToMoveMB         int64           `json:"intraBrokerDataToMoveMB"`
	NumLeaderMovements              int32           `json:"numLeaderMovements"`
	RecentWindows                   int32           `json:"recentWindows"`
	MonitoredPartitionsPercentage   float64         `json:"monitoredPartitionsPercentage"`
	ExcludedTopics                  []string        `json:"excludedTopics"`
	ExcludedBrokersForReplicaMove   []int32         `json:"excludedBrokersForReplicaMove"`
	ExcludedBrokersForLeadership    []int32         `json:"excludedBrokersForLeadership"`
	OnDemandBalancednessScoreBefore float64         `json:"onDemandBalancednessScoreBefore"`
	OnDemandBalancednessScoreAfter  float64         `json:"onDemandBalancednessScoreAfter"`
	ProvisionStatus                 ProvisionStatus `json:"provisionStatus"`
	ProvisionRecommendation         string          `json:"ProvisionRecommendation"`
}

const (
	UndefinedProvisionedStatus ProvisionStatus = iota
	ProvisionStatusRightSized
	ProvisionStatusUnderProvisioned
	ProvisionStatusOverProvisioned
	ProvisionStatusUndecided
)

type ProvisionStatus int8

func (s ProvisionStatus) String() string {
	switch s {
	case ProvisionStatusRightSized:
		return "RIGHT_SIZED"
	case ProvisionStatusUnderProvisioned:
		return "UNDER_PROVISIONED"
	case ProvisionStatusOverProvisioned:
		return "OVER_PROVISIONED"
	case ProvisionStatusUndecided:
		return "UNDECIDED"
	case UndefinedProvisionedStatus:
		fallthrough
	default:
		return Undefined
	}
}

func (s ProvisionStatus) MarshalJSON() ([]byte, error) {
	return []byte(addQuotes(s.String())), nil
}

func (s *ProvisionStatus) UnmarshalJSON(data []byte) error {
	switch removeQuotes(string(data)) {
	case ProvisionStatusRightSized.String():
		*s = ProvisionStatusRightSized
	case ProvisionStatusUnderProvisioned.String():
		*s = ProvisionStatusUnderProvisioned
	case ProvisionStatusOverProvisioned.String():
		*s = ProvisionStatusOverProvisioned
	case ProvisionStatusUndecided.String():
		*s = ProvisionStatusUndecided
	default:
		*s = UndefinedProvisionedStatus
	}
	return nil
}

func (s *ProvisionStatus) UnmarshalText(data []byte) error {
	return s.UnmarshalJSON(data)
}
