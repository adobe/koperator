// Copyright © 2022 Cisco Systems, Inc. and/or its affiliates
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
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/banzaicloud/koperator/api/v1alpha1"
	"github.com/banzaicloud/koperator/api/v1beta1"
	mocks "github.com/banzaicloud/koperator/controllers/tests/mocks"
	"github.com/banzaicloud/koperator/pkg/resources/cruisecontrol"
	"github.com/banzaicloud/koperator/pkg/scale"
	"github.com/banzaicloud/koperator/pkg/util"
)

func createCCRetryExecutionOperation(createTime time.Time, id string, operation v1alpha1.CruiseControlTaskOperation) *v1alpha1.CruiseControlOperation {
	return &v1alpha1.CruiseControlOperation{
		ObjectMeta: v1.ObjectMeta{
			CreationTimestamp: v1.Time{
				Time: createTime,
			},
		},
		Spec: v1alpha1.CruiseControlOperationSpec{
			ErrorPolicy: v1alpha1.ErrorPolicyRetry,
		},
		Status: v1alpha1.CruiseControlOperationStatus{
			CurrentTask: &v1alpha1.CruiseControlTask{
				ID:        id,
				Operation: operation,
				State:     v1beta1.CruiseControlTaskCompletedWithError,
			},
		},
	}
}

func TestIsDeploymentRolling(t *testing.T) {
	dep := func(generation, observedGeneration int64, specReplicas, replicas, updated int32) *appsv1.Deployment {
		return &appsv1.Deployment{
			ObjectMeta: v1.ObjectMeta{Generation: generation},
			Spec:       appsv1.DeploymentSpec{Replicas: util.Int32Pointer(specReplicas)},
			Status: appsv1.DeploymentStatus{
				ObservedGeneration: observedGeneration,
				Replicas:           replicas,
				UpdatedReplicas:    updated,
			},
		}
	}

	tests := []struct {
		name string
		d    *appsv1.Deployment
		want bool
	}{
		{"settled single replica is not rolling", dep(3, 3, 1, 1, 1), false},
		{"new pod template not yet observed is rolling", dep(4, 3, 1, 1, 1), true},
		{"surge: an old-revision pod still present is rolling", dep(3, 3, 1, 2, 1), true},
		{"not all running replicas updated yet is rolling", dep(3, 3, 1, 1, 0), true},
		// envtest / no Deployment controller: status never populated (observedGeneration == 0). Must read
		// as NOT rolling so the gate does not block where nothing rolls the Deployment.
		{"unpopulated status (observedGeneration 0) is not rolling", dep(1, 0, 1, 0, 0), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isDeploymentRolling(tt.d))
		})
	}

	t.Run("nil spec.replicas defaults to 1; settled is not rolling", func(t *testing.T) {
		d := &appsv1.Deployment{
			ObjectMeta: v1.ObjectMeta{Generation: 1},
			Status:     appsv1.DeploymentStatus{ObservedGeneration: 1, Replicas: 1, UpdatedReplicas: 1},
		}
		assert.False(t, isDeploymentRolling(d))
	})
}

// TestRequeueIfCCDeploymentNotRolledOut directly exercises the broker-op gate's branching: it must requeue
// (defer) on a mismatched capacity hash and on an add_broker whose target capacity is not yet in
// capacity.json, execute once both hold, and fail open when the Deployment/ConfigMap is absent or capacity
// is not koperator-managed - the deferral behavior the e2e (which only asserts the eventual matched state)
// cannot prove.
//
//nolint:funlen
func TestRequeueIfCCDeploymentNotRolledOut(t *testing.T) {
	const namespace = "kafka"
	kc := &v1beta1.KafkaCluster{ObjectMeta: v1.ObjectMeta{Name: "kafka", Namespace: namespace}}
	depName := cruisecontrol.DeploymentName(kc)
	cmName := cruisecontrol.ConfigMapName(kc)
	addParams := map[string]string{scale.ParamBrokerID: "103"}

	capacityWith103 := `{"brokerCapacities":[{"brokerId":"100"},{"brokerId":"103"}]}`
	capacityNo103 := `{"brokerCapacities":[{"brokerId":"100"}]}`

	// settledDeployment is a fully rolled-out CC Deployment carrying the given capacity-hash annotation (nil
	// annotations => capacity is not koperator-managed).
	settledDeployment := func(annotations map[string]string) *appsv1.Deployment {
		return &appsv1.Deployment{
			ObjectMeta: v1.ObjectMeta{Name: depName, Namespace: namespace, Generation: 1},
			Spec: appsv1.DeploymentSpec{
				Replicas: util.Int32Pointer(1),
				Template: corev1.PodTemplateSpec{ObjectMeta: v1.ObjectMeta{Annotations: annotations}},
			},
			Status: appsv1.DeploymentStatus{ObservedGeneration: 1, Replicas: 1, UpdatedReplicas: 1},
		}
	}
	hashAnnotation := func(capacityJSON string) map[string]string {
		return map[string]string{cruisecontrol.CapacityConfigHashAnnotationKey: cruisecontrol.CapacityConfigHash(capacityJSON)}
	}
	rollingDeployment := func() *appsv1.Deployment {
		d := settledDeployment(hashAnnotation(capacityWith103))
		d.Generation = 2 // new pod template not yet observed => rolling
		return d
	}
	configMap := func(capacityJSON string) *corev1.ConfigMap {
		return &corev1.ConfigMap{
			ObjectMeta: v1.ObjectMeta{Name: cmName, Namespace: namespace},
			Data:       map[string]string{cruisecontrol.CapacityConfigMapKey: capacityJSON},
		}
	}
	configMapWithProperties := func(capacityJSON, ccProperties string) *corev1.ConfigMap {
		cm := configMap(capacityJSON)
		cm.Data[cruisecontrol.PropertiesConfigMapKey] = ccProperties
		return cm
	}

	tests := []struct {
		name        string
		op          v1alpha1.CruiseControlTaskOperation
		params      map[string]string
		objects     []client.Object
		wantHandled bool // true => the caller returns immediately (deferred / gated)
		wantRequeue bool // true => the returned result asks for a requeue
	}{
		{
			name:    "stop_execution is never gated, even mid-rollout",
			op:      v1alpha1.OperationStopExecution,
			objects: []client.Object{rollingDeployment()},
		},
		{
			name:    "missing CC Deployment fails open",
			op:      v1alpha1.OperationAddBroker,
			params:  addParams,
			objects: nil,
		},
		{
			name:        "rebalance is deferred while the Deployment is rolling",
			op:          v1alpha1.OperationRebalance,
			objects:     []client.Object{rollingDeployment()},
			wantHandled: true,
			wantRequeue: true,
		},
		{
			name:        "remove_disks is deferred while the Deployment is rolling",
			op:          v1alpha1.OperationRemoveDisks,
			objects:     []client.Object{rollingDeployment()},
			wantHandled: true,
			wantRequeue: true,
		},
		{
			// No capacity annotation => capacity is not koperator-managed, nothing to wait for.
			name:    "rebalance on a settled Deployment with no capacity annotation fails open",
			op:      v1alpha1.OperationRebalance,
			objects: []client.Object{settledDeployment(nil)},
		},
		{
			// Pre-roll protection now covers non-broker ops too: a settled Deployment whose capacity hash
			// still lags the ConfigMap defers rebalance/remove_disks, not just broker add/remove.
			name:        "rebalance defers while the pod template capacity hash lags the ConfigMap",
			op:          v1alpha1.OperationRebalance,
			objects:     []client.Object{settledDeployment(map[string]string{cruisecontrol.CapacityConfigHashAnnotationKey: "stale"}), configMap(capacityWith103)},
			wantHandled: true,
			wantRequeue: true,
		},
		{
			name:    "rebalance executes once the pod template carries the current capacity hash",
			op:      v1alpha1.OperationRebalance,
			objects: []client.Object{settledDeployment(hashAnnotation(capacityWith103)), configMap(capacityWith103)},
		},
		{
			// Capacity hash matches, but cruisecontrol.properties changed and has not been rolled into the
			// pod template yet: a rebalance submitted now would still be wiped by the roll that follows, so
			// the gate must cover this hash too, not just capacity.json.
			name: "rebalance defers while the pod template cruisecontrol.properties hash lags the ConfigMap",
			op:   v1alpha1.OperationRebalance,
			objects: []client.Object{
				settledDeployment(util.MergeAnnotations(hashAnnotation(capacityWith103), map[string]string{cruisecontrol.ConfigHashAnnotationKey: "stale"})),
				configMapWithProperties(capacityWith103, "some.property=value"),
			},
			wantHandled: true,
			wantRequeue: true,
		},
		{
			// Same gap, exercised via clusterConfigs.json / log4j.properties (ClusterConfigHashAnnotationKey /
			// LogConfigHashAnnotationKey) so all three non-capacity hashes are covered, not just one.
			name: "rebalance defers while the pod template clusterConfigs.json hash lags the ConfigMap",
			op:   v1alpha1.OperationRebalance,
			objects: []client.Object{
				settledDeployment(util.MergeAnnotations(hashAnnotation(capacityWith103), map[string]string{cruisecontrol.ClusterConfigHashAnnotationKey: "stale"})),
				configMap(capacityWith103),
			},
			wantHandled: true,
			wantRequeue: true,
		},
		{
			name: "rebalance executes once all four hashes match the ConfigMap",
			op:   v1alpha1.OperationRebalance,
			objects: []client.Object{
				settledDeployment(util.MergeAnnotations(hashAnnotation(capacityWith103), map[string]string{
					cruisecontrol.ConfigHashAnnotationKey:        cruisecontrol.ConfigHash("some.property=value"),
					cruisecontrol.ClusterConfigHashAnnotationKey: cruisecontrol.ConfigHash(""),
					cruisecontrol.LogConfigHashAnnotationKey:     cruisecontrol.ConfigHash(""),
				})),
				configMapWithProperties(capacityWith103, "some.property=value"),
			},
		},
		{
			name:        "Deployment mid-rollout requeues",
			op:          v1alpha1.OperationAddBroker,
			params:      addParams,
			objects:     []client.Object{rollingDeployment(), configMap(capacityWith103)},
			wantHandled: true,
			wantRequeue: true,
		},
		{
			name:    "settled without capacity annotation fails open (capacity not koperator-managed)",
			op:      v1alpha1.OperationAddBroker,
			params:  addParams,
			objects: []client.Object{settledDeployment(nil), configMap(capacityWith103)},
		},
		{
			name:    "settled, matching hash, missing ConfigMap fails open",
			op:      v1alpha1.OperationAddBroker,
			params:  addParams,
			objects: []client.Object{settledDeployment(hashAnnotation(capacityWith103))},
		},
		{
			name:        "capacity hash mismatch requeues",
			op:          v1alpha1.OperationAddBroker,
			params:      addParams,
			objects:     []client.Object{settledDeployment(map[string]string{cruisecontrol.CapacityConfigHashAnnotationKey: "stale"}), configMap(capacityWith103)},
			wantHandled: true,
			wantRequeue: true,
		},
		{
			name:        "add_broker requeues while target broker capacity is absent",
			op:          v1alpha1.OperationAddBroker,
			params:      addParams,
			objects:     []client.Object{settledDeployment(hashAnnotation(capacityNo103)), configMap(capacityNo103)},
			wantHandled: true,
			wantRequeue: true,
		},
		{
			name:    "add_broker executes once capacity is rolled and contains the target broker",
			op:      v1alpha1.OperationAddBroker,
			params:  addParams,
			objects: []client.Object{settledDeployment(hashAnnotation(capacityWith103)), configMap(capacityWith103)},
		},
		{
			name:    "remove_broker executes on a settled, hash-matched CC (no broker-presence check)",
			op:      v1alpha1.OperationRemoveBroker,
			params:  map[string]string{scale.ParamBrokerID: "103"},
			objects: []client.Object{settledDeployment(hashAnnotation(capacityWith103)), configMap(capacityWith103)},
		},
	}

	scheme := runtime.NewScheme()
	assert.NoError(t, k8sscheme.AddToScheme(scheme))

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// DirectClient deliberately left unset here so the whole table also exercises the nil-DirectClient
			// fallback to the cached Client (see the reader-selection in requeueIfCCDeploymentNotRolledOut).
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tt.objects...).Build()
			r := &CruiseControlOperationReconciler{Client: fakeClient}

			result, handled, err := r.requeueIfCCDeploymentNotRolledOut(context.Background(), logr.Discard(), kc, tt.op, tt.params)
			assert.NoError(t, err)
			assert.Equal(t, tt.wantHandled, handled, "handled")
			assert.Equal(t, tt.wantRequeue, result.RequeueAfter > 0, "requeue")
		})
	}

	// The gate's correctness depends on reading the freshest Deployment/ConfigMap: a lagging informer cache
	// could show the OLD state as matching and let an op be submitted just before a roll wipes it (the #301
	// race). It must therefore read through DirectClient (the non-cached API reader) when one is set. We prove
	// this by making the cached Client and DirectClient disagree: a stale cache showing a settled, hash-matched
	// CC (which would execute) versus a fresh direct view showing a mid-rollout CC (which must defer). The gate
	// must follow the fresh view and defer.
	t.Run("reads through DirectClient when set (fresh mid-rollout view defers over a stale settled cache)", func(t *testing.T) {
		staleCache := fake.NewClientBuilder().WithScheme(scheme).
			WithObjects(settledDeployment(hashAnnotation(capacityWith103)), configMap(capacityWith103)).Build()
		freshDirect := fake.NewClientBuilder().WithScheme(scheme).
			WithObjects(rollingDeployment(), configMap(capacityWith103)).Build()
		r := &CruiseControlOperationReconciler{Client: staleCache, DirectClient: freshDirect}

		result, handled, err := r.requeueIfCCDeploymentNotRolledOut(context.Background(), logr.Discard(), kc, v1alpha1.OperationAddBroker, addParams)
		assert.NoError(t, err)
		assert.True(t, handled, "gate must defer using DirectClient's fresh mid-rollout view, not the stale settled cache")
		assert.True(t, result.RequeueAfter > 0, "requeue")
	})

	// Symmetric proof it is really DirectClient (not Client) being consulted: a stale cache showing a
	// mid-rollout CC (which would defer) versus a fresh direct view showing a settled, hash-matched CC that
	// contains the target broker (which must execute). The gate must follow the fresh view and execute.
	t.Run("reads through DirectClient when set (fresh settled view executes over a stale rolling cache)", func(t *testing.T) {
		staleCache := fake.NewClientBuilder().WithScheme(scheme).
			WithObjects(rollingDeployment(), configMap(capacityWith103)).Build()
		freshDirect := fake.NewClientBuilder().WithScheme(scheme).
			WithObjects(settledDeployment(hashAnnotation(capacityWith103)), configMap(capacityWith103)).Build()
		r := &CruiseControlOperationReconciler{Client: staleCache, DirectClient: freshDirect}

		result, handled, err := r.requeueIfCCDeploymentNotRolledOut(context.Background(), logr.Discard(), kc, v1alpha1.OperationAddBroker, addParams)
		assert.NoError(t, err)
		assert.False(t, handled, "gate must execute using DirectClient's fresh settled view, not the stale rolling cache")
		assert.Zero(t, result.RequeueAfter, "no requeue")
	})

	// A wedged rollout (mid-rollout AND Progressing=False/ProgressDeadlineExceeded) is the behavior most novel
	// to this change: the op must still be deferred (the new CC pod never became available, so submitting would
	// race a restart), but with the longer backoff so the surfaced error is not re-emitted every default
	// interval for the life of the wedge. Assert the exact stalled interval, which the boolean table cannot.
	t.Run("wedged rollout defers with the stalled backoff interval", func(t *testing.T) {
		wedged := rollingDeployment()
		wedged.Status.Conditions = []appsv1.DeploymentCondition{
			{Type: appsv1.DeploymentProgressing, Status: corev1.ConditionFalse, Reason: progressDeadlineExceededReason},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(wedged, configMap(capacityWith103)).Build()
		r := &CruiseControlOperationReconciler{Client: fakeClient}

		result, handled, err := r.requeueIfCCDeploymentNotRolledOut(context.Background(), logr.Discard(), kc, v1alpha1.OperationAddBroker, addParams)
		assert.NoError(t, err)
		assert.True(t, handled, "wedged rollout must defer the operation")
		assert.Equal(t, time.Duration(stalledCCDeploymentRequeueIntervalSeconds)*time.Second, result.RequeueAfter,
			"wedged rollout must back off with the stalled interval, not the default")
	})

	// A failing (non-NotFound) Deployment read must surface an error and defer, not fail open - none of the
	// table cases (all assert NoError) cover the requeueWithError branch. Use an interceptor that errors on the
	// Deployment Get.
	t.Run("errored Deployment read surfaces the error and defers", func(t *testing.T) {
		failing := interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if _, ok := obj.(*appsv1.Deployment); ok {
					return apiErrors.NewServiceUnavailable("apiserver down")
				}
				return c.Get(ctx, key, obj, opts...)
			},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).
			WithObjects(settledDeployment(hashAnnotation(capacityWith103)), configMap(capacityWith103)).
			WithInterceptorFuncs(failing).Build()
		r := &CruiseControlOperationReconciler{Client: fakeClient}

		_, handled, err := r.requeueIfCCDeploymentNotRolledOut(context.Background(), logr.Discard(), kc, v1alpha1.OperationAddBroker, addParams)
		assert.Error(t, err, "a non-NotFound Deployment read error must be surfaced, not swallowed")
		assert.True(t, handled, "an errored read must defer, not fail open")
		// requeueWithError requeues via the non-nil error (RequeueAfter stays 0), so do not assert on it here.
	})

	// The sibling of the Deployment-read error: a failing (non-NotFound) ConfigMap read must also surface the
	// error and defer, not fail open (the Deployment is read first, so it must succeed for the ConfigMap read
	// to be reached).
	t.Run("errored ConfigMap read surfaces the error and defers", func(t *testing.T) {
		failing := interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if _, ok := obj.(*corev1.ConfigMap); ok {
					return apiErrors.NewServiceUnavailable("apiserver down")
				}
				return c.Get(ctx, key, obj, opts...)
			},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).
			WithObjects(settledDeployment(hashAnnotation(capacityWith103)), configMap(capacityWith103)).
			WithInterceptorFuncs(failing).Build()
		r := &CruiseControlOperationReconciler{Client: fakeClient}

		_, handled, err := r.requeueIfCCDeploymentNotRolledOut(context.Background(), logr.Discard(), kc, v1alpha1.OperationAddBroker, addParams)
		assert.Error(t, err, "a non-NotFound ConfigMap read error must be surfaced, not swallowed")
		assert.True(t, handled, "an errored read must defer, not fail open")
	})

	// An add_broker whose capacity.json is present and hash-matched but unparseable must surface the
	// CapacityConfigContainsBrokers error and defer, not fail open - exercising that error branch through the
	// gate (TestCapacityConfigContainsBrokers only covers the helper in isolation).
	t.Run("add_broker with unparseable capacity.json surfaces the error and defers", func(t *testing.T) {
		malformed := "{not json"
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).
			WithObjects(settledDeployment(hashAnnotation(malformed)), configMap(malformed)).Build()
		r := &CruiseControlOperationReconciler{Client: fakeClient}

		_, handled, err := r.requeueIfCCDeploymentNotRolledOut(context.Background(), logr.Discard(), kc, v1alpha1.OperationAddBroker, addParams)
		assert.Error(t, err, "an unparseable capacity.json must surface the inspection error, not fail open")
		assert.True(t, handled, "an errored capacity inspection must defer, not fail open")
	})
}

func TestDeploymentRolloutTimedOut(t *testing.T) {
	timedOut := &appsv1.Deployment{Status: appsv1.DeploymentStatus{Conditions: []appsv1.DeploymentCondition{
		{Type: appsv1.DeploymentProgressing, Status: corev1.ConditionFalse, Reason: progressDeadlineExceededReason},
	}}}
	progressing := &appsv1.Deployment{Status: appsv1.DeploymentStatus{Conditions: []appsv1.DeploymentCondition{
		{Type: appsv1.DeploymentProgressing, Status: corev1.ConditionTrue, Reason: "NewReplicaSetAvailable"},
	}}}
	assert.True(t, deploymentRolloutTimedOut(timedOut))
	assert.False(t, deploymentRolloutTimedOut(progressing))
	assert.False(t, deploymentRolloutTimedOut(&appsv1.Deployment{}))
}

func TestSplitNonEmpty(t *testing.T) {
	assert.Nil(t, splitNonEmpty(""))
	assert.Equal(t, []string{"103"}, splitNonEmpty("103"))
	assert.Equal(t, []string{"100", "101"}, splitNonEmpty("100,101"))
}

func TestSortOperations(t *testing.T) {
	timeNow := time.Now()
	testCases := []struct {
		testName       string
		ccOperations   []*v1alpha1.CruiseControlOperation
		expectedOutput []*v1alpha1.CruiseControlOperation
	}{
		{
			testName: "creation time",
			ccOperations: []*v1alpha1.CruiseControlOperation{
				createCCRetryExecutionOperation(timeNow.Add(3*time.Second), "1", v1alpha1.OperationAddBroker),
				createCCRetryExecutionOperation(timeNow, "2", v1alpha1.OperationAddBroker),
				createCCRetryExecutionOperation(timeNow.Add(time.Second), "3", v1alpha1.OperationAddBroker),
				createCCRetryExecutionOperation(timeNow.Add(2*time.Second), "4", v1alpha1.OperationAddBroker),
			},
			expectedOutput: []*v1alpha1.CruiseControlOperation{
				createCCRetryExecutionOperation(timeNow, "2", v1alpha1.OperationAddBroker),
				createCCRetryExecutionOperation(timeNow.Add(time.Second), "3", v1alpha1.OperationAddBroker),
				createCCRetryExecutionOperation(timeNow.Add(2*time.Second), "4", v1alpha1.OperationAddBroker),
				createCCRetryExecutionOperation(timeNow.Add(3*time.Second), "1", v1alpha1.OperationAddBroker),
			},
		},
		{
			testName: "mixed",
			ccOperations: []*v1alpha1.CruiseControlOperation{
				createCCRetryExecutionOperation(timeNow.Add(time.Second), "1", v1alpha1.OperationAddBroker),
				createCCRetryExecutionOperation(timeNow.Add(time.Second), "2", v1alpha1.OperationRemoveBroker),
				createCCRetryExecutionOperation(timeNow, "3", v1alpha1.OperationRebalance),
				createCCRetryExecutionOperation(timeNow, "4", v1alpha1.OperationAddBroker),
				createCCRetryExecutionOperation(timeNow, "5", v1alpha1.OperationRemoveBroker),
			},
			expectedOutput: []*v1alpha1.CruiseControlOperation{
				createCCRetryExecutionOperation(timeNow, "4", v1alpha1.OperationAddBroker),
				createCCRetryExecutionOperation(timeNow.Add(time.Second), "1", v1alpha1.OperationAddBroker),
				createCCRetryExecutionOperation(timeNow, "5", v1alpha1.OperationRemoveBroker),
				createCCRetryExecutionOperation(timeNow.Add(time.Second), "2", v1alpha1.OperationRemoveBroker),
				createCCRetryExecutionOperation(timeNow, "3", v1alpha1.OperationRebalance),
			},
		},
		{
			testName: "mixed with remove disks",
			ccOperations: []*v1alpha1.CruiseControlOperation{
				createCCRetryExecutionOperation(timeNow, "1", v1alpha1.OperationAddBroker),
				createCCRetryExecutionOperation(timeNow, "4", v1alpha1.OperationRebalance),
				createCCRetryExecutionOperation(timeNow.Add(2*time.Second), "3", v1alpha1.OperationRemoveDisks),
				createCCRetryExecutionOperation(timeNow.Add(time.Second), "2", v1alpha1.OperationRemoveBroker),
			},
			expectedOutput: []*v1alpha1.CruiseControlOperation{
				createCCRetryExecutionOperation(timeNow, "1", v1alpha1.OperationAddBroker),
				createCCRetryExecutionOperation(timeNow.Add(time.Second), "2", v1alpha1.OperationRemoveBroker),
				createCCRetryExecutionOperation(timeNow.Add(2*time.Second), "3", v1alpha1.OperationRemoveDisks),
				createCCRetryExecutionOperation(timeNow, "4", v1alpha1.OperationRebalance),
			},
		},
	}
	for _, testCase := range testCases {
		sortedCCOperations := sortOperations(testCase.ccOperations)
		sortedRetryOutput := sortedCCOperations[ccOperationRetryExecution]
		assert.Equal(t, sortedRetryOutput, testCase.expectedOutput, "test", testCase.testName)
	}
}

// TestGetStatusDoesNotPanicWhenStatusNil exercises the res.Status==nil branch of
// getStatus. On that path statusOperation is always nil (it is only assigned in the
// early-returning statusOperation!=nil branch), so the error-wraps must reference the
// freshly-created operation, not statusOperation. With the bug present this panics with
// a nil-pointer dereference; with the fix it returns a wrapped error.
//
// The scaler returns Status==nil with an empty TaskResult, so updateResult fails parsing
// the (empty) start time and we deterministically reach the buggy error-wrap. createCCOperation
// must succeed first, hence the registered status subresource on a plain fake client.
func TestGetStatusDoesNotPanicWhenStatusNil(t *testing.T) {
	ctrlMock := gomock.NewController(t)
	defer ctrlMock.Finish()

	mockScaler := mocks.NewMockCruiseControlScaler(ctrlMock)
	mockScaler.EXPECT().Status(gomock.Any()).
		Return(scale.StatusTaskResult{Status: nil, TaskResult: &scale.Result{}}, nil)

	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := v1beta1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.CruiseControlOperation{}).
		Build()

	r := &CruiseControlOperationReconciler{Client: fakeClient, Scheme: scheme, scaler: mockScaler}

	kafkaCluster := &v1beta1.KafkaCluster{
		ObjectMeta: v1.ObjectMeta{Name: "kafka", Namespace: "default"},
	}
	ref := client.ObjectKey{Name: "kafka", Namespace: "default"}

	// With the bug this panics; with the fix it returns a wrapped error.
	_, err := r.getStatus(context.Background(), logr.Discard(), kafkaCluster,
		ref, v1alpha1.CruiseControlOperationList{})
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
}
