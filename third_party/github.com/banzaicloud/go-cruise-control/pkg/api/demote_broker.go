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

package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/pkg/errors"

	"github.com/banzaicloud/go-cruise-control/pkg/types"
)

const (
	EndpointDemoteBroker types.APIEndpoint = "DEMOTE_BROKER"
)

type DemoteBrokerRequest struct {
	types.GenericRequestWithReason

	// Whether to allow capacity estimation when cruise-control is unable to obtain all per-broker capacity information
	AllowCapacityEstimation bool `param:"allow_capacity_estimation"`
	// List of target broker ids
	BrokerIDs []int32 `param:"brokerid"`
	// The upper bound of ongoing leadership movements
	ConcurrentLeaderMovements int32 `param:"concurrent_leader_movements,omitempty"`
	// Whether to dry-run the request or not.
	DryRun bool `param:"dryrun"`
	// Whether to allow leader replicas to be moved to recently demoted brokers
	ExcludeRecentlyDemotedBrokers bool `param:"exclude_recently_demoted_brokers,omitempty"`
	// Execution progress check interval in milliseconds
	ExecutionProgressCheckIntervalMs int64 `param:"execution_progress_check_interval_ms,omitempty"`
	// Replica movement strategies to use
	ReplicaMovementStrategies []types.ReplicaMovementStrategy `param:"replica_movement_strategies,omitempty"`
	// Upper bound on the bandwidth in bytes per second used to move replicas
	ReplicationThrottle int64 `param:"replication_throttle,omitempty"`
	// Review id for 2-step verification
	ReviewID int32 `param:"review_id,omitempty"`
	// Whether to stop the ongoing execution (if any) and start executing the given request
	StopOngoingExecution bool `param:"stop_ongoing_execution,omitempty"`
	// Return detailed state information
	Verbose bool `param:"verbose,omitempty"`
	// Whether to operate on partitions which are currently under-replicated
	SkipUrpDemotion bool `param:"skip_urp_demotion"`
	// Whether to operate on partitions which only have follower replicas on the specified broker(s)
	ExcludeFollowerDemotion bool `param:"exclude_follower_demotion"`
	// List of broker id and logdir pair to be demoted in the cluster
	BrokerIDAndLogDirs types.BrokerIDAndLogDirs `param:"brokerid_and_logdirs,omitempty"`
}

func (s DemoteBrokerRequest) Validate() error {
	if len(s.BrokerIDs) < 1 {
		return errors.New("list of brokers must not be empty (BrokerIDs)")
	}

	if s.ConcurrentLeaderMovements == 0 {
		return errors.New("number of concurrent leader partition movements must be bigger then 0")
	}

	return nil
}

func DemoteBrokerRequestWithDefaults() *DemoteBrokerRequest {
	return &DemoteBrokerRequest{
		AllowCapacityEstimation:          true,
		ConcurrentLeaderMovements:        defaultConcurrentLeaderMovements,
		ExecutionProgressCheckIntervalMs: defaultExecutionProgressCheckIntervalMs,
		SkipUrpDemotion:                  true,
		ExcludeFollowerDemotion:          true,
	}
}

type DemoteBrokerResponse struct {
	types.GenericResponse

	Result *types.OptimizationResult
}

func (r *DemoteBrokerResponse) UnmarshalResponse(resp *http.Response) error {
	if err := r.GenericResponse.UnmarshalResponse(resp); err != nil {
		return fmt.Errorf("failed to parse HTTP response metadata: %w", err)
	}

	var bodyBytes []byte
	var err error

	bodyBytes, err = io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read HTTP response body: %w", err)
	}

	var d interface{}
	switch resp.StatusCode {
	case http.StatusOK:
		r.Result = &types.OptimizationResult{}
		d = r.Result
	case http.StatusAccepted:
		r.Progress = &types.ProgressResult{}
		d = r.Progress
	default:
		r.Error = &types.APIError{}
		d = r.Error
	}

	if err = json.Unmarshal(bodyBytes, d); err != nil {
		return fmt.Errorf("failed to parse JSON response: %w", err)
	}

	return nil
}
