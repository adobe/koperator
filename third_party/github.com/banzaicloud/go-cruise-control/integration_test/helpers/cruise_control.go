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

package helpers

import (
	"context"

	"github.com/banzaicloud/go-cruise-control/pkg/api"
	"github.com/banzaicloud/go-cruise-control/pkg/client"
	"github.com/banzaicloud/go-cruise-control/pkg/types"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
)

func IsCruiseControlReady(ctx context.Context, cruisecontrol *client.Client) (bool, error) {
	log := logr.FromContextOrDiscard(ctx)

	req := api.StateRequestWithDefaults()
	req.Verbose = true
	resp, err := cruisecontrol.State(ctx, req)
	if err != nil {
		return false, err
	}

	monitorReady := resp.Result.MonitorState.State == types.MonitorStateRunning
	executorReady := resp.Result.ExecutorState.State == types.ExecutorStateTypeNoTaskInProgress

	var goalsReady bool
	if len(resp.Result.AnalyzerState.GoalReadiness) > 0 {
		goalsReady = true
		for _, goal := range resp.Result.AnalyzerState.GoalReadiness {
			if goal.Status != types.GoalReadinessStatusReady {
				goalsReady = false
				break
			}
		}
	}
	analyzerReady := resp.Result.AnalyzerState.IsProposalReady && goalsReady

	log.V(0).Info("cruise control readiness",
		"analyzer", analyzerReady, "monitor", monitorReady, "executor", executorReady,
		"goals ready", goalsReady,
		"monitored windows", resp.Result.MonitorState.NumMonitoredWindows,
		"monitoring coverage percentage", resp.Result.MonitorState.MonitoringCoveragePercentage)
	return analyzerReady && monitorReady && executorReady, nil
}

func HasUserTaskFinished(ctx context.Context, cruisecontrol *client.Client, taskID string) (bool, error) {
	log := logr.FromContextOrDiscard(ctx)

	req := api.UserTasksRequestWithDefaults()
	req.Reason = "integration testing"
	req.UserTaskIDs = []string{taskID}

	resp, err := cruisecontrol.UserTasks(ctx, req)
	if err != nil {
		return false, err
	}

	if len(resp.Result.UserTasks) == 0 {
		return false, errors.New("user task does not exist")
	}

	task := resp.Result.UserTasks[0]
	log.V(0).Info("user task state", "task_id", taskID, "state", task.Status)

	switch task.Status {
	case types.UserTaskStatusCompleted:
		return true, nil
	case types.UserTaskStatusCompletedWithError:
		return true, errors.New("user task completed with error")
	}
	return false, nil
}
