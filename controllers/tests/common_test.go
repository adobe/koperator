// Copyright © 2020 Cisco Systems, Inc. and/or its affiliates
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

	"github.com/banzaicloud/koperator/api/v1beta1"
	"github.com/banzaicloud/koperator/pkg/kafkaclient"
)

const defaultBrokerConfigGroup = "default"

func createMinimalKafkaClusterCR(name, namespace string) *v1beta1.KafkaCluster {
	return &v1beta1.KafkaCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Annotations: map[string]string{},
		},
		Spec: v1beta1.KafkaClusterSpec{
			KRaftMode: false,
			ListenersConfig: v1beta1.ListenersConfig{
				ExternalListeners: []v1beta1.ExternalListenerConfig{
					{
						CommonListenerSpec: v1beta1.CommonListenerSpec{
							Name:          "test",
							ContainerPort: 9094,
							Type:          "plaintext",
						},
						ExternalStartingPort: 19090,
						IngressServiceSettings: v1beta1.IngressServiceSettings{
							HostnameOverride: "test-host",
						},
						AccessMethod: corev1.ServiceTypeLoadBalancer,
					},
				},
				InternalListeners: []v1beta1.InternalListenerConfig{
					{
						CommonListenerSpec: v1beta1.CommonListenerSpec{
							Type:                            "plaintext",
							Name:                            "internal",
							ContainerPort:                   29092,
							UsedForInnerBrokerCommunication: true,
						},
					},
					{
						CommonListenerSpec: v1beta1.CommonListenerSpec{
							Type:                            "plaintext",
							Name:                            "controller",
							ContainerPort:                   29093,
							UsedForInnerBrokerCommunication: false,
						},
						UsedForControllerCommunication: true,
					},
				},
			},
			BrokerConfigGroups: map[string]v1beta1.BrokerConfig{
				defaultBrokerConfigGroup: {
					StorageConfigs: []v1beta1.StorageConfig{
						{
							MountPath: "/kafka-logs",
							PvcSpec: &corev1.PersistentVolumeClaimSpec{
								AccessModes: []corev1.PersistentVolumeAccessMode{
									corev1.ReadWriteOnce,
								},
								Resources: corev1.ResourceRequirements{
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
			Brokers: []v1beta1.Broker{
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
			ClusterImage: "ghcr.io/banzaicloud/kafka:2.13-3.4.1",
			ZKAddresses:  []string{},
			MonitoringConfig: v1beta1.MonitoringConfig{
				CCJMXExporterConfig: "custom_property: custom_value",
			},
			ReadOnlyConfig:       "cruise.control.metrics.topic.auto.create=true",
			RollingUpgradeConfig: v1beta1.RollingUpgradeConfig{FailureThreshold: 1, ConcurrentBrokerRestartCountPerRack: 1},
		},
	}
}

func waitForClusterRunningState(ctx context.Context, kafkaCluster *v1beta1.KafkaCluster, namespace string) {
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
				createdKafkaCluster := &v1beta1.KafkaCluster{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: kafkaCluster.Name, Namespace: namespace}, createdKafkaCluster)
				if err != nil || createdKafkaCluster.Status.State != v1beta1.KafkaClusterRunning {
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
	Eventually(ch, 300*time.Second, 50*time.Millisecond).Should(Receive())
}

func getMockedKafkaClientForCluster(kafkaCluster *v1beta1.KafkaCluster) (kafkaclient.KafkaClient, func()) {
	name := types.NamespacedName{
		Name:      kafkaCluster.Name,
		Namespace: kafkaCluster.Namespace,
	}
	if val, ok := mockKafkaClients[name]; ok {
		return val, func() { val.Close() }
	}
	mockKafkaClient, _, _ := kafkaclient.NewMockFromCluster(k8sClient, kafkaCluster)
	mockKafkaClients[name] = mockKafkaClient
	return mockKafkaClient, func() { mockKafkaClient.Close() }
}

func resetMockKafkaClient(kafkaCluster *v1beta1.KafkaCluster) {
	// delete all topics
	mockKafkaClient, _ := getMockedKafkaClientForCluster(kafkaCluster)
	topics, _ := mockKafkaClient.ListTopics()
	for topicName := range topics {
		_ = mockKafkaClient.DeleteTopic(topicName, false)
	}

	// delete all acls
	_ = mockKafkaClient.DeleteUserACLs("", "")
}

// func cleanupNamespaceResources(k8sClient client.Client, namespace string, timeout time.Duration) {
// 	fmt.Printf("Starting cleanup of Service IPs for namespace: %s\n", namespace)

// 	// Helper function to delete and wait for resources to be gone
// 	deleteAndWait := func(name string, deleteFunc func() (int, error)) {
// 		fmt.Printf("Checking for %s in namespace %s...\n", name, namespace)

// 		// First delete pass
// 		count, err := deleteFunc()
// 		if err != nil {
// 			fmt.Printf("Error listing %s: %v\n", name, err)
// 		} else if count == 0 {
// 			fmt.Printf("No %s found in namespace %s\n", name, namespace)
// 			return
// 		} else {
// 			fmt.Printf("Deleting %d %s from namespace %s\n", count, name, namespace)
// 		}

// 		// Wait for resources to be deleted
// 		startTime := time.Now()
// 		gomega.Eventually(func() int {
// 			remainingCount, err := deleteFunc()
// 			if err != nil {
// 				fmt.Printf("Error checking remaining %s: %v\n", name, err)
// 				return 0
// 			}

// 			if remainingCount > 0 {
// 				elapsed := time.Since(startTime)
// 				fmt.Printf("Still waiting for %d %s to be deleted after %v...\n",
// 					remainingCount, name, elapsed.Round(time.Second))
// 			}

// 			return remainingCount
// 		}, timeout, 2*time.Second).Should(gomega.Equal(0),
// 			fmt.Sprintf("Failed to delete all %s within %v", name, timeout))

// 		fmt.Printf("All %s successfully deleted from namespace %s\n", name, namespace)
// 	}

// 	// Force delete options with 0 grace period for faster cleanup
// 	deleteOpts := &client.DeleteOptions{
// 		GracePeriodSeconds: ptr.To[int64](0),
// 	}

// 	// Delete Services (critical for freeing Service IPs)
// 	deleteAndWait("Services", func() (int, error) {
// 		list := &corev1.ServiceList{}
// 		err := k8sClient.List(context.Background(), list, client.InNamespace(namespace))
// 		if err != nil && !errors.IsNotFound(err) {
// 			return 0, err
// 		}

// 		for i := range list.Items {
// 			svc := &list.Items[i]
// 			fmt.Printf("  Deleting Service: %s (type: %s, clusterIP: %s)\n",
// 				svc.Name, svc.Spec.Type, svc.Spec.ClusterIP)

// 			err := k8sClient.Delete(context.Background(), svc, deleteOpts)
// 			if err != nil && !errors.IsNotFound(err) {
// 				fmt.Printf("  Error deleting Service %s: %v\n", svc.Name, err)
// 			}
// 		}
// 		return len(list.Items), nil
// 	})

// 	// Delete Pods
// 	deleteAndWait("Pods", func() (int, error) {
// 		list := &corev1.PodList{}
// 		err := k8sClient.List(context.Background(), list, client.InNamespace(namespace))
// 		if err != nil && !errors.IsNotFound(err) {
// 			return 0, err
// 		}

// 		for i := range list.Items {
// 			pod := &list.Items[i]
// 			fmt.Printf("  Deleting Pod: %s (phase: %s)\n",
// 				pod.Name, pod.Status.Phase)

// 			err := k8sClient.Delete(context.Background(), pod, deleteOpts)
// 			if err != nil && !errors.IsNotFound(err) {
// 				fmt.Printf("  Error deleting Pod %s: %v\n", pod.Name, err)
// 			}
// 		}
// 		return len(list.Items), nil
// 	})

// 	fmt.Printf("Cleanup of Service IPs completed for namespace: %s\n", namespace)
// }
