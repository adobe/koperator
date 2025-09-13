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
	"strconv"

	"github.com/banzaicloud/go-cruise-control/integration_test/helpers"
	"github.com/banzaicloud/go-cruise-control/pkg/api"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Rebalance",
	Label("api:demote_broker", "api:user_tasks", "api:rebalance", "api:state"), func() {

		var (
			brokerID int32 = 2
		)

		BeforeEach(func(ctx SpecContext) {
			By("waiting until Cruise Control is ready")
			Eventually(ctx, func() bool {
				ready, err := helpers.IsCruiseControlReady(ctx, cruisecontrol)
				Expect(err).NotTo(HaveOccurred())
				return ready
			}, CruiseControlReadyTimeout, 15).Should(BeTrue())
		})

		Describe("Re-balancing Kafka cluster", func() {
			Context("after a broker got demoted", func() {
				It("should return no error", func(ctx SpecContext) {
					By("demoting broker")
					req := api.DemoteBrokerRequestWithDefaults()
					req.BrokerIDs = []int32{brokerID}
					req.Reason = "integration testing"
					req.DryRun = false

					resp, err := cruisecontrol.DemoteBroker(ctx, req)
					Expect(err).NotTo(HaveOccurred())
					Expect(resp.Failed()).To(BeFalse())

					By("waiting until executing demote request finished")
					Eventually(ctx, func() bool {
						finished, err := helpers.HasUserTaskFinished(ctx, cruisecontrol, resp.TaskID)
						Expect(err).NotTo(HaveOccurred())
						return finished
					}, 300, 15).Should(BeTrue())

					By("checking that the broker has no leader partitions")
					Eventually(ctx, func() int32 {
						req2 := api.KafkaClusterStateRequestWithDefaults()
						req2.Reason = "integration testing"

						resp3, err := cruisecontrol.KafkaClusterState(ctx, req2)
						Expect(err).NotTo(HaveOccurred())
						Expect(resp3.Failed()).To(BeFalse())

						Expect(resp3.Result.KafkaBrokerState.LeaderCountByBrokerID).ToNot(BeEmpty())
						leaders := resp3.Result.KafkaBrokerState.LeaderCountByBrokerID[strconv.Itoa(int(brokerID))]
						log.V(0).Info("leader partitions on broker",
							"broker_id", brokerID, "leaders", leaders)

						return leaders
					}, 300, 15).Should(BeNumerically("==", 0))

					By("re-balancing cluster after broker got demoted")
					req3 := api.RebalanceRequestWithDefaults()
					req3.DryRun = false
					req3.ExcludeRecentlyDemotedBrokers = false
					resp4, err := cruisecontrol.Rebalance(ctx, req3)
					Expect(err).NotTo(HaveOccurred())
					Expect(resp4.Failed()).To(BeFalse())

					By("waiting until re-balancing Kafka cluster finished")
					Eventually(ctx, func() bool {
						finished, err := helpers.HasUserTaskFinished(ctx, cruisecontrol, resp4.TaskID)
						Expect(err).NotTo(HaveOccurred())
						return finished
					}, 300, 15).Should(BeTrue())
				})
			})
		})
	})
