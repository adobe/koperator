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
	EndpointAdmin types.APIEndpoint = "ADMIN"
)

type AdminRequest struct {
	types.GenericRequestWithReason

	// Change upper bound of ongoing replica movements between disks within each broker.
	ConcurrentIntraBrokerPartitionMovements int32 `param:"concurrent_intra_broker_partition_movements,omitempty"`
	// Change upper bound of ongoing leadership movements.
	ConcurrentLeaderMovements int32 `param:"concurrent_leader_movements,omitempty"`
	// Change upper bound of ongoing replica movements going into/out of each broker.
	ConcurrentPartitionMovementsPerBroker int32 `param:"concurrent_partition_movements_per_broker,omitempty"`
	// Disable concurrency adjuster for given concurrency types.
	DisableConcurrencyAdjusterFor []types.ConcurrencyType `param:"disable_concurrency_adjuster_for,omitempty"`
	// Disable self-healing for certain anomaly types.
	DisableSelfHealingFor []types.AnomalyType `param:"disable_self_healing_for,omitempty"`
	// Drop broker ids from recently demoted broker list so that Cruise Control can move leader replicas or
	// to transfer replica leadership to these brokers.
	DropRecentlyDemotedBrokers []int32 `param:"drop_recently_demoted_brokers,omitempty"`
	// Drop broker ids from recently removed broker list so that Cruise Control can move replicas to these brokers.
	DropRecentlyRemovedBrokers []int32 `param:"drop_recently_removed_brokers,omitempty"`
	// Enable concurrency adjuster for given concurrency types.
	EnableConcurrencyAdjusterFor []types.ConcurrencyType `param:"enable_concurrency_adjuster_for,omitempty"`
	// Enable self-healing for certain anomaly types
	EnableSelfHealingFor []types.AnomalyType `param:"enable_self_healing_for,omitempty"`
	// Change execution progress check interval in milliseconds.
	ExecutionProgressCheckIntervalMs int64 `param:"execution_progress_check_interval_ms,omitempty"`
	// Whether to enable (true) or disable (false) MinISR-based concurrency adjustment.
	MinIsrBasedConcurrencyAdjustment bool `json:"min_isr_based_concurrency_adjustment,omitempty"`
	// Review id for 2-step verification.
	ReviewID int32 `param:"review_id,omitempty"`
}

func (s AdminRequest) Validate() error {
	if s.ConcurrentPartitionMovementsPerBroker == 0 {
		return errors.New("number of concurrent partition movements per broker must be bigger then 0")
	}

	if s.ConcurrentIntraBrokerPartitionMovements == 0 {
		return errors.New("number of concurrent intra broker partition movements must be bigger then 0")
	}

	if s.ConcurrentLeaderMovements == 0 {
		return errors.New("number of concurrent leader partition movements must be bigger then 0")
	}

	supportedAnomalies := map[types.AnomalyType]bool{
		types.AnomalyTypeDiskFailure:      true,
		types.AnomalyTypeBrokerFailure:    true,
		types.AnomalyTypeMetricAnomaly:    true,
		types.AnomalyTypeTopicAnomaly:     true,
		types.AnomalyTypeGoalViolation:    true,
		types.AnomalyTypeMaintenanceEvent: false,
		types.AnomalyTypeUndefined:        false,
	}

	unsupported := make([]types.AnomalyType, 0)
	for _, a := range s.DisableSelfHealingFor {
		if supported := supportedAnomalies[a]; !supported {
			unsupported = append(unsupported, a)
		}
	}
	if len(unsupported) > 0 {
		return errors.Errorf("disabling self healing for the following anomaly types is not supported: %s",
			unsupported)
	}

	unsupported = make([]types.AnomalyType, 0)
	for _, a := range s.DisableSelfHealingFor {
		if supported := supportedAnomalies[a]; !supported {
			unsupported = append(unsupported, a)
		}
	}
	if len(unsupported) > 0 {
		return errors.Errorf("enabling self healing for the following anomaly types is not supported: %s",
			unsupported)
	}

	return nil
}

func AdminRequestWithDefaults() *AdminRequest {
	return &AdminRequest{
		ConcurrentLeaderMovements:             defaultConcurrentLeaderMovements,
		ConcurrentPartitionMovementsPerBroker: defaultConcurrentPartitionMovementsPerBroker,
		ExecutionProgressCheckIntervalMs:      defaultExecutionProgressCheckIntervalMs,
	}
}

type AdminResponse struct {
	types.GenericResponse

	Result *types.AdminResult
}

func (r *AdminResponse) UnmarshalResponse(resp *http.Response) error {
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
		r.Result = &types.AdminResult{}
		d = r.Result
	default:
		r.Error = &types.APIError{}
		d = r.Error
	}

	if err = json.Unmarshal(bodyBytes, d); err != nil {
		return fmt.Errorf("failed to parse JSON response: %w", err)
	}

	return nil
}
