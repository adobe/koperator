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
	"errors"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/banzaicloud/koperator/api/v1alpha1"
	"github.com/banzaicloud/koperator/api/v1beta1"
	mocks "github.com/banzaicloud/koperator/controllers/tests/mocks"
	"github.com/banzaicloud/koperator/pkg/scale"
)

func TestOperationResultPersistsAcrossStatusConflicts(t *testing.T) {
	for _, tt := range []struct {
		name        string
		retry       bool
		deleting    bool
		replaced    bool
		taskRemoved bool
		taskChanged bool
		exhausted   bool
		conflicts   int
		wantRetries int
		wantFailed  int
		wantOp      v1alpha1.CruiseControlTaskOperation
	}{
		{name: "first submission without conflict", wantOp: v1alpha1.OperationAddBroker},
		{name: "retry without conflict preserves history once", retry: true, wantRetries: 3, wantFailed: 2, wantOp: v1alpha1.OperationAddBroker},
		{name: "stop submission without conflict retains operation", deleting: true, wantOp: v1alpha1.OperationStopExecution},
		{name: "first submission", conflicts: 1, wantOp: v1alpha1.OperationAddBroker},
		{name: "retry preserves history once", retry: true, conflicts: 2, wantRetries: 3, wantFailed: 2, wantOp: v1alpha1.OperationAddBroker},
		{name: "stop submission retains operation", deleting: true, conflicts: 1, wantOp: v1alpha1.OperationStopExecution},
		{name: "replacement object rejects stale result", replaced: true, conflicts: 1},
		{name: "removed task rejects stale result", taskRemoved: true, conflicts: 1},
		{name: "newer task rejects stale result", taskChanged: true, conflicts: 1},
		{name: "exhausted conflicts return error", exhausted: true, conflicts: 1000},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			scheme := runtime.NewScheme()
			if err := v1alpha1.AddToScheme(scheme); err != nil {
				t.Fatal(err)
			}
			if err := v1beta1.AddToScheme(scheme); err != nil {
				t.Fatal(err)
			}
			cluster := &v1beta1.KafkaCluster{ObjectMeta: v1.ObjectMeta{Name: "cluster", Namespace: "ns"}}
			operation := &v1alpha1.CruiseControlOperation{
				ObjectMeta: v1.ObjectMeta{
					Name: "operation", Namespace: "ns", UID: "operation-uid",
					Labels: map[string]string{v1beta1.KafkaCRLabelKey: "cluster"},
				},
				Spec: v1alpha1.CruiseControlOperationSpec{ErrorPolicy: v1alpha1.ErrorPolicyRetry},
			}
			operation.Status.ErrorPolicy = v1alpha1.ErrorPolicyRetry
			operation.Status.CurrentTask = &v1alpha1.CruiseControlTask{Operation: v1alpha1.OperationAddBroker}
			taskResult := &scale.Result{
				TaskID: "submitted-task", State: v1beta1.CruiseControlTaskInExecution,
				StartedAt: "Mon, 02 Jan 2006 15:04:05 GMT", ResponseStatusCode: 202,
				RequestURL: "http://cruise-control/add_broker",
			}
			var previous []*scale.Result
			if tt.retry {
				operation.Status.CurrentTask.ID = "failed-task"
				operation.Status.CurrentTask.State = v1beta1.CruiseControlTaskCompletedWithError
				operation.Status.CurrentTask.Finished = &v1.Time{Time: time.Date(2006, 1, 2, 15, 5, 0, 0, time.UTC)}
				operation.Status.RetryCount = 2
				operation.Status.FailedTasks = []v1alpha1.CruiseControlTask{{ID: "earlier-failed-task"}}
				previous = []*scale.Result{{TaskID: "failed-task", State: v1beta1.CruiseControlTaskCompletedWithError}}
			}
			if tt.deleting {
				operation.DeletionTimestamp = &v1.Time{Time: time.Now()}
				operation.Finalizers = []string{ccOperationFinalizerGroup}
				operation.Status.CurrentTask.ID = "running-task"
				operation.Status.CurrentTask.State = v1beta1.CruiseControlTaskInExecution
				previous = []*scale.Result{{TaskID: "running-task", State: v1beta1.CruiseControlTaskInExecution}}
			}
			scaler := mocks.NewMockCruiseControlScaler(gomock.NewController(t))
			scaler.EXPECT().Status(gomock.Any()).Return(scale.StatusTaskResult{Status: &scale.CruiseControlStatus{
				AnalyzerReady: true, MonitorReady: true, ExecutorReady: true,
			}}, nil)
			var taskIDs []any
			if len(previous) != 0 {
				taskIDs = []any{previous[0].TaskID}
			}
			scaler.EXPECT().UserTasks(gomock.Any(), taskIDs...).Return(previous, nil)
			if tt.deleting {
				scaler.EXPECT().StopExecution(gomock.Any()).Return(taskResult, nil).Times(1)
			} else {
				scaler.EXPECT().AddBrokersWithParams(gomock.Any(), operation.CurrentTaskParameters()).Return(taskResult, nil).Times(1)
			}
			var resultWrites int
			kubeClient := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(cluster, operation).
				WithStatusSubresource(operation).
				WithInterceptorFuncs(interceptor.Funcs{
					SubResourceUpdate: func(ctx context.Context, c client.Client, subresource string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
						if op, ok := obj.(*v1alpha1.CruiseControlOperation); ok && subresource == "status" && op.CurrentTaskID() == "submitted-task" {
							resultWrites++
							if resultWrites <= tt.conflicts {
								latest := &v1alpha1.CruiseControlOperation{}
								if err := c.Get(ctx, client.ObjectKeyFromObject(obj), latest); err != nil {
									return err
								}
								latest.Annotations = map[string]string{"other-writer": "preserved"}
								if err := c.Update(ctx, latest); err != nil {
									return err
								}
								if tt.replaced {
									latest.Finalizers = nil
									if err := c.Update(ctx, latest); err != nil {
										return err
									}
									if err := c.Delete(ctx, latest); err != nil {
										return err
									}
									latest.ResourceVersion = ""
									latest.UID = "replacement-uid"
									if err := c.Create(ctx, latest); err != nil {
										return err
									}
								}
								if tt.taskRemoved || tt.taskChanged {
									if tt.taskRemoved {
										latest.Status.CurrentTask = nil
									} else {
										latest.Status.CurrentTask.ID = "newer-task"
									}
									if err := c.Status().Update(ctx, latest); err != nil {
										return err
									}
								}
								return apierrors.NewConflict(schema.GroupResource{Group: "kafka.banzaicloud.io", Resource: "cruisecontroloperations"}, obj.GetName(), errors.New("injected concurrent update"))
							}
						}
						return c.SubResource(subresource).Update(ctx, obj, opts...)
					},
				}).Build()
			var directReads int
			directClient := interceptor.NewClient(kubeClient, interceptor.Funcs{
				Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
					directReads++
					assert.Positive(t, resultWrites, "direct GET must follow a conflicting result write")
					return c.Get(ctx, key, obj, opts...)
				},
			})
			reconciler := &CruiseControlOperationReconciler{
				Client: kubeClient, DirectClient: directClient, Scheme: scheme,
				ScaleFactory: func(context.Context, *v1beta1.KafkaCluster) (scale.CruiseControlScaler, error) {
					return scaler, nil
				},
			}
			_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(operation)})
			if err != nil && !tt.replaced && !tt.taskRemoved && !tt.taskChanged && !tt.exhausted {
				t.Fatal(err)
			}
			persisted := &v1alpha1.CruiseControlOperation{}
			if err := kubeClient.Get(ctx, client.ObjectKeyFromObject(operation), persisted); err != nil {
				t.Fatal(err)
			}
			if tt.replaced || tt.taskRemoved || tt.taskChanged || tt.exhausted {
				if err == nil {
					t.Error("reported success without persisting the result to the original task")
				}
				if persisted.CurrentTaskID() == "submitted-task" {
					t.Error("attached stale external result to a replaced operation or removed task")
				}
				return
			}
			assert.Equal(t, "submitted-task", persisted.CurrentTaskID())
			assert.Equal(t, v1beta1.CruiseControlTaskInExecution, persisted.CurrentTaskState())
			assert.Equal(t, tt.wantOp, persisted.CurrentTaskOperation())
			task := persisted.CurrentTask()
			require.NotNil(t, task)
			assert.Equal(t, taskResult.RequestURL, task.HTTPRequest)
			assert.Equal(t, &taskResult.ResponseStatusCode, task.HTTPResponseCode)
			require.NotNil(t, task.Started)
			assert.True(t, task.Started.Time.Equal(time.Date(2006, 1, 2, 15, 4, 5, 0, time.UTC)))
			assert.Equal(t, tt.wantRetries, persisted.Status.RetryCount)
			assert.Len(t, persisted.Status.FailedTasks, tt.wantFailed)
			if tt.conflicts > 0 {
				assert.Equal(t, "preserved", persisted.Annotations["other-writer"])
			}
			assert.Equal(t, tt.conflicts+1, resultWrites)
			assert.Equal(t, tt.conflicts, directReads)
		})
	}
}

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

// TestRemoveDisksParamsCurrentlyOnline exercises the re-validation predicate used before retrying a
// stalled OperationRemoveDisks task. The task's brokerid_and_logdirs parameter is a snapshot frozen at
// operation-creation time and replayed unchanged on every retry; if Cruise Control's own view of a
// broker's online log dirs has since diverged from that snapshot (e.g. because Cruise Control restarted
// and rebuilt its state while the request was created), blindly resubmitting the frozen snapshot repeats
// the same permanent rejection forever. This predicate re-checks the snapshot against a freshly fetched
// LogDirsByBroker result so a retry can tell whether Cruise Control's current state still agrees with it.
func TestRemoveDisksParamsCurrentlyOnline(t *testing.T) {
	testCases := []struct {
		testName        string
		params          map[string]string
		logDirsByBroker map[string]map[scale.LogDirState][]string
		expectedValid   bool
	}{
		{
			testName: "all requested log dirs are still online for their broker",
			params: map[string]string{
				scale.ParamBrokerIDAndLogDirs: "0-/kafka-logs2/kafka,1-/kafka-logs2/kafka",
			},
			logDirsByBroker: map[string]map[scale.LogDirState][]string{
				"0": {scale.LogDirStateOnline: {"/kafka-logs1/kafka", "/kafka-logs2/kafka"}},
				"1": {scale.LogDirStateOnline: {"/kafka-logs2/kafka"}},
			},
			expectedValid: true,
		},
		{
			testName: "a requested log dir is no longer online for its broker",
			params: map[string]string{
				scale.ParamBrokerIDAndLogDirs: "0-/kafka-logs2/kafka,1-/kafka-logs2/kafka",
			},
			logDirsByBroker: map[string]map[scale.LogDirState][]string{
				// Broker 0 no longer reports this log dir online (e.g. after a Cruise Control restart
				// reset its view) even though it was online when this snapshot was originally taken.
				"0": {scale.LogDirStateOnline: {"/kafka-logs1/kafka"}},
				"1": {scale.LogDirStateOnline: {"/kafka-logs2/kafka"}},
			},
			expectedValid: false,
		},
		{
			testName: "the broker is entirely absent from Cruise Control's current view",
			params: map[string]string{
				scale.ParamBrokerIDAndLogDirs: "0-/kafka-logs2/kafka",
			},
			logDirsByBroker: map[string]map[scale.LogDirState][]string{},
			expectedValid:   false,
		},
		{
			testName: "a requested log dir is reported offline rather than online",
			params: map[string]string{
				scale.ParamBrokerIDAndLogDirs: "0-/kafka-logs2/kafka",
			},
			logDirsByBroker: map[string]map[scale.LogDirState][]string{
				"0": {scale.LogDirStateOffline: {"/kafka-logs2/kafka"}},
			},
			expectedValid: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.testName, func(t *testing.T) {
			assert.Equal(t, tc.expectedValid, removeDisksParamsCurrentlyOnline(tc.params, tc.logDirsByBroker))
		})
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
