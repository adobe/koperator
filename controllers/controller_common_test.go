// Copyright © 2019 Cisco Systems, Inc. and/or its affiliates
// Copyright 2025 Adobe. All rights reserved.
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
	"io"
	"net"
	"net/http"
	"reflect"
	"sync"
	"testing"
	"time"

	emperrors "emperror.dev/errors"
	//nolint:staticcheck
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/banzaicloud/koperator/api/v1alpha1"
	"github.com/banzaicloud/koperator/api/v1beta1"
	"github.com/banzaicloud/koperator/pkg/errorfactory"
	"github.com/banzaicloud/koperator/pkg/kafkaclient"
)

var log = logf.Log.WithName("controller_testing")

const (
	testNamespace = "test-namespace"
)

func TestAlertManagerReportsBindError(t *testing.T) {
	listener, err := net.Listen("tcp", receiverAddr)
	if err != nil {
		t.Skipf("cannot reserve alert listener for bind-error regression: %v", err)
	}
	defer listener.Close()

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Errorf("Start panicked instead of returning the bind error: %v", recovered)
		}
	}()

	if err := (AController{}).Start(context.Background()); err == nil {
		t.Error("Start returned nil for an occupied alert listener")
	}
}

func TestAlertServerLifecycle(t *testing.T) {
	for _, tc := range []struct {
		name        string
		timeout     time.Duration
		finish      bool
		serveError  bool
		expectedErr error
	}{
		{name: "drains in-flight request", timeout: 5 * time.Second, finish: true},
		{name: "bounds shutdown and closes connection", timeout: 50 * time.Millisecond, expectedErr: context.DeadlineExceeded},
		{name: "returns serve error and closes connection", serveError: true, expectedErr: net.ErrClosed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			started, stopped, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
			finish := sync.OnceFunc(func() { close(release) })
			defer finish()
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer close(stopped)
				close(started)
				select {
				case <-release:
					_, _ = w.Write([]byte("drained"))
				case <-r.Context().Done():
				}
			})
			serverDone := make(chan error, 1)
			go func() { serverDone <- serveAlerts(ctx, listener, handler, tc.timeout) }()
			requestDone := make(chan error, 1)
			go func() {
				httpClient := &http.Client{Timeout: 5 * time.Second}
				response, requestErr := httpClient.Get("http://" + listener.Addr().String())
				if requestErr == nil {
					defer response.Body.Close()
					body, readErr := io.ReadAll(response.Body)
					requestErr = readErr
					if requestErr == nil && (response.StatusCode != http.StatusOK || string(body) != "drained") {
						requestErr = fmt.Errorf("unexpected response: status=%d body=%q", response.StatusCode, body)
					}
				}
				requestDone <- requestErr
			}()
			select {
			case <-started:
			case <-time.After(2 * time.Second):
				t.Fatal("alert handler did not start")
			}
			if tc.serveError {
				if err := listener.Close(); err != nil {
					t.Fatal(err)
				}
			} else {
				cancel()
			}
			if tc.finish {
				select {
				case err := <-serverDone:
					t.Fatalf("server returned before draining the request: %v", err)
				case <-time.After(25 * time.Millisecond):
				}
				finish()
			}
			select {
			case err := <-serverDone:
				if !errors.Is(err, tc.expectedErr) {
					t.Fatalf("server error = %v, want %v", err, tc.expectedErr)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("alert server did not stop")
			}
			select {
			case <-stopped:
			case <-time.After(2 * time.Second):
				t.Fatal("alert handler connection was not closed")
			}
			select {
			case err := <-requestDone:
				if (err == nil) != tc.finish {
					t.Fatalf("request error = %v, want successful drain = %t", err, tc.finish)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("alert request did not finish")
			}
			rebound, err := net.Listen("tcp", listener.Addr().String())
			if err != nil {
				t.Fatalf("alert listener was not released: %v", err)
			}
			_ = rebound.Close()
		})
	}
}

func TestRequeueAfter(t *testing.T) {
	sec := 5
	res, err := requeueAfter(sec)
	if err != nil {
		t.Error("Expected to get nil but got error")
	}
	if res.IsZero() {
		t.Error("Result must not be zero")
	}
	if res.RequeueAfter.Seconds() != float64(sec) {
		t.Error("Mismatch in time to set for requeue")
	}
}

func TestRequeueWithError(t *testing.T) {
	_, err := requeueWithError(log, "test", errors.New("test error"))
	if err == nil {
		t.Error("Expected error to fall through, got nil")
	}
}

func TestReconciled(t *testing.T) {
	res, err := reconciled()
	if err != nil {
		t.Error("Expected error to be nil, got:", err)
	}
	if res.RequeueAfter > 0 {
		t.Error("Expected requeue to be false, got true")
	}
}

func TestGetClusterRefNamespace(t *testing.T) {
	ns := testNamespace
	ref := v1alpha1.ClusterReference{
		Name: "test-cluster",
	}
	if refNS := getClusterRefNamespace(ns, ref); refNS != testNamespace {
		t.Error("Expected to get 'test-namespace', got:", refNS)
	}
	ref.Namespace = "another-namespace"
	if refNS := getClusterRefNamespace(ns, ref); refNS != "another-namespace" {
		t.Error("Expected to get 'another-namespace', got:", refNS)
	}
}

func TestClusterLabelString(t *testing.T) {
	cluster := &v1beta1.KafkaCluster{}
	cluster.Name = "test-cluster"
	cluster.Namespace = testNamespace
	if label := clusterLabelString(cluster); label != "test-cluster.test-namespace" {
		t.Error("Expected label value 'test-cluster.test-namespace', got:", label)
	}
}

func TestNewBrokerConnection(t *testing.T) {
	cluster := &v1beta1.KafkaCluster{}
	cluster.Name = "test-kafka"
	cluster.Namespace = testNamespace
	cluster.Spec = v1beta1.KafkaClusterSpec{
		ListenersConfig: v1beta1.ListenersConfig{
			InternalListeners: []v1beta1.InternalListenerConfig{
				{CommonListenerSpec: v1beta1.CommonListenerSpec{
					ContainerPort: 9092,
				},
				},
			},
		},
	}
	client := fake.NewClientBuilder().Build()
	// overwrite the var in controller_common to point kafka connections at mock
	SetNewKafkaFromCluster(kafkaclient.NewMockFromCluster)

	_, close, err := newKafkaFromCluster(client, cluster)
	if err != nil {
		t.Error("Expected no error got:", err)
	}
	close()

	// reset the newKafkaFromCluster var - will attempt to connect to a cluster
	// that doesn't exist
	SetNewKafkaFromCluster(kafkaclient.NewFromCluster)
	if _, _, err = newKafkaFromCluster(client, cluster); err == nil {
		t.Error("Expected error got nil")
	} else if !emperrors.As(err, &errorfactory.BrokersUnreachable{}) {
		t.Error("Expected brokers unreachable error, got:", err)
	}
}

func TestCheckBrokerConnectionError(t *testing.T) {
	var err error

	// Test brokers unreachable
	err = errorfactory.New(errorfactory.BrokersUnreachable{}, errors.New("test error"), "test message")
	if res, err := checkBrokerConnectionError(log, err); err != nil {
		t.Error("Expected no error in result, got:", err)
	} else {
		if res.RequeueAfter == 0 {
			t.Error("Expected requeue to be true, got false")
		}
		if res.RequeueAfter != time.Duration(15)*time.Second {
			t.Error("Expected 15 second requeue time, got:", res.RequeueAfter)
		}
	}

	// Test brokers not ready
	err = errorfactory.New(errorfactory.BrokersNotReady{}, errors.New("test error"), "test message")
	if res, err := checkBrokerConnectionError(log, err); err != nil {
		t.Error("Expected no error in result, got:", err)
	} else {
		if res.RequeueAfter == 0 {
			t.Error("Expected requeue to be true, got false")
		}
		if res.RequeueAfter != time.Duration(15)*time.Second {
			t.Error("Expected 15 second requeue time, got:", res.RequeueAfter)
		}
	}

	// test external resource not ready
	err = errorfactory.New(errorfactory.ResourceNotReady{}, errors.New("test error"), "test message")
	if res, err := checkBrokerConnectionError(log, err); err != nil {
		t.Error("Expected no error in result, got:", err)
	} else {
		if res.RequeueAfter == 0 {
			t.Error("Expected requeue to be true, got false")
		}
		if res.RequeueAfter != time.Duration(5)*time.Second {
			t.Error("Expected 5 second requeue time, got:", res.RequeueAfter)
		}
	}

	// test default response
	err = errorfactory.New(errorfactory.InternalError{}, errors.New("test error"), "test message")
	if _, err := checkBrokerConnectionError(log, err); err == nil {
		t.Error("Expected error to fall through, got nil")
	}
}

func TestApplyClusterRefLabel(t *testing.T) {
	cluster := &v1beta1.KafkaCluster{}
	cluster.Name = "test-kafka"
	cluster.Namespace = testNamespace

	// nil labels input
	var labels map[string]string
	expected := map[string]string{clusterRefLabel: "test-kafka.test-namespace"}
	newLabels := applyClusterRefLabel(cluster, labels)
	if !reflect.DeepEqual(newLabels, expected) {
		t.Error("Expected:", expected, "Got:", newLabels)
	}

	// existing label but no conflicts
	labels = map[string]string{"otherLabel": "otherValue"}
	expected = map[string]string{
		clusterRefLabel: "test-kafka.test-namespace",
		"otherLabel":    "otherValue",
	}
	newLabels = applyClusterRefLabel(cluster, labels)
	if !reflect.DeepEqual(newLabels, expected) {
		t.Error("Expected:", expected, "Got:", newLabels)
	}

	// existing label with wrong value
	labels = map[string]string{
		clusterRefLabel: "test-kafka.wrong-namespace",
		"otherLabel":    "otherValue",
	}
	expected = map[string]string{
		clusterRefLabel: "test-kafka.test-namespace",
		"otherLabel":    "otherValue",
	}
	newLabels = applyClusterRefLabel(cluster, labels)
	if !reflect.DeepEqual(newLabels, expected) {
		t.Error("Expected:", expected, "Got:", newLabels)
	}

	// existing labels with correct value - should come back untainted
	labels = map[string]string{
		clusterRefLabel: "test-kafka.test-namespace",
		"otherLabel":    "otherValue",
	}
	newLabels = applyClusterRefLabel(cluster, labels)
	if !reflect.DeepEqual(newLabels, labels) {
		t.Error("Expected:", labels, "Got:", newLabels)
	}
}
