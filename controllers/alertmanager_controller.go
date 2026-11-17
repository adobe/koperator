// Copyright © 2019 Cisco Systems, Inc. and/or its affiliates
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

package controllers

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/banzaicloud/koperator/internal/alertmanager"
	"github.com/banzaicloud/koperator/pkg/util"
)

const (
	receiverAddr = ":9001"
)

// AController implements Runnable
type AController struct {
	Client client.Client
}

// SetAlertManagerWithManager creates a new Alertmanager Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func SetAlertManagerWithManager(mgr manager.Manager) error {
	return mgr.Add(AController{Client: mgr.GetClient()})
}

// Start initiates the alertmanager controller
func (c AController) Start(ctx context.Context) error {
	logf.SetLogger(util.CreateLogger(false, false))
	log := logf.Log.WithName("alertmanager")

	ln, err := net.Listen("tcp", receiverAddr)
	if err != nil {
		return fmt.Errorf("listen for alerts: %w", err)
	}
	return serveAlerts(ctx, ln, alertmanager.NewApp(log, c.Client), 5*time.Second)
}

func serveAlerts(ctx context.Context, ln net.Listener, handler http.Handler, shutdownTimeout time.Duration) error {
	httpServer := &http.Server{Handler: handler}
	defer httpServer.Close()
	defer ln.Close()

	serveDone := make(chan error, 1)
	go func() { serveDone <- httpServer.Serve(ln) }()
	select {
	case err := <-serveDone:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		shutdownErr := httpServer.Shutdown(shutdownCtx)
		serveErr := <-serveDone
		if errors.Is(serveErr, http.ErrServerClosed) {
			serveErr = nil
		}
		return errors.Join(shutdownErr, serveErr)
	}
}
