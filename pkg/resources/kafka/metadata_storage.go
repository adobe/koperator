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

	"emperror.dev/errors"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apiutil "github.com/banzaicloud/koperator/api/util"
	"github.com/banzaicloud/koperator/api/v1beta1"
	"github.com/banzaicloud/koperator/pkg/errorfactory"
	"github.com/banzaicloud/koperator/pkg/k8sutil"
	kafkautils "github.com/banzaicloud/koperator/pkg/util/kafka"
	properties "github.com/banzaicloud/koperator/properties/pkg"
)

const (
	storagePurposeLabel      = "kafka.banzaicloud.io/storage-purpose"
	metadataPurpose          = "metadata"
	metadataConfigAnnotation = "kafka.banzaicloud.io/metadata-storage-config"
	metadataFreshAnnotation  = "kafka.banzaicloud.io/metadata-fresh-storage"
	metadataReadyAnnotation  = "kafka.banzaicloud.io/metadata-storage-ready"
	metadataVolumeName       = v1beta1.MetadataStorageVolumeName
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
				// Admission rejects this using MetadataStorageState; this guards bypassed admission.
				return fmt.Errorf("broker %d: keep all existing data disks until metadata migration has completed and the replacement pod is ready; "+
					"restore data storageConfig %s to resume reconciliation", broker.Id, pvc.Annotations[mountPathAnnotationKey])
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
	if err := k8sutil.Reconcile(log, r.Client, desired, r.KafkaCluster); err != nil {
		return err
	}
	// The PVC annotation is the durable record; the status projects it for admission.
	if migrationReady && r.KafkaCluster.Status.BrokersState[fmt.Sprint(broker.Id)].MetadataStorageState != v1beta1.MetadataStorageReady {
		return k8sutil.UpdateBrokerStatus(r.Client, []string{fmt.Sprint(broker.Id)}, r.KafkaCluster, v1beta1.MetadataStorageReady, log)
	}
	return nil
}

// deleteMetadataPVC deletes the dedicated metadata PVC of a removed broker, so the
// broker ID can later be reused with or without metadata storage.
func (r *Reconciler) deleteMetadataPVC(ctx context.Context, brokerID string, log logr.Logger) error {
	pvcs := &corev1.PersistentVolumeClaimList{}
	if err := r.List(ctx, pvcs, client.InNamespace(r.KafkaCluster.Namespace), client.MatchingLabels(apiutil.MergeLabels(
		apiutil.LabelsForKafka(r.KafkaCluster.Name), map[string]string{v1beta1.BrokerIdLabelKey: brokerID, storagePurposeLabel: metadataPurpose},
	))); err != nil {
		return err
	}
	for i := range pvcs.Items {
		if !pvcs.Items[i].DeletionTimestamp.IsZero() {
			continue
		}
		if err := client.IgnoreNotFound(r.Delete(ctx, &pvcs.Items[i])); err != nil {
			return err
		}
		log.V(1).Info("metadata pvc for broker deleted", "pvc name", pvcs.Items[i].Name, v1beta1.BrokerIdLabelKey, brokerID)
	}
	return nil
}

func podMountsMetadataVolume(pod *corev1.Pod) bool {
	for _, volume := range pod.Spec.Volumes {
		if volume.Name == metadataVolumeName && volume.PersistentVolumeClaim != nil {
			return true
		}
	}
	return false
}

func configMapHasMetadataLogDir(configMap *corev1.ConfigMap) bool {
	if configMap == nil {
		return false
	}
	props, err := properties.NewFromString(configMap.Data[kafkautils.ConfigPropertyName])
	if err != nil {
		return false
	}
	value, found := props.Get(kafkautils.KafkaConfigMetadataLogDirectory)
	return found && value.Value() != ""
}

// metadataLogDirConfigurable reports whether metadata.log.dir may be written into the
// broker ConfigMap. The ConfigMap is live-mounted, so a still-running pod without the
// metadata volume would otherwise format the unmounted directory on a container restart.
// Once published it is kept, because metadataStorage cannot be disabled.
func (r *Reconciler) metadataLogDirConfigurable(ctx context.Context, brokerID int32, current *corev1.ConfigMap, log logr.Logger) bool {
	if configMapHasMetadataLogDir(current) {
		return true
	}
	pods := &corev1.PodList{}
	if err := r.List(ctx, pods, client.InNamespace(r.KafkaCluster.Namespace), client.MatchingLabels(apiutil.MergeLabels(
		apiutil.LabelsForKafka(r.KafkaCluster.Name), map[string]string{v1beta1.BrokerIdLabelKey: fmt.Sprint(brokerID)},
	))); err != nil {
		log.Error(err, "deferring metadata.log.dir: could not list broker pods", v1beta1.BrokerIdLabelKey, brokerID)
		return false
	}
	for i := range pods.Items {
		if !podMountsMetadataVolume(&pods.Items[i]) {
			log.Info("deferring metadata.log.dir until the broker pod without metadata storage is replaced",
				v1beta1.BrokerIdLabelKey, brokerID, "pod", pods.Items[i].Name)
			return false
		}
	}
	return true
}

// ensureMetadataLogDirConfigured fails closed before creating a metadata-storage pod whose
// ConfigMap does not yet point Kafka at the metadata volume. A missing ConfigMap (rack
// awareness creates it after the pod) is generated later with the setting.
func (r *Reconciler) ensureMetadataLogDirConfigured(ctx context.Context, brokerID string, bConfig *v1beta1.BrokerConfig) error {
	if bConfig == nil || bConfig.MetadataStorage == nil {
		return nil
	}
	configMap := &corev1.ConfigMap{}
	err := r.Get(ctx, client.ObjectKey{Namespace: r.KafkaCluster.Namespace, Name: fmt.Sprintf(brokerConfigTemplate+"-%s", r.KafkaCluster.Name, brokerID)}, configMap)
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !configMapHasMetadataLogDir(configMap) {
		return errorfactory.New(errorfactory.ResourceNotReady{}, errors.New("metadata.log.dir not yet configured"),
			"broker configmap does not set metadata.log.dir; deferring pod creation", v1beta1.BrokerIdLabelKey, brokerID)
	}
	return nil
}

func isMetadataMigrationRestart(currentPod, desiredPod *corev1.Pod) bool {
	return podMountsMetadataVolume(desiredPod) && !podMountsMetadataVolume(currentPod)
}

// metadataMigrationGate lets metadataStorage be enabled for many brokers at once:
// it starts a migration only when no other broker's migration is in progress and
// no other broker has offline or out-of-sync replicas.
func (r *Reconciler) metadataMigrationGate(ctx context.Context, currentPod *corev1.Pod) error {
	waiting := func(reason string, keysAndValues ...interface{}) error {
		return errorfactory.New(errorfactory.ReconcileRollingUpgrade{}, errors.New(reason),
			"waiting before migrating the next broker's metadata", keysAndValues...)
	}
	currentID := currentPod.Labels[v1beta1.BrokerIdLabelKey]
	if r.metadataMigrationStarted {
		return waiting("a metadata migration was started in this reconcile", v1beta1.BrokerIdLabelKey, currentID)
	}
	// Read through the API server: a just-deleted pod may not be in the cache yet.
	var reader client.Reader = r.Client
	if r.DirectClient != nil {
		reader = r.DirectClient
	}
	pods := &corev1.PodList{}
	if err := reader.List(ctx, pods, client.InNamespace(r.KafkaCluster.Namespace),
		client.MatchingLabels(apiutil.LabelsForKafka(r.KafkaCluster.Name))); err != nil {
		return errors.WrapIf(err, "failed to list broker pods for metadata migration gate")
	}
	podsByBroker := make(map[string][]corev1.Pod)
	for _, pod := range pods.Items {
		id := pod.Labels[v1beta1.BrokerIdLabelKey]
		podsByBroker[id] = append(podsByBroker[id], pod)
	}
	for _, broker := range r.KafkaCluster.Spec.Brokers {
		id := fmt.Sprint(broker.Id)
		if id == currentID {
			continue
		}
		config, err := broker.GetBrokerConfig(r.KafkaCluster.Spec)
		if err != nil {
			return err
		}
		if config.MetadataStorage == nil || r.KafkaCluster.Status.BrokersState[id].MetadataStorageState == v1beta1.MetadataStorageReady {
			continue
		}
		// Not Ready and the old pod is gone, going, or already replaced: migration in progress.
		inProgress := len(podsByBroker[id]) == 0
		for i := range podsByBroker[id] {
			if podMountsMetadataVolume(&podsByBroker[id][i]) || !podsByBroker[id][i].DeletionTimestamp.IsZero() {
				inProgress = true
			}
		}
		if inProgress {
			return waiting("metadata migration of another broker is not Ready yet", v1beta1.BrokerIdLabelKey, currentID, "migratingBrokerId", id)
		}
	}
	kClient, closeClient, err := r.kafkaClientProvider.NewFromCluster(r.Client, r.KafkaCluster)
	if err != nil {
		return errorfactory.New(errorfactory.BrokersUnreachable{}, err, "could not connect to kafka brokers")
	}
	defer closeClient()
	offline, err := kClient.AllOfflineReplicas()
	if err != nil {
		return errors.WrapIf(err, "metadata migration health check failed")
	}
	outOfSync, err := kClient.OutOfSyncReplicas()
	if err != nil {
		return errors.WrapIf(err, "metadata migration health check failed")
	}
	// The broker being migrated may itself be unhealthy (e.g. a crashed container).
	for _, brokerID := range append(offline, outOfSync...) {
		if fmt.Sprint(brokerID) != currentID {
			return waiting("cluster has offline or out-of-sync replicas", v1beta1.BrokerIdLabelKey, currentID, "impactedBrokerId", brokerID)
		}
	}
	return nil
}
