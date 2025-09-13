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

type LoadMonitorState struct {
	BootstrapProgressPct float64 `json:"bootstrapProgressPct,omitempty"`
	Error                string  `json:"error,omitempty"`
	LoadingProgressPct   float64 `json:"loadingProgressPct,omitempty"`

	MonitoredWindows map[string]float64 `json:"monitoredWindows,omitempty"`

	MonitoringCoveragePercentage float64 `json:"monitoringCoveragePct"`

	NumFlawedPartitions float32 `json:"numFlawedPartitions"`
	NumMonitoredWindows float32 `json:"numMonitoredWindows"`
	NumTotalPartitions  float32 `json:"numTotalPartitions"`
	NumValidPartitions  float32 `json:"numValidPartitions"`

	ReasonOfLatestPauseOrResume string `json:"reasonOfLatestPauseOrResume,omitempty"`

	Trained            bool         `json:"trained"`
	TrainingPercentage float64      `json:"trainingPct"`
	State              MonitorState `json:"state"`
}

const (
	MonitorStateUndefined MonitorState = iota
	MonitorStateNotStarted
	MonitorStateRunning
	MonitorStatePaused
	MonitorStateSampling
	MonitorStateBootstrapping
	MonitorStateTraining
	MonitorStateLoading
)

type MonitorState int8

func (s MonitorState) String() string {
	switch s {
	case MonitorStateNotStarted:
		return "NOT_STARTED"
	case MonitorStateRunning:
		return "RUNNING"
	case MonitorStatePaused:
		return "PAUSED"
	case MonitorStateSampling:
		return "SAMPLING"
	case MonitorStateBootstrapping:
		return "BOOTSTRAPPING"
	case MonitorStateTraining:
		return "TRAINING"
	case MonitorStateLoading:
		return "LOADING"
	case MonitorStateUndefined:
		fallthrough
	default:
		return Undefined
	}
}

func (s *MonitorState) MarshalJSON() ([]byte, error) {
	return []byte(addQuotes(s.String())), nil
}

func (s *MonitorState) UnmarshalJSON(data []byte) error {
	switch removeQuotes(string(data)) {
	case MonitorStateNotStarted.String():
		*s = MonitorStateNotStarted
	case MonitorStateRunning.String():
		*s = MonitorStateRunning
	case MonitorStatePaused.String():
		*s = MonitorStatePaused
	case MonitorStateSampling.String():
		*s = MonitorStateSampling
	case MonitorStateBootstrapping.String():
		*s = MonitorStateBootstrapping
	case MonitorStateTraining.String():
		*s = MonitorStateTraining
	case MonitorStateLoading.String():
		*s = MonitorStateLoading
	case MonitorStateUndefined.String():
		fallthrough
	default:
		*s = MonitorStateUndefined
	}
	return nil
}

func (s *MonitorState) UnmarshalText(data []byte) error {
	return s.UnmarshalJSON(data)
}

// MonitorStateFromString converts s string to MonitorState
func MonitorStateFromString(s string) MonitorState {
	var m MonitorState
	_ = m.UnmarshalJSON([]byte(s))
	return m
}
