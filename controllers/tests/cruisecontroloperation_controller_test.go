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

package tests

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/banzaicloud/koperator/controllers"
	"github.com/banzaicloud/koperator/controllers/tests/mocks"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/banzaicloud/koperator/api/util"
	"github.com/banzaicloud/koperator/api/v1alpha1"
	"github.com/banzaicloud/koperator/api/v1beta1"
	"github.com/banzaicloud/koperator/pkg/scale"
)

const (
	// Additional test timeout constants for operation controller tests
	operationRetryTimeoutDuration    = time.Duration(v1alpha1.DefaultRetryBackOffDurationSec+10) * time.Second // timeout for retry operations
	operationExtendedTimeoutDuration = 15 * time.Second                                                        // extended timeout for complex operations
)

type operationRetrievalHintClient struct {
	client.Client
	hint *v1alpha1.CruiseControlOperation
	miss bool
}

func (c *operationRetrievalHintClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if operation, ok := obj.(*v1alpha1.CruiseControlOperation); ok {
		if c.miss {
			return apierrors.NewNotFound(schema.GroupResource{Group: "kafka.banzaicloud.io", Resource: "cruisecontroloperations"}, key.Name)
		}
		if c.hint != nil {
			c.hint.DeepCopyInto(operation)
			return nil
		}
	}
	return c.Client.Get(ctx, key, obj, opts...)
}

type operationRetrievalReader struct {
	client.Reader
	selectors []string
	results   []v1alpha1.CruiseControlOperationList
	gets      int
}

func (r *operationRetrievalReader) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	r.gets++
	return r.Reader.Get(ctx, key, obj, opts...)
}

func (r *operationRetrievalReader) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	options := (&client.ListOptions{}).ApplyOptions(opts)
	selector := ""
	if options.LabelSelector != nil {
		selector = options.LabelSelector.String()
	}
	r.selectors = append(r.selectors, selector)
	if err := r.Reader.List(ctx, list, opts...); err != nil {
		return err
	}
	r.results = append(r.results, *list.(*v1alpha1.CruiseControlOperationList).DeepCopy())
	return nil
}

var _ = Describe("CruiseControlOperation scoped retrieval", Serial, func() {
	DescribeTable("uses fresh API objects and preserves compatible fallback", func(ctx SpecContext, mode string) {
		namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "operation-retrieval-"}}
		Expect(k8sClient.Create(ctx, namespace)).To(Succeed())
		DeferCleanup(func(ctx SpecContext) {
			Expect(k8sClient.DeleteAllOf(ctx, &v1alpha1.CruiseControlOperation{}, client.InNamespace(namespace.Name))).To(Succeed())
			Expect(k8sClient.Delete(ctx, namespace)).To(Succeed())
		})
		var current *v1alpha1.CruiseControlOperation
		for _, name := range []string{"z-current", "a-peer", "unrelated"} {
			cluster := "target"
			if name == "unrelated" {
				cluster = "old"
			}
			operation := generateCruiseControlOperation(name, namespace.Name, cluster)
			Expect(k8sClient.Create(ctx, &operation)).To(Succeed())
			operation.Status.CurrentTask = &v1alpha1.CruiseControlTask{Operation: v1alpha1.OperationStatus}
			Expect(k8sClient.Status().Update(ctx, &operation)).To(Succeed())
			if name == "z-current" {
				current = operation.DeepCopy()
			}
		}
		cached := &operationRetrievalHintClient{Client: cruiseControlOperationReconciler.Client}
		Eventually(ctx, func(g Gomega) {
			observed := &v1alpha1.CruiseControlOperation{}
			g.Expect(cached.Get(ctx, client.ObjectKeyFromObject(current), observed)).To(Succeed())
			g.Expect(observed.CurrentTaskOperation()).To(Equal(v1alpha1.OperationStatus))
		}).Should(Succeed())
		switch mode {
		case "miss":
			cached.miss = true
		case "stale":
			cached.hint = current.DeepCopy()
			cached.hint.Labels[v1beta1.KafkaCRLabelKey] = "old"
		case "unlabeled-hint":
			cached.hint = current.DeepCopy()
			cached.hint.Labels = nil
		case "legacy":
			current.Labels = nil
			Expect(k8sClient.Update(ctx, current)).To(Succeed())
			current.Status.CurrentTask.Operation = v1alpha1.OperationAddBroker
			Expect(k8sClient.Status().Update(ctx, current)).To(Succeed())
			cached.hint = current.DeepCopy()
		case "recreated":
			cached.hint = current.DeepCopy()
			Expect(k8sClient.Delete(ctx, current)).To(Succeed())
			replacement := generateCruiseControlOperation(current.Name, namespace.Name, "target")
			Expect(k8sClient.Create(ctx, &replacement)).To(Succeed())
			replacement.Status.CurrentTask = &v1alpha1.CruiseControlTask{Operation: v1alpha1.OperationStatus}
			Expect(k8sClient.Status().Update(ctx, &replacement)).To(Succeed())
			Expect(replacement.UID).NotTo(Equal(current.UID))
			current = replacement.DeepCopy()
		}
		baseline := &v1alpha1.CruiseControlOperationList{}
		Expect(k8sClient.List(ctx, baseline, client.InNamespace(namespace.Name))).To(Succeed())
		reader := &operationRetrievalReader{Reader: k8sClient}
		reconciler := &controllers.CruiseControlOperationReconciler{Client: cached, DirectClient: reader}
		result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(current)})
		if mode == "legacy" {
			Expect(err).To(MatchError(ContainSubstring("could not find kafka cluster reference label")))
		} else {
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeZero())
		}
		scopedSelector := v1beta1.KafkaCRLabelKey + "=target"
		switch mode {
		case "miss", "unlabeled-hint", "legacy":
			Expect(reader.selectors).To(Equal([]string{""}))
		case "stale":
			Expect(reader.selectors).To(Equal([]string{v1beta1.KafkaCRLabelKey + "=old", ""}))
		default:
			Expect(reader.selectors).To(Equal([]string{scopedSelector}))
		}
		expected := make([]string, 0)
		for _, operation := range baseline.Items {
			if len(reader.selectors) != 1 || reader.selectors[0] == "" || operation.GetClusterRef() == "target" {
				expected = append(expected, operation.Name)
			}
		}
		last := reader.results[len(reader.results)-1]
		observed := make([]string, 0, len(last.Items))
		for _, operation := range last.Items {
			observed = append(observed, operation.Name)
			if operation.Name == current.Name {
				Expect(operation.UID).To(Equal(current.UID))
				Expect(operation.Status).To(Equal(current.Status))
			}
		}
		Expect(observed).To(Equal(expected))
		Expect(reader.gets).To(BeZero())
	},
		Entry("warm cache", "cached"),
		Entry("cache miss", "miss"),
		Entry("reassigned label", "stale"),
		Entry("unlabeled hint", "unlabeled-hint"),
		Entry("unlabeled legacy object", "legacy"),
		Entry("deleted and recreated object", "recreated"),
	)
})

var _ = Describe("CruiseControlTaskReconciler", func() {
	var (
		count              uint64 = 0
		namespace          string
		namespaceObj       *corev1.Namespace
		kafkaClusterCRName string
		kafkaCluster       *v1beta1.KafkaCluster
		opName1            = "operation1"
		opName2            = "operation2"
	)
	BeforeEach(func() {
		atomic.AddUint64(&count, 1)

		namespace = fmt.Sprintf("cruisecontroloperation-%v", count)
		namespaceObj = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}
		kafkaClusterCRName = fmt.Sprintf("kafkacluster-%v", count)
		kafkaCluster = createMinimalKafkaClusterCR(kafkaClusterCRName, namespace)
	})

	JustBeforeEach(func(ctx SpecContext) {
		By("creating namespace " + namespace)
		err := k8sClient.Create(ctx, namespaceObj)
		Expect(err).NotTo(HaveOccurred())

		By("creating kafka cluster object " + kafkaCluster.Name + " in namespace " + namespace)
		err = k8sClient.Create(ctx, kafkaCluster)
		Expect(err).NotTo(HaveOccurred())

	})
	JustAfterEach(func(ctx SpecContext) {
		// Clean up any CruiseControlOperations first
		err := k8sClient.DeleteAllOf(ctx, &v1alpha1.CruiseControlOperation{}, client.InNamespace(namespace))
		Expect(err).NotTo(HaveOccurred())

		// Wait for operations to be deleted before resetting the factory
		Eventually(ctx, func() int {
			operationList := &v1alpha1.CruiseControlOperationList{}
			err := k8sClient.List(ctx, operationList, client.InNamespace(namespace))
			if err != nil {
				return -1
			}
			return len(operationList.Items)
		}).Should(Equal(0))

		// Reset the ScaleFactory to prevent interference with other tests
		// Use NoopScaleFactory instead of error-returning factory to avoid race conditions
		cruiseControlOperationReconciler.ScaleFactory = mocks.NewNoopScaleFactory()
	})
	When("there is an add_broker operation for execution", Serial, func() {
		JustBeforeEach(func(ctx SpecContext) {
			cruiseControlOperationReconciler.ScaleFactory = mocks.NewMockScaleFactory(getScaleMock1())
			operation := generateCruiseControlOperation(opName1, namespace, kafkaCluster.GetName())
			err := k8sClient.Create(ctx, &operation)
			Expect(err).NotTo(HaveOccurred())

			operation.Status.CurrentTask = &v1alpha1.CruiseControlTask{
				Operation: v1alpha1.OperationAddBroker,
			}
			err = k8sClient.Status().Update(ctx, &operation)
			Expect(err).NotTo(HaveOccurred())

		})
		It("should execute the task and the task later will be completed", func(ctx SpecContext) {
			Eventually(ctx, func() v1beta1.CruiseControlUserTaskState {
				operation := v1alpha1.CruiseControlOperation{}
				err := k8sClient.Get(ctx, client.ObjectKey{
					Namespace: kafkaCluster.Namespace,
					Name:      opName1,
				}, &operation)
				if err != nil {
					return ""
				}
				return operation.CurrentTaskState()
			}, maxReconcileDuration, reconcilePollingPeriod).Should(Equal(v1beta1.CruiseControlTaskCompleted))
		})
	})
	When("add_broker operation is finished with completedWithError and 30s has not elapsed", Serial, func() {
		JustBeforeEach(func(ctx SpecContext) {
			cruiseControlOperationReconciler.ScaleFactory = mocks.NewMockScaleFactory(getScaleMock2())
			operation := generateCruiseControlOperation(opName1, namespace, kafkaCluster.GetName())
			err := k8sClient.Create(ctx, &operation)
			Expect(err).NotTo(HaveOccurred())

			operation.Status.CurrentTask = &v1alpha1.CruiseControlTask{
				Operation: v1alpha1.OperationAddBroker,
			}
			err = k8sClient.Status().Update(ctx, &operation)
			Expect(err).NotTo(HaveOccurred())
		})
		It("should not retry the failed task", func(ctx SpecContext) {
			Eventually(ctx, func() bool {
				operation := v1alpha1.CruiseControlOperation{}
				err := k8sClient.Get(ctx, client.ObjectKey{
					Namespace: kafkaCluster.Namespace,
					Name:      opName1,
				}, &operation)
				if err != nil {
					return false
				}
				return operation.CurrentTaskState() == v1beta1.CruiseControlTaskCompletedWithError && len(operation.Status.FailedTasks) == 0
			}, maxReconcileDuration, reconcilePollingPeriod).Should(BeTrue())
		})
	})
	When("add_broker operation is finished with completedWithError and 30s has elapsed", Serial, func() {
		JustBeforeEach(func(ctx SpecContext) {
			cruiseControlOperationReconciler.ScaleFactory = mocks.NewMockScaleFactory(getScaleMock5())
			operation := generateCruiseControlOperation(opName1, namespace, kafkaCluster.GetName())
			// Explicitly set error policy to retry to ensure retry behavior
			operation.Spec.ErrorPolicy = v1alpha1.ErrorPolicyRetry
			err := k8sClient.Create(ctx, &operation)
			Expect(err).NotTo(HaveOccurred())
			operation.Status.CurrentTask = &v1alpha1.CruiseControlTask{
				ID:        "12345",
				Operation: v1alpha1.OperationAddBroker,
				State:     v1beta1.CruiseControlTaskCompletedWithError,
				Finished:  &metav1.Time{Time: time.Now().Add(-time.Second*v1alpha1.DefaultRetryBackOffDurationSec - 10)},
			}
			err = k8sClient.Status().Update(ctx, &operation)
			Expect(err).NotTo(HaveOccurred())
		})
		It("should retry the failed task", func(ctx SpecContext) {
			Eventually(ctx, func() bool {
				operation := v1alpha1.CruiseControlOperation{}
				err := k8sClient.Get(ctx, client.ObjectKey{
					Namespace: kafkaCluster.Namespace,
					Name:      opName1,
				}, &operation)
				if err != nil {
					fmt.Printf("Error getting operation: %v\n", err)
					return false
				}

				// Debug logging
				fmt.Printf("Operation state: %s, FailedTasks: %d, IsWaitingForRetryExecution: %v, IsReadyForRetryExecution: %v\n",
					operation.CurrentTaskState(), len(operation.Status.FailedTasks),
					operation.IsWaitingForRetryExecution(), operation.IsReadyForRetryExecution())

				return operation.CurrentTaskState() == v1beta1.CruiseControlTaskCompleted && len(operation.Status.FailedTasks) == 1
			}, operationRetryTimeoutDuration, reconcilePollingPeriod).Should(BeTrue())
		})
	})
	When("there is an errored remove_broker and an add_broker operation", Serial, func() {
		JustBeforeEach(func(ctx SpecContext) {
			cruiseControlOperationReconciler.ScaleFactory = mocks.NewMockScaleFactory(getScaleMock3())
			// First operation will get completedWithError
			operation := generateCruiseControlOperation(opName1, namespace, kafkaCluster.GetName())
			err := k8sClient.Create(ctx, &operation)
			Expect(err).NotTo(HaveOccurred())

			operation.Status.CurrentTask = &v1alpha1.CruiseControlTask{
				Operation: v1alpha1.OperationRemoveBroker,
				Finished:  &metav1.Time{Time: time.Now().Add(-time.Second*v1alpha1.DefaultRetryBackOffDurationSec - 10)},
			}
			err = k8sClient.Status().Update(ctx, &operation)
			Expect(err).NotTo(HaveOccurred())
			// Creating other operation
			operation = generateCruiseControlOperation(opName2, namespace, kafkaCluster.GetName())
			err = k8sClient.Create(ctx, &operation)
			Expect(err).NotTo(HaveOccurred())
			operation.Status.CurrentTask = &v1alpha1.CruiseControlTask{
				Operation: v1alpha1.OperationAddBroker,
			}
			err = k8sClient.Status().Update(ctx, &operation)
			Expect(err).NotTo(HaveOccurred())
		})
		It("should not retry the errored task but should execute the add_broker", func(ctx SpecContext) {
			Eventually(ctx, func() bool {
				operation1 := v1alpha1.CruiseControlOperation{}
				err := k8sClient.Get(ctx, client.ObjectKey{
					Namespace: kafkaCluster.Namespace,
					Name:      opName1,
				}, &operation1)
				if err != nil {
					return false
				}
				operation2 := v1alpha1.CruiseControlOperation{}
				err = k8sClient.Get(ctx, client.ObjectKey{
					Namespace: kafkaCluster.Namespace,
					Name:      opName2,
				}, &operation2)
				if err != nil {
					return false
				}

				return operation2.CurrentTaskState() == v1beta1.CruiseControlTaskCompleted
			}, maxReconcileDuration, reconcilePollingPeriod).Should(BeTrue())
		})
	})
	When("there is a new remove_broker and an errored remove_broker operation with pause annotation", Serial, func() {
		JustBeforeEach(func(ctx SpecContext) {
			cruiseControlOperationReconciler.ScaleFactory = mocks.NewMockScaleFactory(getScaleMock4())
			operation := generateCruiseControlOperation(opName1, namespace, kafkaCluster.GetName())
			operation.Labels["pause"] = "true"
			err := k8sClient.Create(ctx, &operation)
			Expect(err).NotTo(HaveOccurred())

			operation.Status.CurrentTask = &v1alpha1.CruiseControlTask{
				ID:        "12345",
				Operation: v1alpha1.OperationRemoveBroker,
				State:     v1beta1.CruiseControlTaskCompletedWithError,
				Finished:  &metav1.Time{Time: time.Now().Add(-time.Second*v1alpha1.DefaultRetryBackOffDurationSec - 10)},
			}
			err = k8sClient.Status().Update(ctx, &operation)
			Expect(err).NotTo(HaveOccurred())
			// Creating the second operation
			operation = generateCruiseControlOperation(opName2, namespace, kafkaCluster.GetName())
			err = k8sClient.Create(ctx, &operation)
			Expect(err).NotTo(HaveOccurred())
			operation.Status.CurrentTask = &v1alpha1.CruiseControlTask{
				Operation: v1alpha1.OperationRemoveBroker,
			}
			err = k8sClient.Status().Update(ctx, &operation)
			Expect(err).NotTo(HaveOccurred())
		})
		It("should execute new remove_broker operation and should not retry the other one with pause annotation", func(ctx SpecContext) {
			Eventually(ctx, func() bool {
				operation1 := v1alpha1.CruiseControlOperation{}
				err := k8sClient.Get(ctx, client.ObjectKey{
					Namespace: kafkaCluster.Namespace,
					Name:      opName1,
				}, &operation1)
				if err != nil {
					return false
				}
				operation2 := v1alpha1.CruiseControlOperation{}
				err = k8sClient.Get(ctx, client.ObjectKey{
					Namespace: kafkaCluster.Namespace,
					Name:      opName2,
				}, &operation2)
				if err != nil {
					return false
				}

				return operation1.Status.RetryCount == 0 && operation2.CurrentTaskState() == v1beta1.CruiseControlTaskCompleted
			}, maxReconcileDuration, reconcilePollingPeriod).Should(BeTrue())
		})
	})
	When("there is a new remove_broker and an errored remove_broker operation with ignore ErrorPolicy", Serial, func() {
		JustBeforeEach(func(ctx SpecContext) {
			cruiseControlOperationReconciler.ScaleFactory = mocks.NewMockScaleFactory(getScaleMock4())
			// Creating first operation
			operation := generateCruiseControlOperation(opName1, namespace, kafkaCluster.GetName())
			operation.Spec.ErrorPolicy = v1alpha1.ErrorPolicyIgnore
			err := k8sClient.Create(ctx, &operation)
			Expect(err).NotTo(HaveOccurred())

			operation.Status.CurrentTask = &v1alpha1.CruiseControlTask{
				ID:        "12345",
				Finished:  &metav1.Time{Time: time.Now().Add(-time.Second*v1alpha1.DefaultRetryBackOffDurationSec - 10)},
				Operation: v1alpha1.OperationRemoveBroker,
				State:     v1beta1.CruiseControlTaskCompletedWithError,
			}
			err = k8sClient.Status().Update(ctx, &operation)
			Expect(err).NotTo(HaveOccurred())
			// Creating the second operation
			operation = generateCruiseControlOperation(opName2, namespace, kafkaCluster.GetName())
			err = k8sClient.Create(ctx, &operation)
			Expect(err).NotTo(HaveOccurred())
			operation.Status.CurrentTask = &v1alpha1.CruiseControlTask{
				Operation: v1alpha1.OperationRemoveBroker,
			}
			err = k8sClient.Status().Update(ctx, &operation)
			Expect(err).NotTo(HaveOccurred())
		})
		It("should execute the new remove_broker operation and should not execute the errored one", func(ctx SpecContext) {
			Eventually(ctx, func() bool {
				operation1 := v1alpha1.CruiseControlOperation{}
				err := k8sClient.Get(ctx, client.ObjectKey{
					Namespace: kafkaCluster.Namespace,
					Name:      opName1,
				}, &operation1)
				if err != nil {
					return false
				}
				operation2 := v1alpha1.CruiseControlOperation{}
				err = k8sClient.Get(ctx, client.ObjectKey{
					Namespace: kafkaCluster.Namespace,
					Name:      opName2,
				}, &operation2)
				if err != nil {
					return false
				}

				return operation1.Status.RetryCount == 0 && operation2.CurrentTaskState() == v1beta1.CruiseControlTaskCompleted
			}, maxReconcileDuration, reconcilePollingPeriod).Should(BeTrue())
		})
	})
	When("there is an errored remove_disks and a rebalance disks operation for the same broker", Serial, func() {
		JustBeforeEach(func() {
			cruiseControlOperationReconciler.ScaleFactory = mocks.NewMockScaleFactory(getScaleMock6())
			// Remove_disk operation - errored
			operation := generateCruiseControlOperation(opName1, namespace, kafkaCluster.GetName())
			err := k8sClient.Create(context.Background(), &operation)
			Expect(err).NotTo(HaveOccurred())

			operation.Status.CurrentTask = &v1alpha1.CruiseControlTask{
				Operation: v1alpha1.OperationRemoveDisks,
				Finished:  &metav1.Time{Time: time.Now().Add(-time.Second*v1alpha1.DefaultRetryBackOffDurationSec - 10)},
				Parameters: map[string]string{
					scale.ParamBrokerIDAndLogDirs: "101-logdir1",
				},
			}
			err = k8sClient.Status().Update(context.Background(), &operation)
			Expect(err).NotTo(HaveOccurred())
			// Rebalance operation
			operation = generateCruiseControlOperation(opName2, namespace, kafkaCluster.GetName())
			err = k8sClient.Create(context.Background(), &operation)
			Expect(err).NotTo(HaveOccurred())
			operation.Status.CurrentTask = &v1alpha1.CruiseControlTask{
				Operation: v1alpha1.OperationRebalance,
				Parameters: map[string]string{
					scale.ParamDestbrokerIDs: "101,102",
				},
			}
			err = k8sClient.Status().Update(context.Background(), &operation)
			Expect(err).NotTo(HaveOccurred())
		})
		It("should mark the removed disk task as paused and should execute the rebalance", func() {
			Eventually(func() bool {
				removeDisksOp := v1alpha1.CruiseControlOperation{}
				err := k8sClient.Get(context.Background(), client.ObjectKey{
					Namespace: kafkaCluster.Namespace,
					Name:      opName1,
				}, &removeDisksOp)
				if err != nil {
					return false
				}
				rebalanceOp := v1alpha1.CruiseControlOperation{}
				err = k8sClient.Get(context.Background(), client.ObjectKey{
					Namespace: kafkaCluster.Namespace,
					Name:      opName2,
				}, &rebalanceOp)
				if err != nil {
					return false
				}

				return rebalanceOp.CurrentTaskState() == v1beta1.CruiseControlTaskCompleted &&
					removeDisksOp.GetLabels()[v1alpha1.PauseLabel] == v1alpha1.True
			}, maxReconcileDuration, reconcilePollingPeriod).Should(BeTrue())
		})
	})
	When("Cruise Control makes the Status operation async", Serial, func() {
		JustBeforeEach(func(ctx SpecContext) {
			cruiseControlOperationReconciler.ScaleFactory = mocks.NewMockScaleFactory(getScaleMock7())
			operation := generateCruiseControlOperation("add-broker-operation", namespace, kafkaCluster.GetName())
			err := k8sClient.Create(ctx, &operation)
			Expect(err).NotTo(HaveOccurred())

			operation.Status.CurrentTask = &v1alpha1.CruiseControlTask{
				Operation: v1alpha1.OperationAddBroker,
				ID:        "11111",
			}
			err = k8sClient.Status().Update(ctx, &operation)
			Expect(err).NotTo(HaveOccurred())
		})
		It("should create status CruiseControlOperation and retry", func(ctx SpecContext) {
			Eventually(ctx, func() v1beta1.CruiseControlUserTaskState {
				operation := v1alpha1.CruiseControlOperation{}
				err := k8sClient.Get(ctx, client.ObjectKey{
					Namespace: kafkaCluster.Namespace,
					Name:      "add-broker-operation",
				}, &operation)
				if err != nil {
					return ""
				}
				return operation.CurrentTaskState()
			}, operationExtendedTimeoutDuration, reconcilePollingPeriod).Should(Equal(v1beta1.CruiseControlTaskCompleted))
		})
	})
})

func generateCruiseControlOperation(name, namespace, kafkaRef string) v1alpha1.CruiseControlOperation {
	return v1alpha1.CruiseControlOperation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    util.LabelsForKafka(kafkaRef),
		},
		Spec: v1alpha1.CruiseControlOperationSpec{},
	}
}
func getScaleMock2() *mocks.MockCruiseControlScaler {
	mockCtrl := gomock.NewController(GinkgoT())
	scaleMock := mocks.NewMockCruiseControlScaler(mockCtrl)
	scaleMock.EXPECT().IsUp(gomock.Any()).Return(true).AnyTimes()

	userTaskResult := []*scale.Result{scaleResultPointer(scale.Result{
		TaskID:    "12345",
		StartedAt: "Sat, 27 Aug 2022 12:22:21 GMT",
		State:     v1beta1.CruiseControlTaskCompletedWithError,
	})}
	scaleMock.EXPECT().UserTasks(gomock.Any(), gomock.Any()).Return(userTaskResult, nil).AnyTimes()
	scaleMock.EXPECT().Status(gomock.Any()).Return(scale.StatusTaskResult{
		Status: &scale.CruiseControlStatus{
			ExecutorReady: true,
			MonitorReady:  true,
			AnalyzerReady: true,
		}}, nil).AnyTimes()
	scaleMock.EXPECT().AddBrokersWithParams(gomock.Any(), gomock.All()).Return(scaleResultPointer(scale.Result{
		TaskID:    "12345",
		StartedAt: "Sat, 27 Aug 2022 12:22:21 GMT",
		State:     v1beta1.CruiseControlTaskActive,
	}), nil).Times(1)
	return scaleMock
}
func getScaleMock1() *mocks.MockCruiseControlScaler {
	mockCtrl := gomock.NewController(GinkgoT())
	scaleMock := mocks.NewMockCruiseControlScaler(mockCtrl)
	scaleMock.EXPECT().IsUp(gomock.Any()).Return(true).AnyTimes()

	userTaskResult := []*scale.Result{scaleResultPointer(scale.Result{
		TaskID:    "12345",
		StartedAt: "Sat, 27 Aug 2022 12:22:21 GMT",
		State:     v1beta1.CruiseControlTaskCompleted,
	})}
	scaleMock.EXPECT().UserTasks(gomock.Any(), gomock.Any()).Return(userTaskResult, nil).AnyTimes()
	scaleMock.EXPECT().Status(gomock.Any()).Return(scale.StatusTaskResult{
		Status: &scale.CruiseControlStatus{
			ExecutorReady: true,
			MonitorReady:  true,
			AnalyzerReady: true,
		}}, nil).AnyTimes()
	scaleMock.EXPECT().AddBrokersWithParams(gomock.Any(), gomock.All()).Return(scaleResultPointer(scale.Result{
		TaskID:    "12345",
		StartedAt: "Sat, 27 Aug 2022 12:22:21 GMT",
		State:     v1beta1.CruiseControlTaskActive,
	}), nil)
	return scaleMock
}

func getScaleMock3() *mocks.MockCruiseControlScaler {
	mockCtrl := gomock.NewController(GinkgoT())
	scaleMock := mocks.NewMockCruiseControlScaler(mockCtrl)
	scaleMock.EXPECT().IsUp(gomock.Any()).Return(true).AnyTimes()

	userTaskResult := []*scale.Result{scaleResultPointer(scale.Result{
		TaskID:    "12345",
		StartedAt: "Sat, 27 Aug 2022 12:22:21 GMT",
		State:     v1beta1.CruiseControlTaskCompletedWithError,
	}), scaleResultPointer(scale.Result{
		TaskID:    "2",
		StartedAt: "Sat, 27 Aug 2022 12:22:21 GMT",
		State:     v1beta1.CruiseControlTaskCompleted,
	})}
	scaleMock.EXPECT().UserTasks(gomock.Any(), gomock.Any()).Return(userTaskResult, nil).AnyTimes()
	scaleMock.EXPECT().Status(gomock.Any()).Return(scale.StatusTaskResult{
		Status: &scale.CruiseControlStatus{
			ExecutorReady: true,
			MonitorReady:  true,
			AnalyzerReady: true,
		}}, nil).AnyTimes()
	scaleMock.EXPECT().RemoveBrokersWithParams(gomock.Any(), gomock.All()).Return(scaleResultPointer(scale.Result{
		TaskID:    "12345",
		StartedAt: "Sat, 27 Aug 2022 12:22:21 GMT",
		State:     v1beta1.CruiseControlTaskActive,
	}), nil).AnyTimes()
	scaleMock.EXPECT().AddBrokersWithParams(gomock.Any(), gomock.All()).Return(scaleResultPointer(scale.Result{
		TaskID:    "2",
		StartedAt: "Sat, 27 Aug 2022 12:22:21 GMT",
		State:     v1beta1.CruiseControlTaskActive,
	}), nil).Times(1)

	return scaleMock
}

func getScaleMock4() *mocks.MockCruiseControlScaler {
	mockCtrl := gomock.NewController(GinkgoT())
	scaleMock := mocks.NewMockCruiseControlScaler(mockCtrl)
	scaleMock.EXPECT().IsUp(gomock.Any()).Return(true).AnyTimes()

	userTaskResult := []*scale.Result{scaleResultPointer(scale.Result{
		TaskID:    "1",
		StartedAt: "Sat, 27 Aug 2022 12:22:21 GMT",
		State:     v1beta1.CruiseControlTaskCompleted,
	}), scaleResultPointer(scale.Result{
		TaskID:    "12345",
		StartedAt: "Sat, 27 Aug 2022 12:22:21 GMT",
		State:     v1beta1.CruiseControlTaskCompletedWithError,
	})}
	scaleMock.EXPECT().UserTasks(gomock.Any(), gomock.Any()).Return(userTaskResult, nil).AnyTimes()
	scaleMock.EXPECT().Status(gomock.Any()).Return(scale.StatusTaskResult{
		Status: &scale.CruiseControlStatus{
			ExecutorReady: true,
			MonitorReady:  true,
			AnalyzerReady: true,
		}}, nil).AnyTimes()
	scaleMock.EXPECT().RemoveBrokersWithParams(gomock.Any(), gomock.All()).Return(scaleResultPointer(scale.Result{
		TaskID:    "1",
		StartedAt: "Sat, 27 Aug 2022 12:22:21 GMT",
		State:     v1beta1.CruiseControlTaskActive,
	}), nil).Times(1)
	return scaleMock
}

func getScaleMock5() *mocks.MockCruiseControlScaler {
	mockCtrl := gomock.NewController(GinkgoT())
	scaleMock := mocks.NewMockCruiseControlScaler(mockCtrl)
	scaleMock.EXPECT().IsUp(gomock.Any()).Return(true).AnyTimes()

	userTaskResult := []*scale.Result{scaleResultPointer(scale.Result{
		TaskID:    "12345",
		StartedAt: "Sat, 27 Aug 2022 12:22:21 GMT",
		State:     v1beta1.CruiseControlTaskCompletedWithError,
	})}
	userTaskResult2 := []*scale.Result{scaleResultPointer(scale.Result{
		TaskID:    "12345",
		StartedAt: "Sat, 27 Aug 2022 12:22:21 GMT",
		State:     v1beta1.CruiseControlTaskCompleted,
	})}
	first := scaleMock.EXPECT().UserTasks(gomock.Any(), gomock.Any()).Return(userTaskResult, nil).Times(1)
	scaleMock.EXPECT().UserTasks(gomock.Any(), gomock.Any()).Return(userTaskResult2, nil).After(first).AnyTimes()
	scaleMock.EXPECT().Status(gomock.Any()).Return(scale.StatusTaskResult{
		Status: &scale.CruiseControlStatus{
			ExecutorReady: true,
			MonitorReady:  true,
			AnalyzerReady: true,
		}}, nil).AnyTimes()
	scaleMock.EXPECT().AddBrokersWithParams(gomock.Any(), gomock.All()).Return(scaleResultPointer(scale.Result{
		TaskID:    "12345",
		StartedAt: "Sat, 27 Aug 2022 12:22:21 GMT",
		State:     v1beta1.CruiseControlTaskActive,
	}), nil).Times(1)
	return scaleMock
}

func getScaleMock6() *mocks.MockCruiseControlScaler {
	mockCtrl := gomock.NewController(GinkgoT())
	scaleMock := mocks.NewMockCruiseControlScaler(mockCtrl)
	scaleMock.EXPECT().IsUp(gomock.Any()).Return(true).AnyTimes()

	userTaskResult := []*scale.Result{scaleResultPointer(scale.Result{
		TaskID:    "12345",
		StartedAt: "Sat, 27 Aug 2022 12:22:21 GMT",
		State:     v1beta1.CruiseControlTaskCompletedWithError,
	})}
	scaleMock.EXPECT().UserTasks(gomock.Any(), gomock.Any()).Return(userTaskResult, nil).AnyTimes()
	scaleMock.EXPECT().Status(gomock.Any()).Return(scale.StatusTaskResult{
		Status: &scale.CruiseControlStatus{
			ExecutorReady: true,
			MonitorReady:  true,
			AnalyzerReady: true,
		}}, nil).AnyTimes()
	scaleMock.EXPECT().RebalanceWithParams(gomock.Any(), gomock.All()).Return(scaleResultPointer(scale.Result{
		TaskID:    "12346",
		StartedAt: "Sat, 27 Aug 2022 12:22:21 GMT",
		State:     v1beta1.CruiseControlTaskCompleted,
	}), nil).Times(1)

	scaleMock.EXPECT().RemoveDisksWithParams(gomock.Any(), gomock.All()).Return(scaleResultPointer(scale.Result{
		TaskID:    "12345",
		StartedAt: "Sat, 27 Aug 2022 12:22:21 GMT",
		State:     v1beta1.CruiseControlTaskCompletedWithError,
	}), nil).AnyTimes()

	return scaleMock
}

func getScaleMock7() *mocks.MockCruiseControlScaler {
	mockCtrl := gomock.NewController(GinkgoT())
	scaleMock := mocks.NewMockCruiseControlScaler(mockCtrl)
	scaleMock.EXPECT().IsUp(gomock.Any()).Return(true).AnyTimes()

	startTime := metav1.Now().Format(time.RFC1123)
	scaleMock.EXPECT().Status(gomock.Any()).Return(scale.StatusTaskResult{
		TaskResult: &scale.Result{
			TaskID:    "22222",
			StartedAt: startTime,
			State:     v1beta1.CruiseControlTaskActive,
		}}, nil).AnyTimes()
	scaleMock.EXPECT().StatusTask(gomock.Any(), "22222").Return(scale.StatusTaskResult{
		TaskResult: &scale.Result{
			TaskID:    "22222",
			StartedAt: startTime,
			State:     v1beta1.CruiseControlTaskCompleted,
		},
		Status: &scale.CruiseControlStatus{
			ExecutorReady: true,
			MonitorReady:  true,
			AnalyzerReady: true,
		}}, nil).AnyTimes()

	userTaskResult := []*scale.Result{scaleResultPointer(scale.Result{
		TaskID:    "11111",
		StartedAt: "Sat, 27 Aug 2022 12:22:21 GMT",
		State:     v1beta1.CruiseControlTaskCompleted,
	})}
	scaleMock.EXPECT().UserTasks(gomock.Any(), gomock.Any()).Return(userTaskResult, nil).AnyTimes()
	return scaleMock
}

func scaleResultPointer(res scale.Result) *scale.Result {
	return &res
}
