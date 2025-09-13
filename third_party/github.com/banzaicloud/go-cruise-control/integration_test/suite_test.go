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
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"

	"github.com/banzaicloud/go-cruise-control/integration_test/envtest"
	"github.com/banzaicloud/go-cruise-control/integration_test/helpers"
	"github.com/banzaicloud/go-cruise-control/pkg/client"
	"github.com/compose-spec/compose-go/cli"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	CruiseControlReadyTimeout = 600

	AirportsTopicNewPartitionSize     = 35
	AirportsTopicOldPartitionSize     = 30
	AirPortsTopicName                 = "airports"
	AirportsTopicNewReplicationFactor = 3
	AirportsTopicOldReplicationFactor = 2
)

var (
	testEnv       *envtest.Environment
	log           logr.Logger
	logSyncFn     func() error
	cruisecontrol *client.Client
)

func TestCruiseControlClient(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Run integration tests")
}

var _ = BeforeSuite(func(ctx context.Context) {
	var err error

	By("creating test environment")
	log, logSyncFn, err = envtest.NewLogger(GinkgoWriter)
	ctx = logr.NewContext(ctx, log)

	opts, err := cli.NewProjectOptions(
		[]string{"../deploy/docker-compose.yml"},
		cli.WithName("go-cruise-control"),
		cli.WithWorkingDirectory("../deploy"),
		cli.WithResolvedPaths(true),
		cli.WithProfiles([]string{""}),
	)
	Expect(err).NotTo(HaveOccurred())

	err = cli.WithOsEnv(opts)
	Expect(err).NotTo(HaveOccurred())

	var reuseEnv bool
	if reuseEnv, _ = strconv.ParseBool(os.Getenv("USE_EXISTING")); reuseEnv {
		log.V(-1).Info("reusing existing environment...", "use_existing", reuseEnv)
	}

	testEnv, err = envtest.New(opts, reuseEnv)
	Expect(err).NotTo(HaveOccurred())

	By("starting test environment")
	err = testEnv.Start(ctx)
	Expect(err).NotTo(HaveOccurred())

	// Wait until the test environment is ready
	Eventually(func() bool {
		ready, err := testEnv.Ready(ctx)
		Expect(err).NotTo(HaveOccurred())
		return ready
	}, 15, 12).Should(BeTrue())

	// Get Cruise Control URL
	u, err := testEnv.CruiseControlURL()
	Expect(err).NotTo(HaveOccurred())

	// Setup Cruise Control API client
	cfg := &client.Config{
		ServerURL: u.String(),
	}
	cruisecontrol, err = client.NewClient(cfg)
	Expect(err).NotTo(HaveOccurred())

	By("waiting until Cruise Control is ready")
	Eventually(func() bool {
		ready, err := helpers.IsCruiseControlReady(ctx, cruisecontrol)
		Expect(err).NotTo(HaveOccurred())
		return ready
	}, CruiseControlReadyTimeout, 15).Should(BeTrue())
})

var _ = AfterSuite(func(ctx context.Context) {
	By("tearing down test environment")
	err := testEnv.Stop(ctx)
	Expect(err).NotTo(HaveOccurred())
	defer func(fn func() error) {
		err := fn()
		if err != nil {
			fmt.Printf("calling sync on logger failed: %v\n", err)
		}
	}(logSyncFn)
})
