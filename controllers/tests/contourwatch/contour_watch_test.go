// Copyright 2026 Cisco Systems, Inc. and/or its affiliates
// Copyright 2026 Adobe. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package contourwatch contains a focused regression test for
// https://github.com/adobe/koperator/issues/229: the operator must not
// require Project Contour's HTTPProxy CRD unless Contour ingress is enabled.
//
// The KafkaCluster controller Owns(&contour.HTTPProxy{}); if that watch is
// registered while the CRD is absent, controller-runtime never syncs the
// informer, mgr.Start returns an error and the operator pod CrashLoopBackOffs.
// This test boots an envtest API server WITHOUT the Contour CRD and asserts
// the manager stays healthy when Contour is disabled.
package contourwatch

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	k8sscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"

	contour "github.com/projectcontour/contour/apis/projectcontour/v1"

	banzaicloudv1beta1 "github.com/banzaicloud/koperator/api/v1beta1"
	"github.com/banzaicloud/koperator/controllers"
	"github.com/banzaicloud/koperator/pkg/kafkaclient"
)

const (
	// cacheSyncTimeout bounds how long a controller waits for its informers to
	// sync. When the Contour watch is (incorrectly) active without the CRD, the
	// manager reports the sync failure after this timeout.
	cacheSyncTimeout = 15 * time.Second
	// healthyGrace must exceed cacheSyncTimeout: if the manager survives this
	// long without exiting, its caches synced, meaning the HTTPProxy watch was
	// not registered.
	healthyGrace = 20 * time.Second
)

// TestContourDisabledManagerStartsWithoutContourCRD reproduces issue #229: with
// Contour disabled and the projectcontour HTTPProxy CRD absent, the manager must
// start and keep running instead of crash-looping on a never-syncing informer.
func TestContourDisabledManagerStartsWithoutContourCRD(t *testing.T) {
	testEnv := &envtest.Environment{
		ErrorIfCRDPathMissing: true,
		// Deliberately install only Koperator's own CRDs — no projectcontour —
		// to mirror a cluster that does not run Project Contour.
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "config", "base", "crds"),
		},
	}

	cfg, err := testEnv.Start()
	if err != nil {
		t.Fatalf("failed to start envtest control plane: %v", err)
	}
	t.Cleanup(func() {
		if stopErr := testEnv.Stop(); stopErr != nil {
			t.Logf("failed to stop envtest control plane: %v", stopErr)
		}
	})

	scheme := runtime.NewScheme()
	if err := k8sscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add client-go scheme: %v", err)
	}
	if err := banzaicloudv1beta1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add koperator scheme: %v", err)
	}
	// The Contour types are registered on the scheme exactly as in main.go, so
	// the only thing that differs between the crash-looping and fixed behavior
	// is whether the HTTPProxy watch is wired up.
	if err := contour.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add contour scheme: %v", err)
	}

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:         scheme,
		Metrics:        server.Options{BindAddress: "0"},
		LeaderElection: false,
		Controller:     config.Controller{CacheSyncTimeout: cacheSyncTimeout},
	})
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	reconciler := &controllers.KafkaClusterReconciler{
		Client:              mgr.GetClient(),
		DirectClient:        mgr.GetAPIReader(),
		KafkaClientProvider: kafkaclient.NewMockProvider(),
	}

	const contourEnabled = false
	if err := controllers.SetupKafkaClusterWithManager(mgr, contourEnabled).Complete(reconciler); err != nil {
		t.Fatalf("failed to set up KafkaCluster controller: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	startErr := make(chan error, 1)
	go func() {
		startErr <- mgr.Start(ctx)
	}()

	select {
	case err := <-startErr:
		t.Fatalf("manager exited before becoming healthy (regression of issue #229 — "+
			"the Contour HTTPProxy watch is active without the Contour CRD installed): %v", err)
	case <-time.After(healthyGrace):
		// Manager survived past the cache-sync timeout: informers synced, so the
		// HTTPProxy watch was correctly skipped while Contour is disabled.
	}

	cancel()
	if err := <-startErr; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("manager Start returned an unexpected error on shutdown: %v", err)
	}
}
