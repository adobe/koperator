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
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/banzaicloud/koperator/api/v1alpha1"
	"github.com/banzaicloud/koperator/api/v1beta1"
	mocks "github.com/banzaicloud/koperator/controllers/tests/mocks"
	"github.com/banzaicloud/koperator/pkg/scale"
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
