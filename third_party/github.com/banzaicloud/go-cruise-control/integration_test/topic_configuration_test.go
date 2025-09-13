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

var _ = Describe("Topic Configuration", Label("api:topic_configuration", "api:user_tasks", "api:state"),
	Ordered,
	func() {

		BeforeEach(func(ctx SpecContext) {
			By("waiting until Cruise Control is ready")
			Eventually(ctx, func() bool {
				ready, err := helpers.IsCruiseControlReady(ctx, cruisecontrol)
				Expect(err).NotTo(HaveOccurred())
				return ready
			}, CruiseControlReadyTimeout, 15).Should(BeTrue())
		})

		AfterAll(func(ctx SpecContext) {
			By("sending topic configuration request to Cruise Control")
			req := api.TopicConfigurationRequestWithDefaults()
			req.Topic = AirPortsTopicName
			req.ReplicationFactor = AirportsTopicOldReplicationFactor
			req.DryRun = false

			resp, err := cruisecontrol.TopicConfiguration(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Failed()).To(BeFalse())

			By("waiting until the rightsize task finished")
			Eventually(ctx, func() bool {
				finished, err := helpers.HasUserTaskFinished(ctx, cruisecontrol, resp.TaskID)
				Expect(err).NotTo(HaveOccurred())
				return finished
			}, 300, 15).Should(BeTrue())
		})

		Describe("Updating topic configuration in Kafka cluster", func() {
			Context("by increasing it's replication factor", func() {
				It("should return no error", func(ctx SpecContext) {
					By("sending topic configuration request to Cruise Control")
					req := api.TopicConfigurationRequestWithDefaults()
					req.Topic = AirPortsTopicName
					req.ReplicationFactor = AirportsTopicNewReplicationFactor
					req.DryRun = false

					resp, err := cruisecontrol.TopicConfiguration(ctx, req)
					Expect(err).NotTo(HaveOccurred())
					Expect(resp.Failed()).To(BeFalse())

					By("waiting until the topic configuration task finished")
					Eventually(ctx, func() bool {
						finished, err := helpers.HasUserTaskFinished(ctx, cruisecontrol, resp.TaskID)
						Expect(err).NotTo(HaveOccurred())
						return finished
					}, 300, 15).Should(BeTrue())

					By("waiting until Cruise Control analyzer is ready")
					Eventually(ctx, func() bool {
						ready, err := helpers.IsCruiseControlReady(ctx, cruisecontrol)
						Expect(err).NotTo(HaveOccurred())
						return ready
					}, CruiseControlReadyTimeout, 15).Should(BeTrue())
				})
			})
		})
	})
