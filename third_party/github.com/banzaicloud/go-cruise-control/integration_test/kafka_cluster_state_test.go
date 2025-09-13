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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Kafka Cluster State", Label("api:kafka_cluster_state", "api:state"), func() {

	var (
		expectedBrokers = map[int32]struct {
			ID   int32
			Host string
			Rack string
		}{
			0: {
				0,
				"kafka-0",
				"rack-0",
			},
			1: {
				1,
				"kafka-1",
				"rack-1",
			},
			2: {
				2,
				"kafka-2",
				"rack-2",
			},
		}
	)

	BeforeEach(func(ctx SpecContext) {
		By("waiting until Cruise Control is ready")
		Eventually(ctx, func() bool {
			ready, err := helpers.IsCruiseControlReady(ctx, cruisecontrol)
			Expect(err).NotTo(HaveOccurred())
			return ready
		}, CruiseControlReadyTimeout, 15).Should(BeTrue())
	})

	Describe("Getting broker/partition state from Cruise Control", func() {
		Context("including all topics and with default verbosity", func() {
			It("should result no errors", func(ctx SpecContext) {
				By("requesting cluster state information")
				req := api.KafkaClusterStateRequestWithDefaults()
				resp, err := cruisecontrol.KafkaClusterState(ctx, req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.Failed()).To(BeFalse())

				By("getting brokers state")
				brokerState := resp.Result.KafkaBrokerState
				numOfExpectedBrokers := len(expectedBrokers)
				Expect(brokerState.Summary.Brokers).To(BeNumerically("==", numOfExpectedBrokers))
				Expect(brokerState.OnlineLogDirsByBrokerID).To(HaveLen(numOfExpectedBrokers))
				Expect(brokerState.ReplicaCountByBrokerID).To(HaveLen(numOfExpectedBrokers))
				Expect(brokerState.IsController).To(HaveLen(numOfExpectedBrokers))

				By("getting partitions state")
				partitionState := resp.Result.KafkaPartitionState
				Expect(partitionState.Offline).To(HaveLen(0))
				Expect(partitionState.WithOfflineReplicas).To(HaveLen(0))
				Expect(partitionState.UnderReplicatedPartitions).To(HaveLen(0))
				Expect(partitionState.UnderMinISR).To(HaveLen(0))
			})
		})
	})

	Describe("Getting broker/partition state from Cruise Control", func() {
		Context("for airports topic with increased verbosity", func() {
			It("should result no errors", func(ctx SpecContext) {
				By("requesting cluster state information")
				req := api.KafkaClusterStateRequestWithDefaults()
				req.Topic = AirPortsTopicName
				req.Verbose = true
				resp, err := cruisecontrol.KafkaClusterState(ctx, req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.Failed()).To(BeFalse())

				By("getting brokers state")
				brokerState := resp.Result.KafkaBrokerState
				numOfExpectedBrokers := len(expectedBrokers)
				Expect(brokerState.Summary.Brokers).To(BeNumerically("==", numOfExpectedBrokers))
				Expect(brokerState.OnlineLogDirsByBrokerID).To(HaveLen(numOfExpectedBrokers))
				Expect(brokerState.ReplicaCountByBrokerID).To(HaveLen(numOfExpectedBrokers))
				Expect(brokerState.IsController).To(HaveLen(numOfExpectedBrokers))

				By("getting partitions state")
				partitionState := resp.Result.KafkaPartitionState
				Expect(partitionState.Offline).To(HaveLen(0))
				Expect(partitionState.WithOfflineReplicas).To(HaveLen(0))
				Expect(partitionState.UnderReplicatedPartitions).To(HaveLen(0))
				Expect(partitionState.UnderMinISR).To(HaveLen(0))
				Expect(partitionState.Other).To(Or(HaveLen(AirportsTopicOldPartitionSize),
					HaveLen(AirportsTopicNewPartitionSize)))
			})
		})
	})
})
