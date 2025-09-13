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

var _ = Describe("Remove Disks",
	Label("api:remove_disks", "api:user_tasks", "api:kafka_cluster_load", "api:state"),
	Serial,
	func() {
		const (
			brokerID                              = 0
			logDir                                = "/var/lib/kafka/data0"
			pollIntervalSeconds                   = 15
			cruiseControlRemoveDiskTimeoutSeconds = 600
		)

		BeforeEach(func(ctx SpecContext) {
			By("waiting until Cruise Control is ready")
			Eventually(ctx, func() bool {
				ready, err := helpers.IsCruiseControlReady(ctx, cruisecontrol)
				Expect(err).NotTo(HaveOccurred())
				return ready
			}, CruiseControlReadyTimeout, pollIntervalSeconds).Should(BeTrue())
		})

		Describe("Removing a disk in Kafka cluster", func() {
			It("should return no error", func(ctx SpecContext) {
				By("sending a remove request to Cruise Control")
				req := &api.RemoveDisksRequest{}

				req.BrokerIDAndLogDirs = map[int32][]string{
					brokerID: {logDir},
				}
				req.Reason = "integration testing"

				resp, err := cruisecontrol.RemoveDisks(ctx, req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.Failed()).To(BeFalse())

				By("waiting until the remove task finished")
				Eventually(ctx, func() bool {
					finished, err := helpers.HasUserTaskFinished(ctx, cruisecontrol, resp.TaskID)
					Expect(err).NotTo(HaveOccurred())
					return finished
				}, cruiseControlRemoveDiskTimeoutSeconds, pollIntervalSeconds).Should(BeTrue())

				By("checking that the disk has been drained")
				req2 := api.KafkaClusterLoadRequestWithDefaults()
				req2.PopulateDiskInfo = true
				req2.Reason = "integration testing"

				resp2, err := cruisecontrol.KafkaClusterLoad(ctx, req2)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp2.Failed()).To(BeFalse())

				Expect(resp2.Result.Brokers).ToNot(BeEmpty())

				var affectedBroker types.BrokerLoadStats
				for _, broker := range resp2.Result.Brokers {
					if broker.Broker == brokerID {
						affectedBroker = broker
						break
					}
				}

				Expect(affectedBroker).ToNot(BeNil())

				var affectedDiskState types.DiskStats
				for logDir, state := range affectedBroker.DiskState {
					if logDir == logDir {
						affectedDiskState = state
						break
					}
				}

				Expect(affectedDiskState).ToNot(BeNil())

				replicas := affectedDiskState.NumReplicas
				log.V(0).Info("partition replicas on broker disk", "broker_id", brokerID, "logDir", logDir, "replicas", replicas)
				Expect(replicas).To(BeNumerically("==", 0))
			})
		})
	})
