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
	"net/http/httptest"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	emperrors "emperror.dev/errors"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	//nolint:staticcheck
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/banzaicloud/koperator/api/v1alpha1"
	"github.com/banzaicloud/koperator/api/v1beta1"
	"github.com/banzaicloud/koperator/pkg/errorfactory"
	"github.com/banzaicloud/koperator/pkg/kafkaclient"
)

var log = logf.Log.WithName("controller_testing")

const (
	testNamespace = "test-namespace"
)

type alertTestManager struct {
	manager.Manager
	client   client.Client
	reader   client.Reader
	elected  chan struct{}
	runnable manager.Runnable
	check    healthz.Checker
}

func (m *alertTestManager) GetClient() client.Client     { return m.client }
func (m *alertTestManager) GetAPIReader() client.Reader  { return m.reader }
func (m *alertTestManager) Elected() <-chan struct{}     { return m.elected }
func (m *alertTestManager) Add(r manager.Runnable) error { m.runnable = r; return nil }
func (m *alertTestManager) AddReadyzCheck(_ string, check healthz.Checker) error {
	m.check = check
	return nil
}

func TestAlertManagerIdentityRegistration(t *testing.T) {
	for _, tc := range []struct {
		name, namespace, pod, uid string
		wantErr                   bool
	}{
		{name: "legacy"},
		{name: "namespace alone remains legacy", namespace: "operators"},
		{name: "name alone rejected", namespace: "operators", pod: "operator", wantErr: true},
		{name: "UID alone rejected", namespace: "operators", uid: "uid", wantErr: true},
		{name: "missing namespace rejected", pod: "operator", uid: "uid", wantErr: true},
		{name: "complete", namespace: "operators", pod: "operator", uid: "uid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("POD_NAMESPACE", tc.namespace)
			t.Setenv("POD_NAME", tc.pod)
			t.Setenv("POD_UID", tc.uid)
			mgr := &alertTestManager{elected: make(chan struct{})}
			err := SetAlertManagerWithManager(mgr)
			if (err != nil) != tc.wantErr {
				t.Fatalf("registration error = %v, want error = %t", err, tc.wantErr)
			}
			if !tc.wantErr && mgr.check == nil {
				t.Fatal("alert readiness was not registered")
			}
		})
	}
}

type alertTestClient struct {
	client.Client
	update func(context.Context, client.Object, ...client.UpdateOption) error
}

func (c *alertTestClient) Get(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
	return errors.New("operator Pod is outside cached namespaces")
}

func (c *alertTestClient) List(context.Context, client.ObjectList, ...client.ListOption) error {
	return errors.New("operator Pods are outside cached namespaces")
}

func (c *alertTestClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	if c.update != nil {
		return c.update(ctx, obj, opts...)
	}
	return c.Client.Update(ctx, obj, opts...)
}

type alertTestReader struct {
	client.Reader
	getErr, listErr error
}

func (r alertTestReader) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if r.getErr != nil {
		return r.getErr
	}
	return r.Reader.Get(ctx, key, obj, opts...)
}

func (r alertTestReader) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if r.listErr != nil {
		return r.listErr
	}
	return r.Reader.List(ctx, list, opts...)
}

func alertTestPod(name, release string) *corev1.Pod {
	return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name: name, Namespace: "operators", UID: types.UID(name),
		Labels: map[string]string{
			"control-plane": "controller-manager", "controller-tools.k8s.io": "1.0",
			"app.kubernetes.io/name": "kafka-operator", "app.kubernetes.io/instance": release,
			"app.kubernetes.io/component": "operator", "kafka.banzaicloud.io/alert-leader": "true",
		},
	}}
}

func newAlertTestController(pods ...client.Object) (AController, *alertTestClient) {
	storage := fake.NewClientBuilder().WithObjects(pods...).Build()
	writer := &alertTestClient{Client: storage}
	elected := make(chan struct{})
	close(elected)
	return AController{
		Client: writer, reader: storage, elected: elected, state: &alertServingState{},
		pod: types.NamespacedName{Namespace: "operators", Name: "self"}, uid: "self",
	}, writer
}

func runAlertForTest(t *testing.T, c AController, handler http.Handler) (context.CancelFunc, <-chan error, string) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	done, stopped := make(chan error, 1), make(chan struct{})
	go func() {
		done <- c.run(ctx, listener, handler)
		close(stopped)
	}()
	t.Cleanup(func() {
		cancel()
		_ = listener.Close()
		select {
		case <-stopped:
		case <-time.After(4 * time.Second):
			t.Error("alert controller did not stop within its cleanup bound")
		}
	})
	return cancel, done, "http://" + listener.Addr().String()
}

func TestAlertRoutingUsesScopedUncachedIdentity(t *testing.T) {
	self, stale, other := alertTestPod("self", "release"), alertTestPod("stale", "release"), alertTestPod("other", "other-release")
	foreign := alertTestPod("foreign", "release")
	foreign.Namespace = "other-namespace"
	c, writer := newAlertTestController(self, stale, other, foreign)
	var handled atomic.Int32
	cancel, done, address := runAlertForTest(t, c, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		handled.Add(1)
		_, _ = w.Write([]byte("ok"))
	}))
	require.Eventually(t, func() bool { return c.readyz(nil) == nil }, time.Second, time.Millisecond)
	for _, expected := range []struct {
		pod    *corev1.Pod
		active bool
	}{{self, true}, {stale, false}, {other, true}, {foreign, true}} {
		current := &corev1.Pod{}
		require.NoError(t, writer.Client.Get(context.Background(), client.ObjectKeyFromObject(expected.pod), current))
		require.Equal(t, expected.active, current.Labels["kafka.banzaicloud.io/alert-leader"] == "true", expected.pod.Name)
	}
	transport := &http.Transport{}
	defer transport.CloseIdleConnections()
	httpClient := &http.Client{Transport: transport, Timeout: time.Second}
	response, err := httpClient.Get(address)
	require.NoError(t, err)
	_, err = io.ReadAll(response.Body)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	cancel()
	require.Error(t, c.readyz(nil), "observed cancellation must immediately fail elected readiness")
	response, err = httpClient.Get(address)
	if err == nil {
		require.Equal(t, http.StatusServiceUnavailable, response.StatusCode)
		require.NoError(t, response.Body.Close())
	}
	require.NoError(t, <-done)
	require.EqualValues(t, 1, handled.Load(), "stale keepalive traffic reached the receiver")
	current := &corev1.Pod{}
	require.NoError(t, writer.Client.Get(context.Background(), c.pod, current))
	require.Empty(t, current.Labels["kafka.banzaicloud.io/alert-leader"])
}

func TestAlertRoutingStartupFailures(t *testing.T) {
	for _, failure := range []string{"UID", "labels", "get", "deleted", "list", "write", "conflicts", "ambiguous"} {
		t.Run(failure, func(t *testing.T) {
			self := alertTestPod("self", "release")
			delete(self.Labels, "kafka.banzaicloud.io/alert-leader")
			if failure == "labels" {
				delete(self.Labels, "app.kubernetes.io/instance")
			}
			c, writer := newAlertTestController(self)
			switch failure {
			case "UID":
				c.uid = "replacement"
			case "get":
				c.reader = alertTestReader{Reader: c.reader, getErr: errors.New("read denied")}
			case "deleted":
				c.reader = alertTestReader{Reader: c.reader, getErr: apierrors.NewNotFound(schema.GroupResource{Resource: "pods"}, "self")}
			case "list":
				c.reader = alertTestReader{Reader: c.reader, listErr: errors.New("list denied")}
			case "write", "conflicts", "ambiguous":
				writer.update = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
					if obj.GetLabels()["kafka.banzaicloud.io/alert-leader"] == "true" {
						if failure == "ambiguous" {
							if err := writer.Client.Update(ctx, obj, opts...); err != nil {
								return err
							}
						}
						if failure == "conflicts" {
							return apierrors.NewConflict(schema.GroupResource{Resource: "pods"}, "self", errors.New("conflict"))
						}
						return errors.New("publication failed")
					}
					return writer.Client.Update(ctx, obj, opts...)
				}
			}
			_, done, _ := runAlertForTest(t, c, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				t.Error("failed startup admitted an alert")
			}))
			select {
			case err := <-done:
				require.Error(t, err)
			case <-time.After(time.Second):
				t.Fatal("startup failure was not returned")
			}
			require.Error(t, c.readyz(nil))
			current := &corev1.Pod{}
			require.NoError(t, writer.Client.Get(context.Background(), c.pod, current))
			require.Empty(t, current.Labels["kafka.banzaicloud.io/alert-leader"])
		})
	}
}

func TestAlertRoutingConflictPreservesMetadata(t *testing.T) {
	self := alertTestPod("self", "release")
	delete(self.Labels, "kafka.banzaicloud.io/alert-leader")
	c, writer := newAlertTestController(self)
	var updates atomic.Int32
	writer.update = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
		if updates.Add(1) == 1 {
			current := &corev1.Pod{}
			if err := writer.Client.Get(ctx, c.pod, current); err != nil {
				return err
			}
			current.Labels["concurrent-label"] = "preserved"
			if err := writer.Client.Update(ctx, current); err != nil {
				return err
			}
			return apierrors.NewConflict(schema.GroupResource{Resource: "pods"}, "self", errors.New("concurrent update"))
		}
		return writer.Client.Update(ctx, obj, opts...)
	}
	cancel, done, _ := runAlertForTest(t, c, http.NotFoundHandler())
	require.Eventually(t, func() bool { return c.readyz(nil) == nil }, time.Second, time.Millisecond)
	cancel()
	require.NoError(t, <-done)
	current := &corev1.Pod{}
	require.NoError(t, writer.Client.Get(context.Background(), c.pod, current))
	require.Equal(t, "preserved", current.Labels["concurrent-label"])
	require.GreaterOrEqual(t, updates.Load(), int32(3))
}

func TestAlertReadinessPreservesStandbysAndLegacy(t *testing.T) {
	c, _ := newAlertTestController()
	elected := make(chan struct{})
	c.elected = elected
	require.NoError(t, c.readyz(nil), "standby must not require an alert listener")
	close(elected)
	require.Error(t, c.readyz(nil), "elected startup must not claim serving readiness")
	c.pod, c.uid = types.NamespacedName{}, ""
	cancel, done, _ := runAlertForTest(t, c, http.NotFoundHandler())
	require.Eventually(t, func() bool { return c.readyz(nil) == nil }, time.Second, time.Millisecond)
	cancel()
	require.Error(t, c.readyz(nil))
	require.NoError(t, <-done)
}

func TestAlertRoutingGateAndBoundedCleanup(t *testing.T) {
	c, writer := newAlertTestController(alertTestPod("self", "release"))
	cleanupStarted := make(chan struct{})
	var cleanupOnce sync.Once
	writer.update = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
		if obj.GetLabels()["kafka.banzaicloud.io/alert-leader"] != "true" {
			// Only block final cleanup, not startup's stale-label removal.
			select {
			case <-cleanupStarted:
			default:
				c.state.RLock()
				stopping := c.state.ctx != nil && c.state.ctx.Err() != nil
				c.state.RUnlock()
				if stopping {
					cleanupOnce.Do(func() { close(cleanupStarted) })
					<-ctx.Done()
					return ctx.Err()
				}
			}
		}
		return writer.Client.Update(ctx, obj, opts...)
	}
	cancel, done, address := runAlertForTest(t, c, http.NotFoundHandler())
	require.Eventually(t, func() bool { return c.readyz(nil) == nil }, time.Second, time.Millisecond)
	cancel()
	select {
	case <-cleanupStarted:
	case <-time.After(time.Second):
		t.Fatal("cleanup did not start")
	}
	connection, err := net.DialTimeout("tcp", address[len("http://"):], time.Second)
	if connection != nil {
		_ = connection.Close()
	}
	require.Error(t, err, "listener remained open while cleanup was blocked")
	c.state.RLock()
	ctx := c.state.ctx
	c.state.RUnlock()
	recorder := httptest.NewRecorder()
	c.gatedHandler(ctx, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("canceled admission reached the receiver")
	})).ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/", nil))
	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.DeadlineExceeded)
	case <-time.After(3 * time.Second):
		t.Fatal("blocked label cleanup exceeded its bound")
	}
}

func TestAlertRoutingCleanupPreservesReplacementUID(t *testing.T) {
	self := alertTestPod("self", "release")
	c, writer := newAlertTestController(self)
	cancel, done, _ := runAlertForTest(t, c, http.NotFoundHandler())
	require.Eventually(t, func() bool { return c.readyz(nil) == nil }, time.Second, time.Millisecond)
	require.NoError(t, writer.Delete(context.Background(), self))
	replacement := alertTestPod("self", "release")
	replacement.UID = "replacement"
	require.NoError(t, writer.Create(context.Background(), replacement))
	cancel()
	require.NoError(t, <-done)
	current := &corev1.Pod{}
	require.NoError(t, writer.Client.Get(context.Background(), c.pod, current))
	require.Equal(t, "true", current.Labels["kafka.banzaicloud.io/alert-leader"])
}

func TestAlertRoutingServeFailureRemovesLabel(t *testing.T) {
	c, writer := newAlertTestController(alertTestPod("self", "release"))
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- c.run(ctx, listener, http.NotFoundHandler()) }()
	require.Eventually(t, func() bool { return c.readyz(nil) == nil }, time.Second, time.Millisecond)
	require.NoError(t, listener.Close())
	select {
	case err := <-done:
		require.ErrorIs(t, err, net.ErrClosed)
	case <-time.After(3 * time.Second):
		t.Fatal("Serve failure did not stop routing")
	}
	current := &corev1.Pod{}
	require.NoError(t, writer.Client.Get(context.Background(), c.pod, current))
	require.Empty(t, current.Labels["kafka.banzaicloud.io/alert-leader"])
	require.Error(t, c.readyz(nil))
}

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
	c, writer := newAlertTestController(alertTestPod("self", "release"))
	writer.update = func(context.Context, client.Object, ...client.UpdateOption) error {
		t.Error("bind failure attempted to change a routing label")
		return errors.New("unexpected update")
	}
	require.Error(t, c.Start(context.Background()))
	require.Error(t, c.readyz(nil))
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
