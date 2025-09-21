// Copyright © 2020 Cisco Systems, Inc. and/or its affiliates
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
	"time"

	"github.com/banzaicloud/koperator/pkg/util"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	banzaicloudv1beta1 "github.com/banzaicloud/koperator/api/v1beta1"
	"github.com/banzaicloud/koperator/pkg/kafkaclient"
)

const defaultBrokerConfigGroup = "default"

func createMinimalKafkaClusterCR(name, namespace string) *banzaicloudv1beta1.KafkaCluster {
	return &banzaicloudv1beta1.KafkaCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Annotations: map[string]string{},
		},
		Spec: banzaicloudv1beta1.KafkaClusterSpec{
			KRaftMode: false,
			ListenersConfig: banzaicloudv1beta1.ListenersConfig{
				ExternalListeners: []banzaicloudv1beta1.ExternalListenerConfig{
					{
						CommonListenerSpec: banzaicloudv1beta1.CommonListenerSpec{
							Name:          "test",
							ContainerPort: 9094,
							Type:          "plaintext",
						},
						ExternalStartingPort: 19090,
						IngressServiceSettings: banzaicloudv1beta1.IngressServiceSettings{
							HostnameOverride: "test-host",
						},
						AccessMethod: corev1.ServiceTypeLoadBalancer,
					},
				},
				InternalListeners: []banzaicloudv1beta1.InternalListenerConfig{
					{
						CommonListenerSpec: banzaicloudv1beta1.CommonListenerSpec{
							Type:                            "plaintext",
							Name:                            "internal",
							ContainerPort:                   29092,
							UsedForInnerBrokerCommunication: true,
						},
					},
					{
						CommonListenerSpec: banzaicloudv1beta1.CommonListenerSpec{
							Type:                            "plaintext",
							Name:                            "controller",
							ContainerPort:                   29093,
							UsedForInnerBrokerCommunication: false,
						},
						UsedForControllerCommunication: true,
					},
				},
			},
			BrokerConfigGroups: map[string]banzaicloudv1beta1.BrokerConfig{
				defaultBrokerConfigGroup: {
					StorageConfigs: []banzaicloudv1beta1.StorageConfig{
						{
							MountPath: "/kafka-logs",
							PvcSpec: &corev1.PersistentVolumeClaimSpec{
								AccessModes: []corev1.PersistentVolumeAccessMode{
									corev1.ReadWriteOnce,
								},
								Resources: corev1.VolumeResourceRequirements{
									Requests: map[corev1.ResourceName]resource.Quantity{
										corev1.ResourceStorage: resource.MustParse("10Gi"),
									},
								},
							},
							// emptyDir should be ignored as pvcSpec has prio
							EmptyDir: &corev1.EmptyDirVolumeSource{
								SizeLimit: util.QuantityPointer(resource.MustParse("20Mi")),
							},
						},
						{
							MountPath: "/ephemeral-dir1",
							EmptyDir: &corev1.EmptyDirVolumeSource{
								SizeLimit: util.QuantityPointer(resource.MustParse("100Mi")),
							},
						},
					},
				},
			},
			Brokers: []banzaicloudv1beta1.Broker{
				{
					Id:                0,
					BrokerConfigGroup: defaultBrokerConfigGroup,
				},
				{
					Id:                1,
					BrokerConfigGroup: defaultBrokerConfigGroup,
				},
				{
					Id:                2,
					BrokerConfigGroup: defaultBrokerConfigGroup,
				},
			},
			ClusterImage: "ghcr.io/adobe/kafka:2.13-3.9.1",
			ZKAddresses:  []string{},
			MonitoringConfig: banzaicloudv1beta1.MonitoringConfig{
				CCJMXExporterConfig: "custom_property: custom_value",
			},
			ReadOnlyConfig:       "cruise.control.metrics.topic.auto.create=true",
			RollingUpgradeConfig: banzaicloudv1beta1.RollingUpgradeConfig{FailureThreshold: 1, ConcurrentBrokerRestartCountPerRack: 1},
		},
	}
}

func waitForClusterRunningState(ctx context.Context, kafkaCluster *banzaicloudv1beta1.KafkaCluster, namespace string) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	ch := make(chan struct{}, 1)

	treshold := 10
	consecutiveRunningState := 0

	go func() {
		for {
			time.Sleep(50 * time.Millisecond)
			select {
			case <-ctx.Done():
				return
			default:
				createdKafkaCluster := &banzaicloudv1beta1.KafkaCluster{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: kafkaCluster.Name, Namespace: namespace}, createdKafkaCluster)
				if err != nil || createdKafkaCluster.Status.State != banzaicloudv1beta1.KafkaClusterRunning {
					consecutiveRunningState = 0
					continue
				}
				consecutiveRunningState++
				if consecutiveRunningState > treshold {
					ch <- struct{}{}
					return
				}
			}
		}
	}()
	Eventually(ch, 240*time.Second, 50*time.Millisecond).Should(Receive())
}

func getMockedKafkaClientForCluster(kafkaCluster *banzaicloudv1beta1.KafkaCluster) (kafkaclient.KafkaClient, func()) {
	name := types.NamespacedName{
		Name:      kafkaCluster.Name,
		Namespace: kafkaCluster.Namespace,
	}
	if val, ok := mockKafkaClients[name]; ok {
		return val, func() { _ = val.Close() }
	}
	mockKafkaClient, _, _ := kafkaclient.NewMockFromCluster(k8sClient, kafkaCluster)
	mockKafkaClients[name] = mockKafkaClient
	return mockKafkaClient, func() { _ = mockKafkaClient.Close() }
}

func resetMockKafkaClient(kafkaCluster *banzaicloudv1beta1.KafkaCluster) {
	// delete all topics
	mockKafkaClient, _ := getMockedKafkaClientForCluster(kafkaCluster)
	topics, _ := mockKafkaClient.ListTopics()
	for topicName := range topics {
		_ = mockKafkaClient.DeleteTopic(topicName, false)
	}

	// delete all acls
	_ = mockKafkaClient.DeleteUserACLs("", "")
}
