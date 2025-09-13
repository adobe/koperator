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

var _ = Describe("Add Broker", Label("api:add_broker", "api:state"), Serial, func() {

	var (
		brokerID   int32 = 2
		readyGoals []types.Goal
	)

	BeforeEach(func(ctx SpecContext) {
		By("waiting until Cruise Control is ready")
		Eventually(ctx, func() bool {
			ready, err := helpers.IsCruiseControlReady(ctx, cruisecontrol)
			Expect(err).NotTo(HaveOccurred())
			return ready
		}, CruiseControlReadyTimeout, 15).Should(BeTrue())

		By("getting list of ready goals")
		req := api.StateRequestWithDefaults()
		resp, err := cruisecontrol.State(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.Failed()).To(BeFalse())
		readyGoals = resp.Result.AnalyzerState.ReadyGoals
	})

	Describe("Adding broker to Kafka cluster", func() {
		Context("using goals in ready state", func() {
			It("should return no error", func(ctx SpecContext) {
				req := api.AddBrokerRequestWithDefaults()
				req.Goals = readyGoals
				req.BrokerIDs = []int32{brokerID}
				req.Reason = "integration testing"
				req.DryRun = false

				resp, err := cruisecontrol.AddBroker(ctx, req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.Failed()).To(BeFalse())
			})
		})
	})

	Describe("Adding broker to Kafka cluster", func() {
		Context("using the default goals", func() {
			It("should return no error", func(ctx SpecContext) {
				req := api.AddBrokerRequestWithDefaults()
				req.BrokerIDs = []int32{brokerID}
				req.Reason = "integration testing"
				req.DryRun = false

				resp, err := cruisecontrol.AddBroker(ctx, req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.Failed()).To(BeFalse())
			})
		})
	})
})
