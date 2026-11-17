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
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

type webhookReadyTestServer struct {
	webhook.Server
	err error
}

func (s *webhookReadyTestServer) StartedChecker() healthz.Checker {
	return func(*http.Request) error { return s.err }
}

type webhookReadyTestManager struct {
	ctrl.Manager
	server                              *webhookReadyTestServer
	check                               healthz.Checker
	serverRequests, healthRegistrations int
}

func (m *webhookReadyTestManager) GetWebhookServer() webhook.Server {
	m.serverRequests++
	return m.server
}

func (m *webhookReadyTestManager) AddReadyzCheck(_ string, check healthz.Checker) error {
	m.check = check
	return nil
}

func (m *webhookReadyTestManager) AddHealthzCheck(string, healthz.Checker) error {
	m.healthRegistrations++
	return nil
}

func TestWebhookReadinessConditionalRegistration(t *testing.T) {
	for _, disabled := range []bool{false, true} {
		mgr := &webhookReadyTestManager{server: &webhookReadyTestServer{err: errors.New("not serving TLS")}}
		if err := addWebhookReadyzCheck(mgr, disabled); err != nil {
			t.Fatal(err)
		}
		if mgr.healthRegistrations != 0 {
			t.Fatal("serving readiness changed liveness registration")
		}
		if disabled {
			if mgr.serverRequests != 0 || mgr.check != nil {
				t.Fatal("disabled webhooks started or registered a TLS server")
			}
			continue
		}
		if mgr.check == nil || mgr.serverRequests != 1 {
			t.Fatal("enabled webhook serving readiness was not registered")
		}
		if err := mgr.check(httptest.NewRequest(http.MethodGet, "/readyz", nil)); err == nil {
			t.Fatal("unusable webhook standby reported ready")
		}
		mgr.server.err = nil
		if err := mgr.check(httptest.NewRequest(http.MethodGet, "/readyz", nil)); err != nil {
			t.Fatalf("usable webhook standby is not ready: %v", err)
		}
	}
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
