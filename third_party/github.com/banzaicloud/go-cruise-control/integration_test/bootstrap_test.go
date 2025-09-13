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
	"time"

	"github.com/banzaicloud/go-cruise-control/integration_test/helpers"
	"github.com/banzaicloud/go-cruise-control/pkg/api"
	"github.com/banzaicloud/go-cruise-control/pkg/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Bootstrap", Label("api:bootstrap", "api:state"), func() {

	BeforeEach(func(ctx SpecContext) {
		By("waiting until Cruise Control is ready")
		Eventually(ctx, func() bool {
			ready, err := helpers.IsCruiseControlReady(ctx, cruisecontrol)
			Expect(err).NotTo(HaveOccurred())
			return ready
		}, CruiseControlReadyTimeout, 15).Should(BeTrue())
	})

	Describe("Bootstrapping Cruise Control", func() {
		Context("with metrics data from the last hour", func() {
			It("should return no error", func(ctx SpecContext) {
				By("sending a bootstrap request to Cruise Control")
				req := api.BootstrapRequestWithDefaults()
				req.Start = time.Now().UTC().Add(-1 * time.Hour).UnixMilli()
				req.End = time.Now().UTC().UnixMilli()

				resp, err := cruisecontrol.Bootstrap(ctx, req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.Failed()).To(BeFalse())

				By("waiting until monitor state is changed to bootstrapping")
				Eventually(ctx, func() bool {
					req2 := api.StateRequestWithDefaults()
					req2.Substates = []types.Substate{
						types.SubstateMonitor,
					}

					resp2, err := cruisecontrol.State(ctx, req2)
					Expect(err).NotTo(HaveOccurred())
					Expect(resp2.Failed()).To(BeFalse())
					state := resp2.Result.MonitorState.State
					log.V(0).Info("cruise control monitor state", "state", state)

					return state == types.MonitorStateBootstrapping
				}, 300, 15).Should(BeTrue())

				By("waiting until monitor state is changed back to running")
				Eventually(ctx, func() bool {
					req2 := api.StateRequestWithDefaults()
					req2.Substates = []types.Substate{
						types.SubstateMonitor,
					}

					resp2, err := cruisecontrol.State(ctx, req2)
					Expect(err).NotTo(HaveOccurred())
					Expect(resp2.Failed()).To(BeFalse())
					state := resp2.Result.MonitorState.State
					log.V(0).Info("cruise control monitor state", "state", state)

					return state == types.MonitorStateRunning
				}, 300, 15).Should(BeTrue())
			})
		})
	})
})
