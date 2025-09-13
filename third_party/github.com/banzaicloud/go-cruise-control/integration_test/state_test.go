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

package integration_test

import (
	"github.com/banzaicloud/go-cruise-control/integration_test/helpers"
	"github.com/banzaicloud/go-cruise-control/pkg/api"
	"github.com/banzaicloud/go-cruise-control/pkg/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("State", Label("api:state"), func() {

	BeforeEach(func(ctx SpecContext) {
		By("waiting until Cruise Control is ready")
		Eventually(ctx, func() bool {
			ready, err := helpers.IsCruiseControlReady(ctx, cruisecontrol)
			Expect(err).NotTo(HaveOccurred())
			return ready
		}, CruiseControlReadyTimeout, 15).Should(BeTrue())
	})

	Describe("Getting Cruise Control state", func() {
		It("should return no error", func(ctx SpecContext) {
			By("sending request for Cruise Control state")
			req := api.StateRequestWithDefaults()

			resp, err := cruisecontrol.State(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Failed()).To(BeFalse())

			By("checking Executor")
			execStateTypes := []types.ExecutorStateType{
				types.ExecutorStateTypeNoTaskInProgress,
				types.ExecutorStateTypeStartingExecution,
				types.ExecutorStateTypeInterBrokerReplicaMovementTaskInProgress,
				types.ExecutorStateTypeIntraBrokerReplicaMovementTaskInProgress,
				types.ExecutorStateTypeLeaderMovementTaskInProgress,
				types.ExecutorStateTypeStoppingExecution,
				types.ExecutorStateTypeInitializingProposalExecution,
				types.ExecutorStateTypeGeneratingProposalsForExecution,
			}
			Expect(resp.Result.ExecutorState.State).To(BeElementOf(execStateTypes))

			By("checking Analyzer")
			Expect(resp.Result.AnalyzerState.ReadyGoals).ToNot(BeEmpty())

			By("checking Monitor")
			monStateTypes := []types.MonitorState{
				types.MonitorStateNotStarted,
				types.MonitorStateRunning,
				types.MonitorStatePaused,
				types.MonitorStateSampling,
				types.MonitorStateBootstrapping,
				types.MonitorStateTraining,
				types.MonitorStateLoading,
			}
			Expect(resp.Result.MonitorState.State).To(BeElementOf(monStateTypes))
		})
	})
})
