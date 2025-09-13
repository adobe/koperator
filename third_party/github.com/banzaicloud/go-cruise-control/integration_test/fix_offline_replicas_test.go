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
	"fmt"

	"github.com/banzaicloud/go-cruise-control/integration_test/helpers"
	"github.com/banzaicloud/go-cruise-control/pkg/api"
	"github.com/banzaicloud/go-cruise-control/pkg/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Fix Offline Replicas",
	Label("api:fix_offline_replicas", "api:rebalance", "api:user_tasks", "api:kafka_cluster_state",
		"api:kafka_cluster_load", "api:state"),
	Ordered,
	func() {

		var (
			brokerID   int32 = 2
			brokerName       = fmt.Sprintf("kafka-%d", brokerID)
		)

		BeforeEach(func(ctx SpecContext) {
			By("waiting until Cruise Control is ready")
			Eventually(ctx, func() bool {
				ready, err := helpers.IsCruiseControlReady(ctx, cruisecontrol)
				Expect(err).NotTo(HaveOccurred())
				return ready
			}, CruiseControlReadyTimeout, 15).Should(BeTrue())

			By("taking the broker offline")
			err := testEnv.StopService(ctx, brokerName)
			Expect(err).NotTo(HaveOccurred())

			By("waiting until Cruise Control detects the offline broker")
			req2 := api.KafkaClusterLoadRequestWithDefaults()
			Eventually(ctx, func() bool {
				resp, err := cruisecontrol.KafkaClusterLoad(ctx, req2)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.Failed()).To(BeFalse())
				var offlineBrokerStats *types.BrokerLoadStats
				for _, broker := range resp.Result.Brokers {
					if broker.Broker == brokerID {
						offlineBrokerStats = &broker
					}
				}
				if offlineBrokerStats == nil || offlineBrokerStats.BrokerState == types.BrokerStateDead {
					return true
				}
				return false
			}, 300, 15).Should(BeTrue())
		})

		AfterAll(func(ctx SpecContext) {
			By("taking the broker online")
			err := testEnv.StartService(ctx, brokerName)
			Expect(err).NotTo(HaveOccurred())

			By("waiting until Cruise Control detects that the broker is online")
			req := api.KafkaClusterLoadRequestWithDefaults()
			Eventually(ctx, func() bool {
				resp, err := cruisecontrol.KafkaClusterLoad(ctx, req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.Failed()).To(BeFalse())
				var offlineBroker *types.BrokerLoadStats
				for _, broker := range resp.Result.Brokers {
					if broker.Broker == brokerID {
						offlineBroker = &broker
					}
				}
				if offlineBroker != nil &&
					(offlineBroker.BrokerState == types.BrokerStateNew ||
						offlineBroker.BrokerState == types.BrokerStateAlive) {
					return true
				}
				return false
			}, 300, 15).Should(BeTrue())

			By("rebalancing the Kafka cluster")
			req2 := api.RebalanceRequestWithDefaults()
			req2.DryRun = false
			resp2, err := cruisecontrol.Rebalance(ctx, req2)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp2.Failed()).To(BeFalse())
		})

		Describe("Fixing offline replicas in Kafka cluster", func() {
			Context("when one of the brokers are down", func() {
				It("should return no error", func(ctx SpecContext) {
					By("checking if there are offline replicas in the cluster")
					req := api.KafkaClusterStateRequestWithDefaults()
					resp, err := cruisecontrol.KafkaClusterState(ctx, req)
					Expect(err).NotTo(HaveOccurred())
					Expect(resp.Failed()).To(BeFalse())
					state := resp.Result.KafkaPartitionState
					if len(state.Offline) == 0 && len(state.WithOfflineReplicas) == 0 {
						Skip("skipping test as there are offline replicas in the Kafka cluster")
					}

					By("sending fixing offline replicas request to Cruise Control")
					req2 := api.FixOfflineReplicasRequestWithDefaults()
					req2.Reason = "integration testing"
					req2.DryRun = false
					resp2, err := cruisecontrol.FixOfflineReplicas(ctx, req2)
					Expect(err).NotTo(HaveOccurred())
					Expect(resp2.Failed()).To(BeFalse())

					By("waiting until Cruise Control fixes all the offline replicas")
					Eventually(ctx, func() bool {
						resp, err := cruisecontrol.KafkaClusterState(ctx, req)
						Expect(err).NotTo(HaveOccurred())
						Expect(resp.Failed()).To(BeFalse())
						state := resp.Result.KafkaPartitionState
						if len(state.Offline) > 0 || len(state.WithOfflineReplicas) > 0 {
							log.V(0).Info("Kafka cluster state",
								"offline partitions", len(state.Offline),
								"partitions with offline replicas", len(state.WithOfflineReplicas))
							return false
						}
						return true
					}, 300, 15).Should(BeTrue())
				})
			})
		})
	})
