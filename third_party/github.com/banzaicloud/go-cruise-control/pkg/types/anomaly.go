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

const (
	AnomalyTypeUndefined AnomalyType = iota
	AnomalyTypeGoalViolation
	AnomalyTypeBrokerFailure
	AnomalyTypeMetricAnomaly
	AnomalyTypeDiskFailure
	AnomalyTypeTopicAnomaly
	AnomalyTypeMaintenanceEvent
)

type AnomalyType int8

func (g AnomalyType) String() string {
	switch g {
	case AnomalyTypeGoalViolation:
		return "GOAL_VIOLATION"
	case AnomalyTypeBrokerFailure:
		return "BROKER_FAILURE"
	case AnomalyTypeMetricAnomaly:
		return "METRIC_ANOMALY"
	case AnomalyTypeDiskFailure:
		return "DISK_FAILURE"
	case AnomalyTypeTopicAnomaly:
		return "TOPIC_ANOMALY"
	case AnomalyTypeMaintenanceEvent:
		return "MAINTENANCE_EVENT"
	case AnomalyTypeUndefined:
		fallthrough
	default:
		return Undefined
	}
}

func (g *AnomalyType) MarshalJSON() ([]byte, error) {
	return []byte(addQuotes(g.String())), nil
}

func (g *AnomalyType) UnmarshalJSON(data []byte) error {
	switch removeQuotes(string(data)) {
	case AnomalyTypeGoalViolation.String():
		*g = AnomalyTypeGoalViolation
	case AnomalyTypeBrokerFailure.String():
		*g = AnomalyTypeBrokerFailure
	case AnomalyTypeMetricAnomaly.String():
		*g = AnomalyTypeMetricAnomaly
	case AnomalyTypeDiskFailure.String():
		*g = AnomalyTypeDiskFailure
	case AnomalyTypeTopicAnomaly.String():
		*g = AnomalyTypeTopicAnomaly
	case AnomalyTypeMaintenanceEvent.String():
		*g = AnomalyTypeMaintenanceEvent
	case AnomalyTypeUndefined.String():
		fallthrough
	default:
		*g = AnomalyTypeUndefined
	}
	return nil
}

func (g *AnomalyType) UnmarshalText(data []byte) error {
	return g.UnmarshalJSON(data)
}
