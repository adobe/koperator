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

/*
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package tests

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"

	"github.com/onsi/ginkgo/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/banzaicloud/koperator/api/v1beta1"
)

var (
	nodePorts     = make(map[int32]bool)
	nodePortMutex sync.Mutex
)

func SafeKafkaCleanup(ctx context.Context, kClient client.Client, cluster *v1beta1.KafkaCluster, kraftCluster *v1beta1.KafkaCluster, ns string) {
	fmt.Println("Starting safe Kafka cleanup")

	if cluster != nil {
		clusterName := cluster.Name
		clusterNamespace := cluster.Namespace

		fmt.Printf("Safely deleting Kafka cluster %s/%s\n", clusterNamespace, clusterName)

		err := kClient.DeleteAllOf(ctx, &v1beta1.KafkaCluster{},
			client.InNamespace(clusterNamespace),
			client.MatchingLabels{v1beta1.KafkaCRLabelKey: clusterName})

		if err != nil {
			fmt.Printf("Warning: Error deleting KafkaCluster %s/%s: %v\n",
				clusterNamespace, clusterName, err)
		}
	}

	if kraftCluster != nil {
		clusterName := kraftCluster.Name
		clusterNamespace := kraftCluster.Namespace

		fmt.Printf("Safely deleting KRaft cluster %s/%s\n", clusterNamespace, clusterName)

		err := kClient.DeleteAllOf(ctx, &v1beta1.KafkaCluster{},
			client.InNamespace(clusterNamespace),
			client.MatchingLabels{v1beta1.KafkaCRLabelKey: clusterName})

		if err != nil {
			fmt.Printf("Warning: Error deleting KRaft KafkaCluster %s/%s: %v\n",
				clusterNamespace, clusterName, err)
		}
	}

	if ns != "" {
		fmt.Printf("Cleaning up services in namespace %s\n", ns)

		svcList := &corev1.ServiceList{}
		if err := kClient.List(ctx, svcList, client.InNamespace(ns)); err == nil {
			for i := range svcList.Items {
				svc := &svcList.Items[i]
				fmt.Printf("Deleting service %s/%s\n", ns, svc.Name)
				if len(svc.Finalizers) > 0 {
					patchedSvc := svc.DeepCopy()
					patchedSvc.Finalizers = nil
					err = kClient.Patch(ctx, patchedSvc, client.MergeFrom(svc)) //TODO Handle Error
				}
				err = kClient.Delete(ctx, svc, &client.DeleteOptions{ //TODO Handle error
					GracePeriodSeconds: ptr.To[int64](0),
				})
			}
		} else {
			fmt.Printf("Warning: Error listing services in namespace %s: %v\n", ns, err)
		}
	}

	fmt.Println("Finished Kafka cleanup")
}

func GetNodePort() int32 {
	nodePortMutex.Lock()
	defer nodePortMutex.Unlock()

	for port := int32(30000); port < 32767; port++ {
		if !nodePorts[port] {
			nodePorts[port] = true
			fmt.Printf("Allocated NodePort %d\n", port)
			return port
		}
	}

	fmt.Println("Warning: No free NodePorts found, returning 0 for auto-assignment")
	return 0
}

func ReleaseNodePort(port int32) {
	nodePortMutex.Lock()
	defer nodePortMutex.Unlock()

	delete(nodePorts, port)
	fmt.Printf("Released NodePort %d\n", port)
}

var _ = ginkgo.AfterEach(func() {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("Recovered from panic in global AfterEach: %v\n", r)
			debug.PrintStack()
		}
	}()

	fmt.Printf("Running global cleanup for test\n")

})

func SafeJustAfterEach(cleanupFunc func(context.Context)) func() {
	return func() {
		// Create a background context
		ctx := context.Background()

		// Recover from panics
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("Recovered from panic in JustAfterEach: %v\n", r)
				debug.PrintStack()
			}
		}()

		cleanupFunc(ctx)
	}
}
