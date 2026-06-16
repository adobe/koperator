// Copyright © 2023 Cisco Systems, Inc. and/or its affiliates
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

package kafka

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"go.uber.org/mock/gomock"

	apiutil "github.com/banzaicloud/koperator/api/util"
	"github.com/banzaicloud/koperator/api/v1beta1"
	"github.com/banzaicloud/koperator/pkg/resources"
	mocks "github.com/banzaicloud/koperator/pkg/resources/kafka/mocks"
	"github.com/banzaicloud/koperator/pkg/util"
)

func TestService(t *testing.T) {
	testCases := []struct {
		testName        string
		r               *Reconciler
		expectedService *corev1.Service
	}{
		{
			testName: "Basic Internal And External Service",
			r: &Reconciler{
				Reconciler: resources.Reconciler{
					KafkaCluster: &v1beta1.KafkaCluster{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "kafka",
							Namespace: "kafka",
						},
						Spec: v1beta1.KafkaClusterSpec{
							LocalDebugEnabled: false,
							KRaftMode:         false,
							ListenersConfig: v1beta1.ListenersConfig{
								InternalListeners: []v1beta1.InternalListenerConfig{
									{
										CommonListenerSpec: v1beta1.CommonListenerSpec{
											Name:                            "internal",
											ContainerPort:                   29092,
											Type:                            "plaintext",
											UsedForInnerBrokerCommunication: true,
										},
									},
								},
								ExternalListeners: []v1beta1.ExternalListenerConfig{
									{
										CommonListenerSpec: v1beta1.CommonListenerSpec{
											Name:                            "plaintext",
											ContainerPort:                   29094,
											Type:                            "plaintext",
											UsedForInnerBrokerCommunication: false,
										},
										AccessMethod: corev1.ServiceTypeLoadBalancer,
									},
								},
							},
						},
					},
				},
			},
			expectedService: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "kafka-1",
					Namespace:   "kafka",
					Labels:      map[string]string{"app": "kafka", "brokerId": "1", "kafka_cr": "kafka"},
					Annotations: map[string]string{},
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         "",
							Kind:               "",
							Name:               "kafka",
							UID:                "",
							Controller:         util.BoolPointer(true),
							BlockOwnerDeletion: util.BoolPointer(true),
						},
					},
				},
				Spec: corev1.ServiceSpec{
					Type:            corev1.ServiceTypeClusterIP,
					SessionAffinity: corev1.ServiceAffinityNone,
					Selector:        apiutil.MergeLabels(apiutil.LabelsForKafka("kafka"), map[string]string{v1beta1.BrokerIdLabelKey: "1"}),
					Ports: []corev1.ServicePort{
						{
							Name:       "tcp-internal",
							Protocol:   "TCP",
							Port:       29092,
							TargetPort: intstr.FromInt(29092),
							NodePort:   0,
						},
						{
							Name:       "tcp-plaintext",
							Protocol:   "TCP",
							Port:       29094,
							TargetPort: intstr.FromInt(29094),
							NodePort:   0,
						},
						{
							Name:       "metrics",
							Protocol:   "TCP",
							Port:       9020,
							TargetPort: intstr.FromInt(9020),
							NodePort:   0,
						},
					},
					ClusterIP:                "",
					PublishNotReadyAddresses: false,
				},
			},
		},
		{
			testName: "Basic Internal And External Service",
			r: &Reconciler{
				Reconciler: resources.Reconciler{
					KafkaCluster: &v1beta1.KafkaCluster{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "kafka",
							Namespace: "kafka",
						},
						Spec: v1beta1.KafkaClusterSpec{
							LocalDebugEnabled: true,
							KRaftMode:         false,
							ListenersConfig: v1beta1.ListenersConfig{
								InternalListeners: []v1beta1.InternalListenerConfig{
									{
										CommonListenerSpec: v1beta1.CommonListenerSpec{
											Name:                            "internal",
											ContainerPort:                   29092,
											Type:                            "plaintext",
											UsedForInnerBrokerCommunication: true,
										},
									},
								},
								ExternalListeners: []v1beta1.ExternalListenerConfig{
									{
										CommonListenerSpec: v1beta1.CommonListenerSpec{
											Name:                            "plaintext",
											ContainerPort:                   29094,
											Type:                            "plaintext",
											UsedForInnerBrokerCommunication: false,
										},
										AccessMethod: corev1.ServiceTypeLoadBalancer,
									},
								},
							},
						},
					},
				},
			},
			expectedService: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "kafka-1",
					Namespace:   "kafka",
					Labels:      map[string]string{"app": "kafka", "brokerId": "1", "kafka_cr": "kafka"},
					Annotations: map[string]string{},
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         "",
							Kind:               "",
							Name:               "kafka",
							UID:                "",
							Controller:         util.BoolPointer(true),
							BlockOwnerDeletion: util.BoolPointer(true),
						},
					},
				},
				Spec: corev1.ServiceSpec{
					Type:            corev1.ServiceTypeLoadBalancer,
					SessionAffinity: corev1.ServiceAffinityNone,
					Selector:        apiutil.MergeLabels(apiutil.LabelsForKafka("kafka"), map[string]string{v1beta1.BrokerIdLabelKey: "1"}),
					Ports: []corev1.ServicePort{
						{
							Name:       "tcp-internal",
							Protocol:   "TCP",
							Port:       29092,
							TargetPort: intstr.FromInt(29092),
							NodePort:   0,
						},
						{
							Name:       "tcp-plaintext",
							Protocol:   "TCP",
							Port:       29094,
							TargetPort: intstr.FromInt(29094),
							NodePort:   0,
						},
						{
							Name:       "metrics",
							Protocol:   "TCP",
							Port:       9020,
							TargetPort: intstr.FromInt(9020),
							NodePort:   0,
						},
					},
					ClusterIP:                "",
					PublishNotReadyAddresses: false,
				},
			},
		},
	}
	mockCtrl := gomock.NewController(t)

	for _, test := range testCases {
		t.Run(test.testName, func(t *testing.T) {
			mockClient := mocks.NewMockClient(mockCtrl)
			mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
			r := test.r

			actualService := r.service(1, nil)

			require.Equal(t, test.expectedService, actualService)
		})
	}
}
