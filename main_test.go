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

package main

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

type readinessTestManager struct {
	ctrl.Manager
	server webhook.Server
	readyz healthz.Handler
}

func (m *readinessTestManager) AddReadyzCheck(name string, check healthz.Checker) error {
	if m.readyz.Checks == nil {
		m.readyz.Checks = make(map[string]healthz.Checker)
	}
	m.readyz.Checks[name] = check
	return nil
}

func (m *readinessTestManager) GetWebhookServer() webhook.Server {
	if m.server == nil {
		panic("disabled webhooks must not get the webhook server")
	}
	return m.server
}

func (m *readinessTestManager) readinessStatus() int {
	response := httptest.NewRecorder()
	m.readyz.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	return response.Code
}

func TestWebhookReadinessTracksTLSListener(t *testing.T) {
	tlsServer := httptest.NewTLSServer(http.NotFoundHandler())
	certificate := tlsServer.TLS.Certificates[0]
	port := tlsServer.Listener.Addr().(*net.TCPAddr).Port
	tlsServer.Close()
	server := webhook.NewServer(webhook.Options{
		Host: "127.0.0.1",
		Port: port,
		TLSOpts: []func(*tls.Config){func(cfg *tls.Config) {
			cfg.GetCertificate = func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
				return &certificate, nil
			}
		}},
	})
	mgr := &readinessTestManager{server: server}
	require.NoError(t, addReadinessChecks(mgr, false))
	require.Equal(t, http.StatusInternalServerError, mgr.readinessStatus(), "ready before TLS serving started")

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- server.Start(ctx) }()
	require.Eventually(t, func() bool {
		return mgr.readinessStatus() == http.StatusOK
	}, 2*time.Second, 10*time.Millisecond, "unready after TLS serving started")

	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("webhook server did not stop")
	}
	require.Equal(t, http.StatusInternalServerError, mgr.readinessStatus(), "ready after TLS serving stopped")
}

func TestWebhookReadinessDisabled(t *testing.T) {
	mgr := &readinessTestManager{}
	require.NoError(t, addReadinessChecks(mgr, true))
	require.Equal(t, http.StatusOK, mgr.readinessStatus())
}

type slowWebhookChecker struct {
	webhook.Server
	release <-chan struct{}
	calls   atomic.Int32
}

func (s *slowWebhookChecker) StartedChecker() healthz.Checker {
	return func(*http.Request) error {
		s.calls.Add(1)
		<-s.release
		return nil
	}
}

func TestWebhookReadinessBoundsConcurrentProbes(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	server := &slowWebhookChecker{release: release}
	mgr := &readinessTestManager{server: server}
	require.NoError(t, addReadinessChecks(mgr, false))

	statuses := make(chan int, 2)
	for range 2 {
		go func() { statuses <- mgr.readinessStatus() }()
	}
	timeout := time.After(time.Second)
	for range 2 {
		select {
		case status := <-statuses:
			require.Equal(t, http.StatusInternalServerError, status)
		case <-timeout:
			t.Fatal("readiness probe did not return while the TLS check was blocked")
		}
	}
	require.Equal(t, int32(1), server.calls.Load(), "overlapping probes ran multiple TLS checks")
}

func TestRESTRequestMetricsAreExposed(t *testing.T) {
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"apiVersion":"v1","kind":"NamespaceList","items":[]}`))
	}))
	defer apiServer.Close()

	client, err := kubernetes.NewForConfig(&rest.Config{Host: apiServer.URL})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.CoreV1().Namespaces().List(context.Background(), metav1.ListOptions{}); err != nil {
		t.Fatal(err)
	}

	families, err := metrics.Registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"rest_client_request_duration_seconds", "rest_client_response_size_bytes"} {
		var observed bool
		for _, family := range families {
			if family.GetName() != name {
				continue
			}
			for _, metric := range family.GetMetric() {
				if metric.GetHistogram().GetSampleCount() > 0 {
					observed = true
				}
			}
		}
		if !observed {
			t.Errorf("%s has no observed REST request", name)
		}
	}
}
