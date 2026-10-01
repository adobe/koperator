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

package kafka

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apiutil "github.com/banzaicloud/koperator/api/util"
	"github.com/banzaicloud/koperator/api/v1beta1"
	"github.com/banzaicloud/koperator/pkg/k8sutil"
)

const (
	storagePurposeLabel      = "kafka.banzaicloud.io/storage-purpose"
	metadataPurpose          = "metadata"
	metadataConfigAnnotation = "kafka.banzaicloud.io/metadata-storage-config"
	metadataFreshAnnotation  = "kafka.banzaicloud.io/metadata-fresh-storage"
	metadataReadyAnnotation  = "kafka.banzaicloud.io/metadata-storage-ready"
	metadataVolumeName       = "kraft-metadata"
)

func metadataMigrationRecovered(pod *corev1.Pod, pvcName string) bool {
	if !pod.DeletionTimestamp.IsZero() || pod.Status.Phase != corev1.PodRunning || !isPodReady(pod) {
		return false
	}
	mounted := false
	for _, volume := range pod.Spec.Volumes {
		if volume.Name == metadataVolumeName && volume.PersistentVolumeClaim != nil &&
			volume.PersistentVolumeClaim.ClaimName == pvcName {
			mounted = true
			break
		}
	}
	if !mounted {
		return false
	}
	for _, status := range pod.Status.InitContainerStatuses {
		if status.Name == "migrate-broker-metadata" {
			return status.State.Terminated != nil && status.State.Terminated.ExitCode == 0
		}
	}
	return false
}

func isMetadataPVC(pvc corev1.PersistentVolumeClaim) bool {
	return pvc.Labels[storagePurposeLabel] == metadataPurpose
}

func dataPVCs(pvcs []corev1.PersistentVolumeClaim) []corev1.PersistentVolumeClaim {
	result := make([]corev1.PersistentVolumeClaim, 0, len(pvcs))
	for _, pvc := range pvcs {
		if !isMetadataPVC(pvc) {
			result = append(result, pvc)
		}
	}
	return result
}

func (r *Reconciler) metadataPVC(ctx context.Context, brokerID int32) (*corev1.PersistentVolumeClaim, error) {
	pvcs := &corev1.PersistentVolumeClaimList{}
	if err := r.List(ctx, pvcs, client.InNamespace(r.KafkaCluster.Namespace), client.MatchingLabels(apiutil.MergeLabels(
		apiutil.LabelsForKafka(r.KafkaCluster.Name), map[string]string{v1beta1.BrokerIdLabelKey: fmt.Sprint(brokerID)},
	))); err != nil {
		return nil, err
	}
	var result *corev1.PersistentVolumeClaim
	for _, pvc := range pvcs.Items {
		if !isMetadataPVC(pvc) {
			continue
		}
		if result != nil || !pvc.DeletionTimestamp.IsZero() {
			return nil, fmt.Errorf("broker %d metadata PVC is ambiguous or terminating", brokerID)
		}
		result = pvc.DeepCopy()
	}
	return result, nil
}

// Metadata PVCs have their own lifecycle. They must never enter disk-draining
// reconciliation, capacity generation or Cruise Control volume states.
func (r *Reconciler) reconcileMetadataStorage(ctx context.Context, broker v1beta1.Broker, config *v1beta1.BrokerConfig, log logr.Logger) error {
	if config == nil {
		return fmt.Errorf("broker %d has no effective configuration", broker.Id)
	}
	if err := config.ValidateMetadataStorage(r.KafkaCluster.Spec.KRaftMode); err != nil {
		return err
	}
	current, err := r.metadataPVC(ctx, broker.Id)
	if err != nil {
		return err
	}
	if config.MetadataStorage == nil {
		if current != nil {
			return fmt.Errorf("broker %d cannot remove enabled metadataStorage", broker.Id)
		}
		return nil
	}
	if !shouldUseKRaftModeForBroker(getBrokerReadOnlyConfig(broker, r.KafkaCluster, log)) {
		return fmt.Errorf("metadataStorage cannot be used by a ZooKeeper migration broker")
	}
	pvcs := &corev1.PersistentVolumeClaimList{}
	if err := r.List(ctx, pvcs, client.InNamespace(r.KafkaCluster.Namespace), client.MatchingLabels(apiutil.MergeLabels(
		apiutil.LabelsForKafka(r.KafkaCluster.Name), map[string]string{v1beta1.BrokerIdLabelKey: fmt.Sprint(broker.Id)},
	))); err != nil {
		return err
	}
	pods := &corev1.PodList{}
	if err := r.List(ctx, pods, client.InNamespace(r.KafkaCluster.Namespace), client.MatchingLabels(apiutil.MergeLabels(
		apiutil.LabelsForKafka(r.KafkaCluster.Name), map[string]string{v1beta1.BrokerIdLabelKey: fmt.Sprint(broker.Id)},
	))); err != nil {
		return err
	}
	migrationReady := current != nil && current.Annotations[metadataReadyAnnotation] == configValueTrue
	if current != nil && !migrationReady {
		for i := range pods.Items {
			if metadataMigrationRecovered(&pods.Items[i], current.Name) {
				migrationReady = true
				break
			}
		}
	}
	for _, pvc := range dataPVCs(pvcs.Items) {
		if v1beta1.StoragePathsOverlap(config.MetadataStorage.MountPath, pvc.Annotations[mountPathAnnotationKey]) {
			return fmt.Errorf("metadataStorage overlaps existing data PVC %s", pvc.Name)
		}
		if !migrationReady {
			found := false
			for _, storage := range config.StorageConfigs {
				if storage.MountPath == pvc.Annotations[mountPathAnnotationKey] {
					found = true
				}
			}
			if !found {
				return fmt.Errorf("keep all existing data disks until metadata migration has completed and the replacement pod is ready")
			}
		}
	}
	desired, err := r.pvc(broker.Id, 0, *config.MetadataStorage, config, true)
	if err != nil {
		return err
	}
	desired.Name = fmt.Sprintf("%s-%d-metadata", r.KafkaCluster.Name, broker.Id)
	desired.GenerateName = ""
	desired.Labels[storagePurposeLabel] = metadataPurpose
	location := config.MetadataStorage.DeepCopy()
	location.PvcSpec.Resources = corev1.VolumeResourceRequirements{}
	locationJSON, err := json.Marshal(location)
	if err != nil {
		return err
	}
	desired.Annotations[metadataConfigAnnotation] = string(locationJSON)
	if current == nil {
		desired.Annotations[metadataFreshAnnotation] = fmt.Sprint(len(dataPVCs(pvcs.Items)) == 0 && len(pods.Items) == 0 &&
			r.KafkaCluster.Status.BrokersState[fmt.Sprint(broker.Id)].Version == "")
	}
	if current != nil {
		if current.Annotations[metadataConfigAnnotation] != string(locationJSON) ||
			current.Annotations[mountPathAnnotationKey] != config.MetadataStorage.MountPath {
			return fmt.Errorf("broker %d cannot relocate or replace enabled metadataStorage", broker.Id)
		}
		desired = current.DeepCopy()
		desired.Spec.Resources = config.MetadataStorage.PvcSpec.Resources
		if isDesiredStorageValueInvalid(desired, current) {
			return fmt.Errorf("cannot reduce metadata PVC size")
		}
		if migrationReady {
			desired.Annotations[metadataReadyAnnotation] = configValueTrue
		}
	}
	return k8sutil.Reconcile(log, r.Client, desired, r.KafkaCluster)
}
