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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/banzaicloud/go-cruise-control/integration_test/helpers"
	"github.com/banzaicloud/go-cruise-control/pkg/api"
)

var _ = Describe("Kafka Cluster Load", Label("api:load", "api:state"), func() {

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

		expectedHosts = map[string]struct {
			Host string
			Rack string
		}{
			"kafka-0": {
				"kafka-0",
				"rack-0",
			},
			"kafka-1": {
				"kafka-1",
				"rack-1",
			},
			"kafka-2": {
				"kafka-2",
				"rack-2",
			},
		}

		expectedBrokerDisks = []string{
			"/var/lib/kafka/data0",
			"/var/lib/kafka/data1",
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

	Describe("Getting server/broker stats from Cruise Control", func() {
		Context("for the last hour", func() {
			It("should result no errors", func(ctx SpecContext) {
				By("requesting load information")
				req := &api.KafkaClusterLoadRequest{
					AllowCapacityEstimation: true,
					PopulateDiskInfo:        true,
				}
				resp, err := cruisecontrol.KafkaClusterLoad(ctx, req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.Failed()).To(BeFalse())

				By("getting broker load statistics")
				brokerStats := resp.Result.Brokers
				Expect(brokerStats).To(HaveLen(len(expectedBrokers)))
				for _, broker := range brokerStats {
					log.V(0).Info("broker stats", "id", broker.Broker, "stats", broker)
					expectedBroker, ok := expectedBrokers[broker.Broker]
					Expect(ok).To(BeTrue())
					Expect(broker.Host).To(Equal(expectedBroker.Host))
					Expect(broker.Rack).To(Equal(expectedBroker.Rack))

					for _, disk := range expectedBrokerDisks {
						_, ok := broker.DiskState[disk]
						Expect(ok).To(BeTrue())
					}
				}

				By("getting host load statistics")
				hostStats := resp.Result.Hosts
				Expect(hostStats).To(HaveLen(len(expectedHosts)))
				for _, host := range hostStats {
					log.V(0).Info("host stats", "host", host.Host, "stats", host)
					expectedBroker, ok := expectedHosts[host.Host]
					Expect(ok).To(BeTrue())
					Expect(host.Host).To(Equal(expectedBroker.Host))
					Expect(host.Rack).To(Equal(expectedBroker.Rack))
				}
			})
		})
	})
})
