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

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"emperror.dev/errors"
	"github.com/gruntwork-io/terratest/modules/k8s"
	ginkgo "github.com/onsi/ginkgo/v2"
	gomega "github.com/onsi/gomega"
)

const (
	tsResizeClusterName = "kafka-ts-resize"

	tsResizeInitialManifest = "../../config/samples/kafkacluster_tiered_storage_cache_resize_initial.yaml"
	tsResizeShrunkManifest  = "../../config/samples/kafkacluster_tiered_storage_cache_resize_shrunk.yaml"

	tsResizeCacheMountPath = "/tiered-storage-cache"
	tsResizeInitialSize    = "2Gi"
	tsResizeShrunkSize     = "1Gi"

	// Annotation keys written by the cache-resize reconciler.
	pvcResizeStateAnnotation  = "koperator.adobe.com/cache-resize-state"
	pvcResizeStatePending     = "pending-deletion"
	pvcResizeStateReplacement = "replacement"

	tsResizePhaseTimeout    = 10 * time.Minute
	tsResizePollingInterval = 15 * time.Second

	tsResizeBrokerID = 0
)

// pvcItem is a minimal representation of a PVC for assertion helpers.
type pvcItem struct {
	Name        string
	Annotations map[string]string
	StorageSize string
}

// listBrokerCachePVCs returns PVCs for broker tsResizeBrokerID that have the
// tiered-storage-cache mount path annotation.
func listBrokerCachePVCs(kubectlOptions k8s.KubectlOptions) ([]pvcItem, error) {
	selector := fmt.Sprintf("%s=%s,brokerId=%d", kafkaCRLabelKey, tsResizeClusterName, tsResizeBrokerID)

	rawOutput, err := k8s.RunKubectlAndGetOutputE(ginkgo.GinkgoT(), &kubectlOptions,
		"get", "persistentvolumeclaims",
		"-l", selector,
		"--output", "json",
	)
	if err != nil {
		return nil, errors.WrapIf(err, "listing PVCs failed")
	}

	var pvcList struct {
		Items []struct {
			Metadata struct {
				Name        string            `json:"name"`
				Annotations map[string]string `json:"annotations"`
			} `json:"metadata"`
			Spec struct {
				Resources struct {
					Requests struct {
						Storage string `json:"storage"`
					} `json:"requests"`
				} `json:"resources"`
			} `json:"spec"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(rawOutput), &pvcList); err != nil {
		return nil, errors.WrapIf(err, "parsing PVC list JSON failed")
	}

	var result []pvcItem
	for _, item := range pvcList.Items {
		if item.Metadata.Annotations["mountPath"] == tsResizeCacheMountPath {
			result = append(result, pvcItem{
				Name:        item.Metadata.Name,
				Annotations: item.Metadata.Annotations,
				StorageSize: item.Spec.Resources.Requests.Storage,
			})
		}
	}
	return result, nil
}

// getBrokerPodUID returns the UID of the running (non-terminating) broker pod for the
// given broker ID, or an error if no such pod is found.
func getBrokerPodUID(kubectlOptions k8s.KubectlOptions, clusterName string, brokerID int) (string, error) {
	rawOutput, err := k8s.RunKubectlAndGetOutputE(ginkgo.GinkgoT(), &kubectlOptions,
		"get", "pod",
		"-l", fmt.Sprintf("%s=%s,brokerId=%d,app=kafka", kafkaCRLabelKey, clusterName, brokerID),
		"--output", "json",
	)
	if err != nil {
		return "", errors.WrapIf(err, "listing broker pods failed")
	}

	var podList struct {
		Items []struct {
			Metadata struct {
				UID               string  `json:"uid"`
				DeletionTimestamp *string `json:"deletionTimestamp"`
			} `json:"metadata"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(rawOutput), &podList); err != nil {
		return "", errors.WrapIf(err, "parsing pod list JSON failed")
	}

	for _, item := range podList.Items {
		if item.Metadata.DeletionTimestamp == nil {
			return item.Metadata.UID, nil
		}
	}
	return "", fmt.Errorf("no running broker pod found for broker %d in cluster %s", brokerID, clusterName)
}

// testTieredStorageCachePvcResize tests the full multi-phase delete-and-recreate
// flow for a tiered storage cache PVC shrink. It:
//
//  1. Installs a minimal KRaft cluster with a 2Gi cache PVC on broker 0.
//  2. Applies an updated manifest that shrinks the cache to 1Gi.
//  3. Waits for Phase 1 (staging): both the old PVC (pending-deletion) and
//     the new PVC (replacement) exist simultaneously.
//  4. Waits for Phase 2 (pod cycle): the pod restarts, the old PVC is deleted,
//     and the new pod starts referencing the replacement PVC.
//  5. Waits for Phase 3 (completion): the replacement annotation is stripped and
//     the broker pod is running again.
//  6. Verifies the surviving PVC carries the new 1Gi size.
//  7. Cleans up the cluster.
func testTieredStorageCachePvcResize() bool {
	return ginkgo.When("Testing tiered storage cache PVC shrink (delete-and-recreate flow)", ginkgo.Ordered, func() {
		var kubectlOptions k8s.KubectlOptions
		var err error
		var broker0PodUID string // UID of the broker-0 pod before the resize, used to detect recycling

		ginkgo.It("Acquiring K8s config and context", func() {
			kubectlOptions, err = kubectlOptionsForCurrentContext()
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			kubectlOptions.Namespace = koperatorLocalHelmDescriptor.Namespace
		})

		// Pre-cleanup: forcibly remove any leftover cluster from a previous interrupted run
		// so the install step always starts from a clean slate.
		// We cannot rely on requireDeleteKafkaCluster here because a stuck finalizer (left by a
		// mid-reconcile interruption) prevents the cascade deletion from completing in time.
		// Instead we strip the finalizer first, delete the CR, then explicitly delete every
		// owned resource so nothing blocks the subsequent fresh install.
		ginkgo.It("Pre-cleanup: removing any leftover kafka-ts-resize cluster", func() {
			// 1. Remove finalizers so the CR can be deleted regardless of operator state.
			_, _ = k8s.RunKubectlAndGetOutputE(ginkgo.GinkgoT(), &kubectlOptions,
				"patch", kafkaKind, tsResizeClusterName,
				"--type=merge", `--patch={"metadata":{"finalizers":[]}}`,
			)
			// 2. Delete the CR itself (ignore not-found).
			_, _ = k8s.RunKubectlAndGetOutputE(ginkgo.GinkgoT(), &kubectlOptions,
				"delete", kafkaKind, tsResizeClusterName, "--ignore-not-found",
			)
			// 3. Explicitly delete PVCs — they carry a pvc-protection finalizer that
			//    blocks cascade GC while pods are still bound.
			_, _ = k8s.RunKubectlAndGetOutputE(ginkgo.GinkgoT(), &kubectlOptions,
				"delete", "persistentvolumeclaims",
				"-l", fmt.Sprintf("%s=%s", kafkaCRLabelKey, tsResizeClusterName),
				"--ignore-not-found",
			)
			// 4. Delete pods so they release PVC mounts promptly.
			_, _ = k8s.RunKubectlAndGetOutputE(ginkgo.GinkgoT(), &kubectlOptions,
				"delete", "pods",
				"-l", fmt.Sprintf("%s=%s", kafkaCRLabelKey, tsResizeClusterName),
				"--ignore-not-found", "--grace-period=0",
			)
			// 5. Wait for the CR itself to be gone (GC will handle the rest).
			gomega.Eventually(context.Background(), func() error {
				out, err := k8s.RunKubectlAndGetOutputE(ginkgo.GinkgoT(), &kubectlOptions,
					"get", kafkaKind, tsResizeClusterName,
				)
				if err != nil || strings.Contains(out, "NotFound") || strings.Contains(out, "not found") {
					return nil
				}
				return errors.New("KafkaCluster CR still exists")
			}, tsResizePhaseTimeout, tsResizePollingInterval).ShouldNot(gomega.HaveOccurred())
		})

		ginkgo.It("Installing KafkaCluster with tiered storage cache PVC (2Gi)", func() {
			ginkgo.By("Applying initial KafkaCluster manifest")
			applyK8sResourceManifest(kubectlOptions, tsResizeInitialManifest)

			ginkgo.By("Waiting for all broker pods to be ready")
			// kubectl wait --for=condition=Ready fails immediately when no pods exist yet.
			// We don't wait for ClusterRunning — the cluster may stay in ClusterReconciling
			// because GracefulDiskRebalanceRequired for log volumes needs CC, which this minimal
			// test cluster does not deploy. Pod readiness is sufficient to proceed with the resize.
			gomega.Eventually(context.Background(), func() error {
				output, err := k8s.RunKubectlAndGetOutputE(ginkgo.GinkgoT(), &kubectlOptions,
					"get", "pod",
					"-l", fmt.Sprintf("%s=%s,app=kafka,isBrokerNode=true", kafkaCRLabelKey, tsResizeClusterName),
					"--output", "json",
				)
				if err != nil {
					return errors.WrapIf(err, "listing pods failed")
				}
				var podList struct {
					Items []struct {
						Status struct {
							Conditions []struct {
								Type   string `json:"type"`
								Status string `json:"status"`
							} `json:"conditions"`
						} `json:"status"`
					} `json:"items"`
				}
				if err := json.Unmarshal([]byte(output), &podList); err != nil {
					return errors.WrapIf(err, "parsing pod list JSON failed")
				}
				if len(podList.Items) < 1 {
					return fmt.Errorf("expected at least 1 broker pod, got %d", len(podList.Items))
				}
				for _, pod := range podList.Items {
					ready := false
					for _, cond := range pod.Status.Conditions {
						if cond.Type == "Ready" && cond.Status == "True" {
							ready = true
							break
						}
					}
					if !ready {
						return errors.New("not all pods are ready yet")
					}
				}
				return nil
			}, kafkaClusterCreateTimeout, tsResizePollingInterval).ShouldNot(gomega.HaveOccurred())

			ginkgo.By("Capturing broker-0 pod UID before resize")
			broker0PodUID, err = getBrokerPodUID(kubectlOptions, tsResizeClusterName, 0)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(broker0PodUID).NotTo(gomega.BeEmpty())

			ginkgo.By("Verifying initial cache PVC size is " + tsResizeInitialSize)
			pvcs, err := listBrokerCachePVCs(kubectlOptions)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pvcs).To(gomega.HaveLen(1), "expected exactly one cache PVC for broker 0")
			gomega.Expect(pvcs[0].StorageSize).To(gomega.Equal(tsResizeInitialSize))
		})

		ginkgo.It("Triggering cache PVC shrink from 2Gi to 1Gi", func() {
			ginkgo.By("Applying shrunk KafkaCluster manifest")
			applyK8sResourceManifest(kubectlOptions, tsResizeShrunkManifest)
		})

		ginkgo.It("Phase 1: old PVC annotated pending-deletion and replacement PVC created", func() {
			ginkgo.By("Waiting until both pending-deletion and replacement PVCs coexist for broker 0")
			gomega.Eventually(context.Background(), func() error {
				pvcs, err := listBrokerCachePVCs(kubectlOptions)
				if err != nil {
					return err
				}
				var hasPendingDeletion, hasReplacement bool
				for _, pvc := range pvcs {
					switch pvc.Annotations[pvcResizeStateAnnotation] {
					case pvcResizeStatePending:
						hasPendingDeletion = true
					case pvcResizeStateReplacement:
						hasReplacement = true
					}
				}
				if !hasPendingDeletion {
					return errors.New("no PVC with pending-deletion annotation yet")
				}
				if !hasReplacement {
					return errors.New("no PVC with replacement annotation yet")
				}
				return nil
			}, tsResizePhaseTimeout, tsResizePollingInterval).ShouldNot(gomega.HaveOccurred())
		})

		ginkgo.It("Phase 2: broker pod restarts, pending-deletion PVC is deleted", func() {
			ginkgo.By("Waiting for broker-0 pod to be recycled (UID change indicates rolling restart)")
			// We detect recycling by UID change rather than waiting for the pod count to hit zero.
			// The pod may restart fast enough to be back before the next polling tick, causing a
			// "pod count == 0" check to time out even when the restart already completed.
			gomega.Eventually(context.Background(), func() error {
				uid, err := getBrokerPodUID(kubectlOptions, tsResizeClusterName, 0)
				if err != nil {
					// Pod may be absent mid-restart; treat as not-yet-recycled.
					return err
				}
				if uid == broker0PodUID {
					return errors.New("broker-0 pod has not been recycled yet (same UID)")
				}
				return nil
			}, tsResizePhaseTimeout, tsResizePollingInterval).ShouldNot(gomega.HaveOccurred())

			ginkgo.By("Waiting for the pending-deletion PVC to be deleted")
			gomega.Eventually(context.Background(), func() error {
				pvcs, err := listBrokerCachePVCs(kubectlOptions)
				if err != nil {
					return err
				}
				for _, pvc := range pvcs {
					if pvc.Annotations[pvcResizeStateAnnotation] == pvcResizeStatePending {
						return fmt.Errorf("pending-deletion PVC %s still exists", pvc.Name)
					}
				}
				return nil
			}, tsResizePhaseTimeout, tsResizePollingInterval).ShouldNot(gomega.HaveOccurred())
		})

		ginkgo.It("Phase 3: broker pod running again with new PVC, replacement annotation stripped", func() {
			ginkgo.By("Waiting for the broker-0 pod to come back Ready")
			err = waitK8sResourceCondition(kubectlOptions, "pod", "condition=Ready",
				tsResizePhaseTimeout,
				fmt.Sprintf("%s=%s,brokerId=0,app=kafka", kafkaCRLabelKey, tsResizeClusterName), "")
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Waiting for replacement annotation to be stripped from the surviving PVC")
			gomega.Eventually(context.Background(), func() error {
				pvcs, err := listBrokerCachePVCs(kubectlOptions)
				if err != nil {
					return err
				}
				if len(pvcs) != 1 {
					return fmt.Errorf("expected 1 cache PVC for broker 0, got %d", len(pvcs))
				}
				if state := pvcs[0].Annotations[pvcResizeStateAnnotation]; state != "" {
					return fmt.Errorf("cache-resize-state annotation still present: %q", state)
				}
				return nil
			}, tsResizePhaseTimeout, tsResizePollingInterval).ShouldNot(gomega.HaveOccurred())
		})

		ginkgo.It("Verifying the surviving cache PVC has the new size "+tsResizeShrunkSize, func() {
			pvcs, err := listBrokerCachePVCs(kubectlOptions)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pvcs).To(gomega.HaveLen(1))
			gomega.Expect(pvcs[0].StorageSize).To(gomega.Equal(tsResizeShrunkSize),
				"surviving PVC should carry the shrunk size")
			gomega.Expect(strings.Contains(pvcs[0].Name, tsResizeClusterName)).To(gomega.BeTrue(),
				"surviving PVC should belong to the test cluster")
		})

		// requireDeleteKafkaCluster receives a copy of kubectlOptions at registration
		// time, so Namespace must be set here (not inside the It block above).
		kubectlOptions.Namespace = koperatorLocalHelmDescriptor.Namespace
		// requireDeleteKafkaCluster registers its own It block — must be called at
		// the When scope, not nested inside an It.
		requireDeleteKafkaCluster(kubectlOptions, tsResizeClusterName)
	})
}
