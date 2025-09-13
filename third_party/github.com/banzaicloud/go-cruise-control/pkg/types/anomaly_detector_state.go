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

type AnomalyDetectorState struct {
	SelfHealingEnabled  []AnomalyType `json:"selfHealingEnabled"`
	SelfHealingDisabled []AnomalyType `json:"selfHealingDisabled"`

	SelfHealingEnabledRatio SelfHealingEnabledRatio `json:"selfHealingEnabledRatio"`

	RecentGoalViolations    []AnomalyDetails `json:"recentGoalViolations"`
	RecentBrokerFailures    []AnomalyDetails `json:"recentBrokerFailures"`
	RecentMetricAnomalies   []AnomalyDetails `json:"recentMetricAnomalies"`
	RecentDiskFailures      []AnomalyDetails `json:"recentDiskFailures"`
	RecentTopicAnomalies    []AnomalyDetails `json:"recentTopicAnomalies"`
	RecentMaintenanceEvents []AnomalyDetails `json:"recentMaintenanceEvents"`

	Metrics                   AnomalyMetrics `json:"metrics"`
	OngoingSelfHealingAnomaly AnomalyType    `json:"ongoingSelfHealingAnomaly"`
	BalancednessScore         float64        `json:"balancednessScore"`
}

type SelfHealingEnabledRatio struct {
	GoalViolation    float64 `json:"GOAL_VIOLATION"`
	BrokerFailure    float64 `json:"BROKER_FAILURE"`
	MetricAnomaly    float64 `json:"METRIC_ANOMALY"`
	DiskFailure      float64 `json:"DISK_FAILURE"`
	TopicAnomaly     float64 `json:"TOPIC_ANOMALY"`
	MaintenanceEvent float64 `json:"MAINTENANCE_EVENT"`
}

const (
	AnomalyStatusUndefined AnomalyStatus = iota
	AnomalyStatusDetected
	AnomalyStatusIgnored
	AnomalyStatusFixStarted
	AnomalyStatusFixFailedToStart
	AnomalyStatusCheckWithDelay
	AnomalyStatusLoadMonitorNotReady
	AnomalyStatusCompletenessNotReady
)

type AnomalyStatus int8

func (g AnomalyStatus) String() string {
	switch g {
	case AnomalyStatusDetected:
		return "DETECTED"
	case AnomalyStatusIgnored:
		return "IGNORED"
	case AnomalyStatusFixStarted:
		return "FIX_STARTED"
	case AnomalyStatusFixFailedToStart:
		return "FIX_FAILED_TO_START"
	case AnomalyStatusCheckWithDelay:
		return "CHECK_WITH_DELAY"
	case AnomalyStatusLoadMonitorNotReady:
		return "LOAD_MONITOR_NOT_READY"
	case AnomalyStatusCompletenessNotReady:
		return "COMPLETENESS_NOT_READY"
	case AnomalyStatusUndefined:
		fallthrough
	default:
		return Undefined
	}
}

func (g *AnomalyStatus) MarshalJSON() ([]byte, error) {
	return []byte(addQuotes(g.String())), nil
}

func (g *AnomalyStatus) UnmarshalJSON(data []byte) error {
	switch removeQuotes(string(data)) {
	case AnomalyStatusDetected.String():
		*g = AnomalyStatusDetected
	case AnomalyStatusIgnored.String():
		*g = AnomalyStatusIgnored
	case AnomalyStatusFixStarted.String():
		*g = AnomalyStatusFixStarted
	case AnomalyStatusFixFailedToStart.String():
		*g = AnomalyStatusFixFailedToStart
	case AnomalyStatusCheckWithDelay.String():
		*g = AnomalyStatusCheckWithDelay
	case AnomalyStatusLoadMonitorNotReady.String():
		*g = AnomalyStatusLoadMonitorNotReady
	case AnomalyStatusCompletenessNotReady.String():
		*g = AnomalyStatusCompletenessNotReady
	case AnomalyStatusUndefined.String():
		fallthrough
	default:
		*g = AnomalyStatusUndefined
	}
	return nil
}

func (g *AnomalyStatus) UnmarshalText(data []byte) error {
	return g.UnmarshalJSON(data)
}

type AnomalyDetails struct {
	StatusUpdateMs         int64            `json:"statusUpdateMs"`
	DetectionMs            int64            `json:"detectionMs"`
	Status                 AnomalyStatus    `json:"status"`
	AnomalyID              string           `json:"anomalyId"` // It might be a separate type
	FixableViolatedGoals   []Goal           `json:"fixableViolatedGoals"`
	UnfixableViolatedGoals []Goal           `json:"unfixableViolatedGoals"`
	OptimizationResult     string           `json:"optimizationResult"` // It might be a separate type
	FailedBrokersByTimeMs  map[string]int64 `json:"failedBrokersByTimeMs"`
	FailedDisksByTimeMs    map[string]int64 `json:"failedDisksByTimeMs"`
	Description            string           `json:"description"`
}

type AnomalyMetrics struct {
	MeanTimeBetweenAnomaliesMs MeanTimeBetweenAnomaliesMs `json:"meanTimeBetweenAnomaliesMs"`

	MeanTimeToStartFixMs        float64 `json:"meanTimeToStartFixMs"`
	NumSelfHealingStarted       int64   `json:"numSelfHealingStarted"`
	NumSelfHealingFailedToStart int64   `json:"numSelfHealingFailedToStart"`
	OngoingAnomalyDurationMs    int64   `json:"ongoingAnomalyDurationMs"`
}

type MeanTimeBetweenAnomaliesMs struct {
	GoalViolation    float64 `json:"GOAL_VIOLATION"`
	BrokerFailure    float64 `json:"BROKER_FAILURE"`
	MetricAnomaly    float64 `json:"METRIC_ANOMALY"`
	DiskFailure      float64 `json:"DISK_FAILURE"`
	TopicAnomaly     float64 `json:"TOPIC_ANOMALY"`
	MaintenanceEvent float64 `json:"MAINTENANCE_EVENT"`
}
