// Copyright © 2025 Cisco Systems, Inc. and/or its affiliates
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mocks

import (
	"context"

	api "github.com/banzaicloud/go-cruise-control/pkg/api"
	cctypes "github.com/banzaicloud/go-cruise-control/pkg/types"

	v1beta1 "github.com/banzaicloud/koperator/api/v1beta1"
	"github.com/banzaicloud/koperator/pkg/scale"
)

// noopCruiseControlScaler implements scale.CruiseControlScaler with safe no-op behavior.
// It avoids returning errors so controllers simply observe CC as not ready and requeue.
type noopCruiseControlScaler struct{}

func (n *noopCruiseControlScaler) IsReady(ctx context.Context) bool { return false }
func (n *noopCruiseControlScaler) IsUp(ctx context.Context) bool    { return false }

func (n *noopCruiseControlScaler) Status(ctx context.Context) (scale.StatusTaskResult, error) {
	// Report CC components as not ready without error
	st := scale.CruiseControlStatus{
		MonitorReady:  false,
		ExecutorReady: false,
		AnalyzerReady: false,
		ProposalReady: false,
		GoalsReady:    false,
	}
	return scale.StatusTaskResult{Status: &st, TaskResult: &scale.Result{State: v1beta1.CruiseControlTaskActive}}, nil
}

func (n *noopCruiseControlScaler) StatusTask(ctx context.Context, taskId string) (scale.StatusTaskResult, error) {
	return scale.StatusTaskResult{TaskResult: &scale.Result{TaskID: taskId, State: v1beta1.CruiseControlTaskCompleted}}, nil
}

func (n *noopCruiseControlScaler) UserTasks(ctx context.Context, taskIDs ...string) ([]*scale.Result, error) {
	return []*scale.Result{}, nil
}

func (n *noopCruiseControlScaler) AddBrokers(ctx context.Context, brokerIDs ...string) (*scale.Result, error) {
	return &scale.Result{State: v1beta1.CruiseControlTaskActive}, nil
}

func (n *noopCruiseControlScaler) AddBrokersWithParams(ctx context.Context, params map[string]string) (*scale.Result, error) {
	return &scale.Result{State: v1beta1.CruiseControlTaskActive}, nil
}

func (n *noopCruiseControlScaler) RemoveBrokersWithParams(ctx context.Context, params map[string]string) (*scale.Result, error) {
	return &scale.Result{State: v1beta1.CruiseControlTaskActive}, nil
}

func (n *noopCruiseControlScaler) RebalanceWithParams(ctx context.Context, params map[string]string) (*scale.Result, error) {
	return &scale.Result{State: v1beta1.CruiseControlTaskActive}, nil
}

func (n *noopCruiseControlScaler) StopExecution(ctx context.Context) (*scale.Result, error) {
	return &scale.Result{State: v1beta1.CruiseControlTaskCompleted}, nil
}

func (n *noopCruiseControlScaler) RemoveBrokers(ctx context.Context, brokerIDs ...string) (*scale.Result, error) {
	return &scale.Result{State: v1beta1.CruiseControlTaskActive}, nil
}

func (n *noopCruiseControlScaler) RemoveDisksWithParams(ctx context.Context, params map[string]string) (*scale.Result, error) {
	return &scale.Result{State: v1beta1.CruiseControlTaskActive}, nil
}

func (n *noopCruiseControlScaler) RebalanceDisks(ctx context.Context, brokerIDs ...string) (*scale.Result, error) {
	return &scale.Result{State: v1beta1.CruiseControlTaskActive}, nil
}

func (n *noopCruiseControlScaler) BrokersWithState(ctx context.Context, states ...scale.KafkaBrokerState) ([]string, error) {
	return []string{}, nil
}

func (n *noopCruiseControlScaler) KafkaClusterState(ctx context.Context) (*cctypes.KafkaClusterState, error) {
	return &cctypes.KafkaClusterState{}, nil
}

func (n *noopCruiseControlScaler) PartitionReplicasByBroker(ctx context.Context) (map[string]int32, error) {
	return map[string]int32{}, nil
}

func (n *noopCruiseControlScaler) BrokerWithLeastPartitionReplicas(ctx context.Context) (string, error) {
	return "", nil
}

func (n *noopCruiseControlScaler) LogDirsByBroker(ctx context.Context) (map[string]map[scale.LogDirState][]string, error) {
	return map[string]map[scale.LogDirState][]string{}, nil
}

func (n *noopCruiseControlScaler) KafkaClusterLoad(ctx context.Context) (*api.KafkaClusterLoadResponse, error) {
	return &api.KafkaClusterLoadResponse{}, nil
}

// NewNoopCruiseControlScaler returns a singleton-ish no-op scaler instance.
func NewNoopCruiseControlScaler() scale.CruiseControlScaler { return &noopCruiseControlScaler{} }

// NewNoopScaleFactory produces a factory returning the no-op scaler to avoid test races.
func NewNoopScaleFactory() func(ctx context.Context, kafkaCluster *v1beta1.KafkaCluster) (scale.CruiseControlScaler, error) {
	return func(ctx context.Context, kafkaCluster *v1beta1.KafkaCluster) (scale.CruiseControlScaler, error) {
		return NewNoopCruiseControlScaler(), nil
	}
}
