// Copyright 2026 Adobe. All rights reserved.
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

package cruisecontrol

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/banzaicloud/koperator/api/v1beta1"
)

func TestIsBrokerRemovalPending(t *testing.T) {
	cluster := func(specIDs []int32, statusIDs []string) *v1beta1.KafkaCluster {
		kc := &v1beta1.KafkaCluster{}
		for _, id := range specIDs {
			kc.Spec.Brokers = append(kc.Spec.Brokers, v1beta1.Broker{Id: id})
		}
		kc.Status.BrokersState = map[string]v1beta1.BrokerState{}
		for _, id := range statusIDs {
			kc.Status.BrokersState[id] = v1beta1.BrokerState{}
		}
		return kc
	}

	tests := []struct {
		testName string
		cluster  *v1beta1.KafkaCluster
		expected bool
	}{
		{
			testName: "steady state: every status broker is in the spec",
			cluster:  cluster([]int32{0, 1, 2}, []string{"0", "1", "2"}),
			expected: false,
		},
		{
			testName: "removal pending: a status broker was dropped from the spec",
			cluster:  cluster([]int32{0, 1, 2}, []string{"0", "1", "2", "103"}),
			expected: true,
		},
		{
			testName: "upscale in progress: a new spec broker not yet in status is NOT a removal",
			cluster:  cluster([]int32{0, 1, 2, 103}, []string{"0", "1", "2"}),
			expected: false,
		},
		{
			testName: "empty status",
			cluster:  cluster([]int32{0, 1, 2}, nil),
			expected: false,
		},
	}

	for _, test := range tests {
		t.Run(test.testName, func(t *testing.T) {
			require.Equal(t, test.expected, isBrokerRemovalPending(test.cluster))
		})
	}
}

// TestIsBrokerDeletionInProgress guards against reintroducing the #301 deadlock: isBrokerDeletionInProgress
// must only report true for a downscale that Cruise Control is actively executing right now
// (GracefulDownscaleRunning), not for Required/Scheduled (no in-flight CC-side task to protect) or Succeeded
// (already done). Treating Required/Scheduled as "in progress" makes the capacity generator refuse to add a
// concurrently-added broker's capacity forever, which in turn stalls its add_broker op and - because the task
// controller processes add_broker before remove_broker - prevents the downscale from ever advancing past
// Required/Scheduled either.
func TestIsBrokerDeletionInProgress(t *testing.T) {
	state := func(ccState v1beta1.CruiseControlState) v1beta1.BrokerState {
		return v1beta1.BrokerState{GracefulActionState: v1beta1.GracefulActionState{CruiseControlState: ccState}}
	}

	tests := []struct {
		testName string
		ccState  v1beta1.CruiseControlState
		expected bool
	}{
		{testName: "no downscale state", ccState: "", expected: false},
		{testName: "downscale required: no CruiseControlOperation exists yet", ccState: v1beta1.GracefulDownscaleRequired, expected: false},
		{testName: "downscale scheduled: CruiseControlOperation created but not yet submitted to CC", ccState: v1beta1.GracefulDownscaleScheduled, expected: false},
		{testName: "downscale running: actively executing in CC", ccState: v1beta1.GracefulDownscaleRunning, expected: true},
		{testName: "downscale succeeded", ccState: v1beta1.GracefulDownscaleSucceeded, expected: false},
		{testName: "downscale completed with error", ccState: v1beta1.GracefulDownscaleCompletedWithError, expected: false},
		{testName: "downscale paused", ccState: v1beta1.GracefulDownscalePaused, expected: false},
		{testName: "upscale running is not a downscale", ccState: v1beta1.GracefulUpscaleRunning, expected: false},
	}

	for _, test := range tests {
		t.Run(test.testName, func(t *testing.T) {
			brokerState := map[string]v1beta1.BrokerState{"100": state(test.ccState)}
			require.Equal(t, test.expected, isBrokerDeletionInProgress(brokerState))
		})
	}
}
