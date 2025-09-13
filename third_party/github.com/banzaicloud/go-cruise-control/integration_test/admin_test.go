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

var _ = Describe("Admin", Label("api:admin", "api:state"), Ordered, func() {

	BeforeEach(func(ctx SpecContext) {
		By("waiting until Cruise Control is ready")
		Eventually(ctx, func() bool {
			ready, err := helpers.IsCruiseControlReady(ctx, cruisecontrol)
			Expect(err).NotTo(HaveOccurred())
			return ready
		}, CruiseControlReadyTimeout, 15).Should(BeTrue())
	})

	Describe("Updating Cruise Control configuration", func() {

		var (
			anomalies = []types.AnomalyType{
				types.AnomalyTypeDiskFailure,
				types.AnomalyTypeBrokerFailure,
				types.AnomalyTypeGoalViolation,
				types.AnomalyTypeMetricAnomaly,
				types.AnomalyTypeTopicAnomaly,
				types.AnomalyTypeMaintenanceEvent,
			}
		)

		Context("to enable self-healing for all anomaly types", func() {
			It("successfully", func(ctx SpecContext) {
				By("sending the request")
				req := api.AdminRequestWithDefaults()
				req.Reason = "integration testing"
				req.EnableSelfHealingFor = anomalies

				resp, err := cruisecontrol.Admin(ctx, req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.Failed()).To(BeFalse())

				By("checking that self-healing is enabled for all anomaly types")
				for _, anomaly := range anomalies {
					enabled, ok := resp.Result.SelfHealingEnabledAfter[anomaly]
					Expect(ok).To(BeTrue())
					Expect(enabled).To(BeTrue())
				}
			})
		})

		Context("to disable self-healing for all anomaly types", func() {
			It("should result no errors", func(ctx SpecContext) {
				By("sending the request")
				req := api.AdminRequestWithDefaults()
				req.Reason = "integration testing"
				req.DisableSelfHealingFor = anomalies

				resp, err := cruisecontrol.Admin(ctx, req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.Failed()).To(BeFalse())

				By("checking that self-healing is disabled for all anomaly types")
				for _, anomaly := range anomalies {
					enabled, ok := resp.Result.SelfHealingEnabledAfter[anomaly]
					Expect(ok).To(BeTrue())
					Expect(enabled).To(BeFalse())
				}
			})
		})
	})
})
