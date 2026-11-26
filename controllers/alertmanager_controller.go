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
	"fmt"
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
	return c.start(ctx, receiverAddr)
}

func (c AController) start(ctx context.Context, addr string) error {
	logf.SetLogger(util.CreateLogger(false, false))
	log := logf.Log.WithName("alertmanager")

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	httpServer := &http.Server{Addr: addr, Handler: alertmanager.NewApp(log, c.Client)}
	defer httpServer.Close()
	shutdownTimeout := 5 * time.Second
	server := manager.Server{Name: "alertmanager", Server: httpServer, ShutdownTimeout: &shutdownTimeout}
	if err := server.Start(ctx); err != nil {
		return fmt.Errorf("serve alerts: %w", err)
	}
	return nil
}
