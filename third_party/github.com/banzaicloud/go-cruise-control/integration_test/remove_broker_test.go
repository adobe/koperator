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

var _ = Describe("Remove Broker",
	Label("api:remove_broker", "api:add_broker", "api:user_tasks", "api:kafka_cluster_state", "api:state"),
	Serial,
	func() {

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

		AfterEach(func(ctx SpecContext) {
			By("adding broker back to the Kafka cluster")
			req := api.AddBrokerRequestWithDefaults()
			req.BrokerIDs = []int32{brokerID}
			req.DryRun = false
			req.Reason = "integration testing"
			resp, err := cruisecontrol.AddBroker(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Failed()).To(BeFalse())
		})

		Describe("Removing a broker in Kafka cluster", func() {
			It("should return no error", func(ctx SpecContext) {
				By("sending a remove request to Cruise Control")
				req := api.RemoveBrokerRequestWithDefaults()
				req.BrokerIDs = []int32{brokerID}
				req.DryRun = false
				req.Reason = "integration testing"

				resp, err := cruisecontrol.RemoveBroker(ctx, req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.Failed()).To(BeFalse())

				By("waiting until the remove task finished")
				Eventually(ctx, func() bool {
					finished, err := helpers.HasUserTaskFinished(ctx, cruisecontrol, resp.TaskID)
					Expect(err).NotTo(HaveOccurred())
					return finished
				}, 300, 15).Should(BeTrue())

				By("checking that the broker has no partition replicas")
				Eventually(ctx, func() int32 {
					req2 := api.KafkaClusterStateRequestWithDefaults()
					req2.Reason = "integration testing"

					resp2, err := cruisecontrol.KafkaClusterState(ctx, req2)
					Expect(err).NotTo(HaveOccurred())
					Expect(resp2.Failed()).To(BeFalse())

					Expect(resp2.Result.KafkaBrokerState.ReplicaCountByBrokerID).ToNot(BeEmpty())
					replicas := resp2.Result.KafkaBrokerState.ReplicaCountByBrokerID[strconv.Itoa(int(brokerID))]
					log.V(0).Info("partition replicas on broker",
						"broker_id", brokerID, "replicas", replicas)
					return replicas
				}, 300, 15).Should(BeNumerically("==", 0))
			})
		})
	})
