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
	"os"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/banzaicloud/koperator/internal/alertmanager"
	"github.com/banzaicloud/koperator/pkg/util"
)

const (
	receiverAddr     = ":9001"
	alertLeaderLabel = "kafka.banzaicloud.io/alert-leader"
)

// AController implements Runnable
type AController struct {
	Client  client.Client
	reader  client.Reader
	elected <-chan struct{}
	state   *alertServingState
	pod     types.NamespacedName
	uid     types.UID
}

type alertServingState struct {
	sync.RWMutex
	started bool
	ready   bool
	ctx     context.Context
}

// SetAlertManagerWithManager creates a new Alertmanager Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func SetAlertManagerWithManager(mgr manager.Manager) error {
	name, uid, namespace := os.Getenv("POD_NAME"), os.Getenv("POD_UID"), os.Getenv("POD_NAMESPACE")
	if (name == "") != (uid == "") || (name != "" && namespace == "") {
		return errors.New("managed alert routing requires POD_NAMESPACE, POD_NAME and POD_UID together")
	}
	c := &AController{
		Client: mgr.GetClient(), reader: mgr.GetAPIReader(), elected: mgr.Elected(),
		state: &alertServingState{}, pod: types.NamespacedName{Namespace: namespace, Name: name}, uid: types.UID(uid),
	}
	if err := mgr.Add(c); err != nil {
		return err
	}
	return mgr.AddReadyzCheck("alerts", c.readyz)
}

// Start initiates the alertmanager controller
func (c AController) Start(ctx context.Context) error {
	logf.SetLogger(util.CreateLogger(false, false))
	log := logf.Log.WithName("alertmanager")

	ln, err := net.Listen("tcp", receiverAddr)
	if err != nil {
		return fmt.Errorf("listen for alerts: %w", err)
	}
	return c.run(ctx, ln, alertmanager.NewApp(log, c.Client))
}

func (c AController) readyz(_ *http.Request) error {
	if c.state == nil {
		return errors.New("alert serving state is unavailable")
	}
	c.state.RLock()
	defer c.state.RUnlock()
	if !c.state.started {
		select {
		case <-c.elected:
			return errors.New("elected alert listener has not started")
		default:
			return nil
		}
	}
	if !c.state.ready || c.state.ctx.Err() != nil {
		return errors.New("elected alert listener is not ready")
	}
	return nil
}

func (c AController) run(ctx context.Context, ln net.Listener, handler http.Handler) (err error) {
	if c.state == nil {
		c.state = &alertServingState{}
	}
	c.state.Lock()
	c.state.started, c.state.ready, c.state.ctx = true, false, ctx
	c.state.Unlock()
	var scope client.MatchingLabels
	defer func() {
		c.state.Lock()
		c.state.ready = false
		c.state.Unlock()
		_ = ln.Close()
		if len(scope) != 0 {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if cleanupErr := c.updateRouteLabel(cleanupCtx, c.pod, c.uid, scope, false); cleanupErr != nil {
				err = errors.Join(err, fmt.Errorf("remove alert routing label: %w", cleanupErr))
			}
		}
	}()
	if c.pod.Name != "" {
		scope, err = c.prepareRouting(ctx)
		if err != nil {
			return err
		}
	}
	c.state.Lock()
	c.state.ready = true
	c.state.Unlock()
	return serveAlerts(ctx, ln, c.gatedHandler(ctx, handler), 5*time.Second)
}

func (c AController) gatedHandler(ctx context.Context, handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.state.RLock()
		ready := c.state.ready
		c.state.RUnlock()
		if ctx.Err() != nil || !ready {
			http.Error(w, "alert listener is not accepting requests", http.StatusServiceUnavailable)
			return
		}
		handler.ServeHTTP(w, r)
	})
}

func (c AController) prepareRouting(ctx context.Context) (client.MatchingLabels, error) {
	self := &corev1.Pod{}
	if err := c.reader.Get(ctx, c.pod, self); err != nil {
		return nil, fmt.Errorf("read alert listener Pod: %w", err)
	}
	if self.UID != c.uid {
		return nil, errors.New("alert listener Pod UID does not match POD_UID")
	}
	scope := client.MatchingLabels{}
	for _, key := range []string{
		"control-plane", "controller-tools.k8s.io", "app.kubernetes.io/name",
		"app.kubernetes.io/instance", "app.kubernetes.io/component",
	} {
		if self.Labels[key] == "" {
			return nil, fmt.Errorf("alert listener Pod requires installation label %s", key)
		}
		scope[key] = self.Labels[key]
	}
	if scope["control-plane"] != "controller-manager" || scope["controller-tools.k8s.io"] != "1.0" ||
		scope["app.kubernetes.io/component"] != "operator" {
		return nil, errors.New("alert listener Pod labels do not match the alert Service selector")
	}
	pods := &corev1.PodList{}
	if err := c.reader.List(ctx, pods, client.InNamespace(c.pod.Namespace), scope); err != nil {
		return scope, fmt.Errorf("list alert routing Pods: %w", err)
	}
	for i := range pods.Items {
		pod := &pods.Items[i]
		if pod.Labels[alertLeaderLabel] == True {
			if err := c.updateRouteLabel(ctx, client.ObjectKeyFromObject(pod), pod.UID, scope, false); err != nil {
				return scope, fmt.Errorf("clear stale alert routing label: %w", err)
			}
		}
	}
	if err := c.updateRouteLabel(ctx, c.pod, c.uid, scope, true); err != nil {
		return scope, fmt.Errorf("publish alert routing label: %w", err)
	}
	return scope, nil
}

func (c AController) updateRouteLabel(ctx context.Context, key client.ObjectKey, uid types.UID, scope client.MatchingLabels, active bool) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		pod := &corev1.Pod{}
		if err := c.reader.Get(ctx, key, pod); err != nil {
			if !active && apierrors.IsNotFound(err) {
				return nil
			}
			return err
		}
		matches := pod.UID == uid
		for label, value := range scope {
			matches = matches && pod.Labels[label] == value
		}
		if !matches {
			if !active {
				return nil
			}
			return errors.New("alert routing Pod identity or installation labels changed")
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if active {
			if pod.DeletionTimestamp != nil {
				return errors.New("cannot route alerts to a terminating Pod")
			}
			pod.Labels[alertLeaderLabel] = True
		} else {
			if pod.Labels[alertLeaderLabel] != True {
				return nil
			}
			delete(pod.Labels, alertLeaderLabel)
		}
		return c.Client.Update(ctx, pod)
	})
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
