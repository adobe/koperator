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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	banzaiv1alpha1 "github.com/banzaicloud/koperator/api/v1alpha1"
	"github.com/banzaicloud/koperator/api/v1beta1"
	scalemocks "github.com/banzaicloud/koperator/controllers/tests/mocks"
	"github.com/banzaicloud/koperator/pkg/resources/kafka/mocks"
	"github.com/banzaicloud/koperator/pkg/scale"
)

func TestBrokersJBODSelector(t *testing.T) {
	testCases := []struct {
		testName               string
		capacityConfig         string
		brokerIDs              []string
		expectedBrokersJBOD    []string
		expectedBrokersNotJBOD []string
	}{
		{
			testName:               "All JBOD because there is no custom config",
			brokerIDs:              []string{"0", "1", "2"},
			capacityConfig:         "",
			expectedBrokersJBOD:    []string{"0", "1", "2"},
			expectedBrokersNotJBOD: []string(nil),
		},
		{
			testName:  "All JBOD",
			brokerIDs: []string{"0", "1"},
			capacityConfig: `
			{
			"brokerCapacities":[
			  {
				"brokerId": "0",
				"capacity": {
				  "DISK": {"/tmp/kafka-logs": "500000"},
				  "CPU": "100",
				  "NW_IN": "50000",
				  "NW_OUT": "50000"
				}
			  },
			  {
				"brokerId": "1",
				"capacity": {
				  "DISK": {"/tmp/kafka-logs-1": "250000", "/tmp/kafka-logs-2": "250000"},
				  "CPU": "100",
				  "NW_IN": "50000",
				  "NW_OUT": "50000"
				}
			  }
			]
		  }`,
			expectedBrokersJBOD:    []string{"0", "1"},
			expectedBrokersNotJBOD: []string(nil),
		},
		{
			testName:  "All JBOD and brokerID selection",
			brokerIDs: []string{"1"},
			capacityConfig: `
			{
			"brokerCapacities":[
			  {
				"brokerId": "0",
				"capacity": {
				  "DISK": {"/tmp/kafka-logs": "500000"},
				  "CPU": "100",
				  "NW_IN": "50000",
				  "NW_OUT": "50000"
				}
			  },
			  {
				"brokerId": "1",
				"capacity": {
				  "DISK": {"/tmp/kafka-logs-1": "250000", "/tmp/kafka-logs-2": "250000"},
				  "CPU": "100",
				  "NW_IN": "50000",
				  "NW_OUT": "50000"
				}
			  }
			]
		  }`,
			expectedBrokersJBOD:    []string{"1"},
			expectedBrokersNotJBOD: []string(nil),
		},
		{
			testName:  "All JBOD and more brokerID selection",
			brokerIDs: []string{"1", "3"},
			capacityConfig: `
			{
			"brokerCapacities":[
			  {
				"brokerId": "0",
				"capacity": {
				  "DISK": {"/tmp/kafka-logs": "500000"},
				  "CPU": "100",
				  "NW_IN": "50000",
				  "NW_OUT": "50000"
				}
			  },
			  {
				"brokerId": "1",
				"capacity": {
				  "DISK": {"/tmp/kafka-logs-1": "250000", "/tmp/kafka-logs-2": "250000"},
				  "CPU": "100",
				  "NW_IN": "50000",
				  "NW_OUT": "50000"
				}
			  }
			]
		  }`,
			expectedBrokersJBOD:    []string{"1", "3"},
			expectedBrokersNotJBOD: []string(nil),
		},
		{
			testName:  "All JBOD and more brokerID selection v2",
			brokerIDs: []string{"3", "4"},
			capacityConfig: `
			{
			"brokerCapacities":[
			  {
				"brokerId": "0",
				"capacity": {
				  "DISK": {"/tmp/kafka-logs": "500000"},
				  "CPU": "100",
				  "NW_IN": "50000",
				  "NW_OUT": "50000"
				}
			  },
			  {
				"brokerId": "1",
				"capacity": {
				  "DISK": {"/tmp/kafka-logs-1": "250000", "/tmp/kafka-logs-2": "250000"},
				  "CPU": "100",
				  "NW_IN": "50000",
				  "NW_OUT": "50000"
				}
			  }
			]
		  }`,
			expectedBrokersJBOD:    []string{"4", "3"},
			expectedBrokersNotJBOD: []string(nil),
		},
		{
			testName:  "JBOD and not JBOD",
			brokerIDs: []string{"0", "1", "2"},
			capacityConfig: `
			{
			"brokerCapacities":[
			  {
				"brokerId": "0",
				"capacity": {
				  "DISK": {"/tmp/kafka-logs": "500000"},
				  "CPU": "100",
				  "NW_IN": "50000",
				  "NW_OUT": "50000"
				}
			  },
			  {
				"brokerId": "1",
				"capacity": {
				  "DISK": {"/tmp/kafka-logs-1": "250000", "/tmp/kafka-logs-2": "250000"},
				  "CPU": "100",
				  "NW_IN": "50000",
				  "NW_OUT": "50000"
				}
			  },
			  {
				"brokerId": "2",
				"capacity": {
				  "DISK": "5000",
				  "CPU": "100",
				  "NW_IN": "50000",
				  "NW_OUT": "50000"
				}
			  }
			]
		  }`,
			expectedBrokersJBOD:    []string{"0", "1"},
			expectedBrokersNotJBOD: []string{"2"},
		},
		{
			testName:  "All JBOD with -1 default",
			brokerIDs: []string{"0", "1", "2"},
			capacityConfig: `
			{
			"brokerCapacities":[
			  {
				"brokerId": "-1",
				"capacity": {
				  "DISK": {"/tmp/kafka-logs": "500000"},
				  "CPU": "100",
				  "NW_IN": "50000",
				  "NW_OUT": "50000"
				}
			  }
			]
		  }`,
			expectedBrokersJBOD:    []string{"0", "1", "2"},
			expectedBrokersNotJBOD: []string(nil),
		},
		{
			testName:  "All not JBOD with -1 default",
			brokerIDs: []string{"0", "1", "2"},
			capacityConfig: `
			{
			"brokerCapacities":[
			  {
				"brokerId": "-1",
				"capacity": {
				  "DISK": "54343",
				  "CPU": "100",
				  "NW_IN": "50000",
				  "NW_OUT": "50000"
				}
			  }
			]
		  }`,
			expectedBrokersJBOD:    []string(nil),
			expectedBrokersNotJBOD: []string{"0", "1", "2"},
		},
		{
			testName:  "not JBOD with -1 default but there is one JBOD overwrite default ",
			brokerIDs: []string{"0", "1", "2"},
			capacityConfig: `
			{
			"brokerCapacities":[
			  {
				"brokerId": "-1",
				"capacity": {
				  "DISK": "54343",
				  "CPU": "100",
				  "NW_IN": "50000",
				  "NW_OUT": "50000"
				}
			  },
			  {
				"brokerId": "1",
				"capacity": {
				  "DISK": {"/tmp/kafka-logs": "500000"},
				  "CPU": "100",
				  "NW_IN": "50000",
				  "NW_OUT": "50000"
				}
			  }
			]
		  }`,
			expectedBrokersJBOD:    []string{"1"},
			expectedBrokersNotJBOD: []string{"0", "2"},
		},
		{
			testName:  "JBOD with -1 default but there is one not JBOD overwrite default ",
			brokerIDs: []string{"0", "1", "2"},
			capacityConfig: `
			{
			"brokerCapacities":[
			  {
				"brokerId": "-1",
				"capacity": {
				  "DISK": {"/tmp/kafka-logs": "500000"},
				  "CPU": "100",
				  "NW_IN": "50000",
				  "NW_OUT": "50000"
				}
			  },
			  {
				"brokerId": "1",
				"capacity": {
				  "DISK": "5000",
				  "CPU": "100",
				  "NW_IN": "50000",
				  "NW_OUT": "50000"
				}
			  }
			]
		  }`,
			expectedBrokersJBOD:    []string{"0", "2"},
			expectedBrokersNotJBOD: []string{"1"},
		},
	}
	for _, testCase := range testCases {
		brokersJBOD, brokersNotJBOD, err := brokersJBODSelector(testCase.brokerIDs, testCase.capacityConfig)
		assert.NoError(t, err)
		assert.ElementsMatch(t, testCase.expectedBrokersJBOD, brokersJBOD, "testName", testCase.testName)
		assert.ElementsMatch(t, testCase.expectedBrokersNotJBOD, brokersNotJBOD, "testName", testCase.testName)
	}
}

func TestCreateCCOperation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ttlSecondsAfterFinished := 60

	testCases := []struct {
		operationType      banzaiv1alpha1.CruiseControlTaskOperation
		brokerIDs          []string
		isJBOD             bool
		brokerIdsToLogDirs map[string][]string
		parameterCheck     func(t *testing.T, params map[string]string)
	}{
		{
			operationType:      banzaiv1alpha1.OperationAddBroker,
			brokerIDs:          []string{"1", "2", "3"},
			isJBOD:             false,
			brokerIdsToLogDirs: nil,
			parameterCheck: func(t *testing.T, params map[string]string) {
				assert.Equal(t, "1,2,3", params[scale.ParamBrokerID])
				assert.Equal(t, "true", params[scale.ParamExcludeDemoted])
				assert.Equal(t, "true", params[scale.ParamExcludeRemoved])
			},
		},
		{
			operationType:      banzaiv1alpha1.OperationRemoveBroker,
			brokerIDs:          []string{"1"},
			isJBOD:             false,
			brokerIdsToLogDirs: nil,
			parameterCheck: func(t *testing.T, params map[string]string) {
				assert.Equal(t, "1", params[scale.ParamBrokerID])
				assert.Equal(t, "true", params[scale.ParamExcludeDemoted])
				assert.Equal(t, "true", params[scale.ParamExcludeRemoved])
			},
		},
		{
			operationType:      banzaiv1alpha1.OperationRemoveBroker,
			brokerIDs:          []string{"1", "2", "3"},
			isJBOD:             false,
			brokerIdsToLogDirs: nil,
			parameterCheck: func(t *testing.T, params map[string]string) {
				assert.Equal(t, "1,2,3", params[scale.ParamBrokerID])
				assert.Equal(t, "true", params[scale.ParamExcludeDemoted])
				assert.Equal(t, "true", params[scale.ParamExcludeRemoved])
			},
		},
		{
			operationType: banzaiv1alpha1.OperationRemoveDisks,
			brokerIDs:     []string{"1", "2"},
			isJBOD:        false,
			brokerIdsToLogDirs: map[string][]string{
				"1": {"logdir1", "logdir2"},
				"2": {"logdir1"},
			},
			parameterCheck: func(t *testing.T, params map[string]string) {
				// can be in any order
				expectedString1 := "1-logdir1,1-logdir2,2-logdir1"
				expectedString2 := "2-logdir1,1-logdir1,1-logdir2"
				assert.Contains(t, []string{expectedString1, expectedString2}, params[scale.ParamBrokerIDAndLogDirs])
			},
		},
		{
			operationType:      banzaiv1alpha1.OperationRebalance,
			brokerIDs:          []string{"1", "2", "3"},
			isJBOD:             true,
			brokerIdsToLogDirs: nil,
			parameterCheck: func(t *testing.T, params map[string]string) {
				assert.Equal(t, "1,2,3", params[scale.ParamDestbrokerIDs])
				assert.Equal(t, "true", params[scale.ParamRebalanceDisk])
				assert.Equal(t, "true", params[scale.ParamExcludeDemoted])
				assert.Equal(t, "true", params[scale.ParamExcludeRemoved])
			},
		},
	}

	mockCtrl := gomock.NewController(t)
	for _, testCase := range testCases {
		mockClient := mocks.NewMockClient(mockCtrl)
		mockSubResourceClient := mocks.NewMockSubResourceClient(mockCtrl)
		scheme := runtime.NewScheme()
		_ = v1beta1.AddToScheme(scheme)
		_ = corev1.AddToScheme(scheme)

		r := CruiseControlTaskReconciler{
			Client: mockClient,
			Scheme: scheme,
		}

		kafkaCluster := &v1beta1.KafkaCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "kafka",
				Namespace: "kafka",
			}}

		// Mock the Create call and capture the operation
		var createdOperation *banzaiv1alpha1.CruiseControlOperation
		mockClient.EXPECT().Create(ctx, gomock.AssignableToTypeOf(&banzaiv1alpha1.CruiseControlOperation{})).Do(func(ctx context.Context, obj client.Object, opts ...client.CreateOption) {
			createdOperation = obj.(*banzaiv1alpha1.CruiseControlOperation)
			createdOperation.ObjectMeta.Name = "generated-name"
		}).Return(nil)

		// Mock the Status call
		mockClient.EXPECT().Status().Return(mockSubResourceClient)

		// Mock the Update call
		mockSubResourceClient.EXPECT().Update(ctx, gomock.AssignableToTypeOf(&banzaiv1alpha1.CruiseControlOperation{})).Do(func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) {
			arg := obj.(*banzaiv1alpha1.CruiseControlOperation)
			createdOperation.Status = arg.Status
		}).Return(nil)

		_, err := r.createCCOperation(ctx, kafkaCluster, banzaiv1alpha1.ErrorPolicyRetry, &ttlSecondsAfterFinished, testCase.operationType, testCase.brokerIDs, testCase.isJBOD, testCase.brokerIdsToLogDirs)
		assert.NoError(t, err)

		// Use the captured operation for further assertions
		assert.Equal(t, testCase.operationType, createdOperation.Status.CurrentTask.Operation)
		testCase.parameterCheck(t, createdOperation.Status.CurrentTask.Parameters)
	}
}

func TestReconcileDiskRemovalMatchesExactLogDir(t *testing.T) {
	const brokerID = "1"
	testCases := []struct {
		name       string
		mountPath  string
		onlineDirs []string
		wantTarget string
	}{
		{
			name:       "sibling path reported first",
			mountPath:  "/kafka-logs1",
			onlineDirs: []string{"/kafka-logs10/kafka", "/kafka-logs1/kafka"},
			wantTarget: "1-/kafka-logs1/kafka",
		},
		{
			name:       "Kafka normalizes a trailing separator",
			mountPath:  "/kafka-logs1/",
			onlineDirs: []string{"/kafka-logs10/kafka", "/kafka-logs1/kafka"},
			wantTarget: "1-/kafka-logs1/kafka",
		},
		{
			name:       "requested path not online on this broker",
			mountPath:  "/kafka-logs1",
			onlineDirs: []string{"/kafka-logs10/kafka"},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			scheme := runtime.NewScheme()
			require.NoError(t, v1beta1.AddToScheme(scheme))
			require.NoError(t, banzaiv1alpha1.AddToScheme(scheme))
			cluster := &v1beta1.KafkaCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "kafka", Namespace: "kafka"},
				Status: v1beta1.KafkaClusterStatus{BrokersState: map[string]v1beta1.BrokerState{
					brokerID: {GracefulActionState: v1beta1.GracefulActionState{VolumeStates: map[string]v1beta1.VolumeState{
						tc.mountPath: {CruiseControlVolumeState: v1beta1.GracefulDiskRemovalRequired},
					}}},
				}},
			}
			kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster).
				WithStatusSubresource(cluster, &banzaiv1alpha1.CruiseControlOperation{}).Build()
			scaler := scalemocks.NewMockCruiseControlScaler(gomock.NewController(t))
			scaler.EXPECT().LogDirsByBroker(ctx).Return(map[string]map[scale.LogDirState][]string{
				brokerID: {scale.LogDirStateOnline: tc.onlineDirs},
				"2":      {scale.LogDirStateOnline: {"/kafka-logs1/kafka"}},
			}, nil)
			r := &CruiseControlTaskReconciler{
				Client: kube, DirectClient: kube, Scheme: scheme, ScaleFactory: scalemocks.NewMockScaleFactory(scaler),
			}

			key := client.ObjectKeyFromObject(cluster)
			_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: key})
			operations := &banzaiv1alpha1.CruiseControlOperationList{}
			require.NoError(t, kube.List(ctx, operations, client.InNamespace(cluster.Namespace)))
			require.NoError(t, kube.Get(ctx, key, cluster))
			state := cluster.Status.BrokersState[brokerID].GracefulActionState.VolumeStates[tc.mountPath]
			if tc.wantTarget == "" {
				assert.Error(t, err)
				assert.Empty(t, operations.Items)
				assert.Equal(t, v1beta1.GracefulDiskRemovalRequired, state.CruiseControlVolumeState)
				assert.Nil(t, state.CruiseControlOperationReference)
				return
			}
			require.NoError(t, err)
			require.Len(t, operations.Items, 1)
			operation := &operations.Items[0]
			assert.Equal(t, banzaiv1alpha1.OperationRemoveDisks, operation.CurrentTaskOperation())
			assert.Equal(t, tc.wantTarget, operation.CurrentTaskParameters()[scale.ParamBrokerIDAndLogDirs])
			assert.Equal(t, v1beta1.GracefulDiskRemovalScheduled, state.CruiseControlVolumeState)
			require.NotNil(t, state.CruiseControlOperationReference)
			assert.Equal(t, operation.Name, state.CruiseControlOperationReference.Name)
		})
	}
}
