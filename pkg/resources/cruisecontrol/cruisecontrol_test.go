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

package cruisecontrol

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/banzaicloud/koperator/api/v1beta1"
)

func TestGetPreviousCapacityConfig(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal("could not register core scheme:", err)
	}
	if err := v1beta1.AddToScheme(scheme); err != nil {
		t.Fatal("could not register KafkaCluster scheme:", err)
	}

	oldConfig := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kafka-cruisecontrol-config",
			Namespace: "default",
		},
		Data: map[string]string{"capacity.json": `{"brokerCapacities":[]}`},
	}

	testCases := []struct {
		name          string
		brokerState   map[string]v1beta1.BrokerState
		includeConfig bool
		expectConfig  bool
	}{
		{
			name: "loads config during disk operation",
			brokerState: map[string]v1beta1.BrokerState{
				"0": {
					GracefulActionState: v1beta1.GracefulActionState{
						VolumeStates: map[string]v1beta1.VolumeState{
							"/kafka-logs3": {
								CruiseControlVolumeState: v1beta1.GracefulDiskRemovalRunning,
							},
						},
					},
				},
			},
			includeConfig: true,
			expectConfig:  true,
		},
		{
			name: "loads config during broker deletion",
			brokerState: map[string]v1beta1.BrokerState{
				"1": {
					GracefulActionState: v1beta1.GracefulActionState{
						CruiseControlState: v1beta1.GracefulDownscaleRunning,
					},
				},
			},
			includeConfig: true,
			expectConfig:  true,
		},
		{
			name:          "skips lookup without an ongoing operation",
			brokerState:   map[string]v1beta1.BrokerState{"0": {}},
			includeConfig: true,
			expectConfig:  false,
		},
		{
			name: "returns nil when config is not found",
			brokerState: map[string]v1beta1.BrokerState{
				"0": {
					GracefulActionState: v1beta1.GracefulActionState{
						VolumeStates: map[string]v1beta1.VolumeState{
							"/kafka-logs3": {
								CruiseControlVolumeState: v1beta1.GracefulDiskRebalanceRunning,
							},
						},
					},
				},
			},
			expectConfig: false,
		},
	}

	for _, test := range testCases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			builder := fake.NewClientBuilder().WithScheme(scheme)
			if test.includeConfig {
				builder = builder.WithObjects(oldConfig.DeepCopy())
			}
			cluster := &v1beta1.KafkaCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "kafka", Namespace: "default"},
				Status:     v1beta1.KafkaClusterStatus{BrokersState: test.brokerState},
			}
			reconciler := New(builder.Build(), cluster, nil)

			config, err := reconciler.getPreviousCapacityConfig(context.Background())
			if err != nil {
				t.Fatal("unexpected error:", err)
			}
			if test.expectConfig && config == nil {
				t.Fatal("expected previous capacity config to be loaded")
			}
			if !test.expectConfig && config != nil {
				t.Fatalf("expected no previous capacity config, got %s/%s", config.Namespace, config.Name)
			}
		})
	}
}
