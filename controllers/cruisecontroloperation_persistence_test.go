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

package controllers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/banzaicloud/koperator/api/v1alpha1"
	"github.com/banzaicloud/koperator/api/v1beta1"
	mocks "github.com/banzaicloud/koperator/controllers/tests/mocks"
	"github.com/banzaicloud/koperator/pkg/scale"
)

func TestCruiseControlResultPersistence(t *testing.T) {
	const submittedID = "submitted-task"
	stale := func(err error) bool {
		return err != nil && strings.Contains(err.Error(), "changed after external task submission")
	}
	unavailable := apierrors.NewServiceUnavailable("cannot read operation")
	for _, tt := range []struct {
		name, mode string
		conflicts  int
		refreshErr error
		change     func(*v1alpha1.CruiseControlOperation)
		wantErr    func(error) bool
	}{
		{name: "first write uses the listed object"},
		{name: "conflict saves the accepted task once", conflicts: 1},
		{name: "retry keeps concurrent history", mode: "retry", conflicts: 2},
		{name: "successful stop is saved after conflict", mode: "stop", conflicts: 1},
		{name: "failed stop preserves running task", mode: "failed-stop"},
		{name: "failed stop preserves running task after conflict", mode: "failed-stop", conflicts: 1},
		{name: "different UID", conflicts: 1, wantErr: stale, change: func(op *v1alpha1.CruiseControlOperation) { op.UID = "replacement" }},
		{name: "missing task", conflicts: 1, wantErr: stale, change: func(op *v1alpha1.CruiseControlOperation) { op.Status.CurrentTask = nil }},
		{name: "different task ID", conflicts: 1, wantErr: stale, change: func(op *v1alpha1.CruiseControlOperation) { op.Status.CurrentTask.ID = "other-task" }},
		{name: "different cluster", conflicts: 1, wantErr: stale, change: func(op *v1alpha1.CruiseControlOperation) { op.Labels[v1beta1.KafkaCRLabelKey] = "other-cluster" }},
		{name: "different operation", conflicts: 1, wantErr: stale, change: func(op *v1alpha1.CruiseControlOperation) {
			op.Status.CurrentTask.Operation = v1alpha1.OperationRemoveBroker
		}},
		{name: "different parameters", conflicts: 1, wantErr: stale, change: func(op *v1alpha1.CruiseControlOperation) { op.Status.CurrentTask.Parameters["brokerid"] = "2" }},
		{name: "refresh failure is not retried", conflicts: 1, refreshErr: unavailable, wantErr: apierrors.IsServiceUnavailable},
		{name: "refresh cancellation is returned", conflicts: 1, refreshErr: context.Canceled, wantErr: func(err error) bool { return errors.Is(err, context.Canceled) }},
		{name: "exhausted conflicts return an error", conflicts: 6, wantErr: apierrors.IsConflict},
	} {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx := t.Context()
				scheme := runtime.NewScheme()
				require.NoError(t, v1alpha1.AddToScheme(scheme))
				require.NoError(t, v1beta1.AddToScheme(scheme))
				cluster := &v1beta1.KafkaCluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: "ns"}}
				op := &v1alpha1.CruiseControlOperation{ObjectMeta: metav1.ObjectMeta{
					Name: "operation", Namespace: "ns", UID: "original", Labels: map[string]string{v1beta1.KafkaCRLabelKey: cluster.Name},
				}}
				op.Spec.ErrorPolicy = v1alpha1.ErrorPolicyRetry
				op.Status.ErrorPolicy = v1alpha1.ErrorPolicyRetry
				op.Status.CurrentTask = &v1alpha1.CruiseControlTask{Operation: v1alpha1.OperationAddBroker, Parameters: map[string]string{"brokerid": "1"}}
				task := op.CurrentTask()
				stopping := tt.mode == "stop" || tt.mode == "failed-stop"
				if tt.mode == "retry" {
					task.ID, task.State = "failed-task", v1beta1.CruiseControlTaskCompletedWithError
					task.Finished = &metav1.Time{Time: time.Now().Add(-time.Hour)}
					op.Status.RetryCount = 2
					op.Status.FailedTasks = []v1alpha1.CruiseControlTask{{ID: "earlier-task"}}
				} else if stopping {
					task.ID, task.State = "running-task", v1beta1.CruiseControlTaskInExecution
					task.Started = &metav1.Time{Time: time.Now().Add(-time.Minute)}
					op.DeletionTimestamp = &metav1.Time{Time: time.Now()}
					op.Finalizers = []string{ccOperationFinalizerGroup}
					op.Status.RetryCount = 2
					for i := range defaultFailedTasksHistoryMaxLength {
						op.Status.FailedTasks = append(op.Status.FailedTasks, v1alpha1.CruiseControlTask{ID: fmt.Sprint(i)})
					}
				}
				key := client.ObjectKeyFromObject(op)
				scaler := mocks.NewMockCruiseControlScaler(gomock.NewController(t))
				scaler.EXPECT().Status(gomock.Any()).Return(scale.StatusTaskResult{Status: &scale.CruiseControlStatus{
					AnalyzerReady: true, MonitorReady: true, ExecutorReady: true,
				}}, nil)
				if task.ID == "" {
					scaler.EXPECT().UserTasks(gomock.Any()).Return(nil, nil)
				} else {
					scaler.EXPECT().UserTasks(gomock.Any(), task.ID).Return([]*scale.Result{{TaskID: task.ID, State: task.State}}, nil)
				}
				result := &scale.Result{TaskID: submittedID, State: v1beta1.CruiseControlTaskInExecution, StartedAt: "Mon, 02 Jan 2006 15:04:05 GMT"}
				switch tt.mode {
				case "failed-stop":
					stopErr := errors.New("stop failed")
					scaler.EXPECT().StopExecution(gomock.Any()).Return(&scale.Result{State: v1beta1.CruiseControlTaskCompletedWithError, Err: stopErr}, stopErr).Times(1)
				case "stop":
					result.State = v1beta1.CruiseControlTaskActive
					scaler.EXPECT().StopExecution(gomock.Any()).Return(result, nil).Times(1)
				default:
					scaler.EXPECT().AddBrokersWithParams(gomock.Any(), map[string]string{"brokerid": "1"}).Return(result, nil).Times(1)
				}
				var writes, reads int
				var concurrent *v1alpha1.CruiseControlOperation
				store := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster, op).WithStatusSubresource(op).
					WithInterceptorFuncs(interceptor.Funcs{SubResourceUpdate: func(ctx context.Context, c client.Client, sub string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
						persisted := obj.(*v1alpha1.CruiseControlOperation)
						history := persisted.Status.FailedTasks
						failedStopWrite := tt.mode == "failed-stop" && (persisted.CurrentTaskID() != task.ID ||
							(len(history) > 0 && history[len(history)-1].Operation == v1alpha1.OperationStopExecution))
						if persisted.CurrentTaskID() == submittedID || failedStopWrite {
							writes++
							if writes <= tt.conflicts {
								concurrent = &v1alpha1.CruiseControlOperation{}
								require.NoError(t, c.Get(ctx, key, concurrent))
								if tt.mode == "retry" {
									concurrent.Status.FailedTasks[0].ErrorMessage = "edited concurrently"
									require.NoError(t, c.Status().Update(ctx, concurrent))
								}
								return apierrors.NewConflict(v1alpha1.GroupVersion.WithResource("cruisecontroloperations").GroupResource(), key.Name, errors.New("injected conflict"))
							}
						}
						return c.SubResource(sub).Update(ctx, obj, opts...)
					}}).Build()
				direct := interceptor.NewClient(store, interceptor.Funcs{
					List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
						options := (&client.ListOptions{}).ApplyOptions(opts)
						require.Equal(t, "ns", options.Namespace)
						if tt.conflicts == 0 && tt.mode == "" {
							require.Nil(t, options.LabelSelector, "scheduling uses the full namespace list")
						}
						return c.List(ctx, list, opts...)
					},
					Get: func(ctx context.Context, c client.WithWatch, k client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
						reads++
						require.Positive(t, writes, "refresh only after a conflicting write")
						if tt.refreshErr != nil {
							return tt.refreshErr
						}
						if err := c.Get(ctx, k, obj, opts...); err != nil {
							return err
						}
						if tt.change != nil {
							tt.change(obj.(*v1alpha1.CruiseControlOperation))
						}
						return nil
					},
				})
				r := &CruiseControlOperationReconciler{Client: store, DirectClient: direct,
					ScaleFactory: func(context.Context, *v1beta1.KafkaCluster) (scale.CruiseControlScaler, error) { return scaler, nil }}
				out, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: key})
				saved := &v1alpha1.CruiseControlOperation{}
				require.NoError(t, store.Get(ctx, key, saved))
				if tt.wantErr != nil {
					require.Error(t, err)
					require.True(t, tt.wantErr(err), "unexpected error: %v", err)
					require.NotNil(t, concurrent)
					require.Equal(t, concurrent.Status, saved.Status)
					require.Equal(t, min(tt.conflicts, 5), reads)
					require.Equal(t, tt.conflicts, writes)
					return
				}
				require.NoError(t, err)
				require.Equal(t, tt.conflicts, reads)
				require.Equal(t, tt.conflicts+1, writes)
				if tt.mode == "failed-stop" {
					require.Equal(t, 10*time.Second, out.RequeueAfter)
					require.Contains(t, saved.Finalizers, ccOperationFinalizerGroup)
					require.Equal(t, task, saved.CurrentTask())
					require.EqualValues(t, 2, saved.Status.RetryCount)
					require.Len(t, saved.Status.FailedTasks, defaultFailedTasksHistoryMaxLength)
					require.Equal(t, op.Status.FailedTasks[1:], saved.Status.FailedTasks[:defaultFailedTasksHistoryMaxLength-1])
					stop := saved.Status.FailedTasks[defaultFailedTasksHistoryMaxLength-1]
					require.Equal(t, v1alpha1.OperationStopExecution, stop.Operation)
					require.Equal(t, v1beta1.CruiseControlTaskCompletedWithError, stop.State)
					require.Equal(t, "stop failed", stop.ErrorMessage)
					return
				}
				require.Equal(t, submittedID, saved.CurrentTaskID())
				require.Equal(t, result.State, saved.CurrentTaskState())
				switch tt.mode {
				case "stop":
					require.Equal(t, v1alpha1.OperationStopExecution, saved.CurrentTaskOperation())
				case "retry":
					require.EqualValues(t, 3, saved.Status.RetryCount)
					require.Len(t, saved.Status.FailedTasks, 2)
					require.Equal(t, "edited concurrently", saved.Status.FailedTasks[0].ErrorMessage)
					require.Equal(t, "failed-task", saved.Status.FailedTasks[1].ID)
				}
			})
		})
	}
}
