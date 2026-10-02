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
	"reflect"
	"testing"

	"emperror.dev/errors"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	apiutil "github.com/banzaicloud/koperator/api/util"
	"github.com/banzaicloud/koperator/api/v1beta1"
	"github.com/banzaicloud/koperator/pkg/errorfactory"
	"github.com/banzaicloud/koperator/pkg/kafkaclient"
	"github.com/banzaicloud/koperator/pkg/resources"
	"github.com/banzaicloud/koperator/pkg/resources/kafka/mocks"
	kafkautils "github.com/banzaicloud/koperator/pkg/util/kafka"
	properties "github.com/banzaicloud/koperator/properties/pkg"
)

func metadataTestReconciler(t *testing.T) (*Reconciler, v1beta1.Broker) {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, v1beta1.AddToScheme(scheme))
	config := &v1beta1.BrokerConfig{
		Roles: []string{"broker"},
		StorageConfigs: []v1beta1.StorageConfig{{MountPath: "/csi-kafka-logs1", PvcSpec: &corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources:   corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("100Gi")}},
		}}},
		MetadataStorage: &v1beta1.StorageConfig{MountPath: "/csi-kafka-metadata", PvcSpec: &corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources:   corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("10Gi")}},
		}},
	}
	broker := v1beta1.Broker{Id: 1, BrokerConfig: config}
	cluster := &v1beta1.KafkaCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "test", UID: "cluster-uid"},
		Spec:       v1beta1.KafkaClusterSpec{KRaftMode: true, Brokers: []v1beta1.Broker{broker}},
		Status:     v1beta1.KafkaClusterStatus{ClusterID: "cluster-one"},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster).WithStatusSubresource(cluster).Build()
	return &Reconciler{Reconciler: resources.Reconciler{Client: c, KafkaCluster: cluster}}, broker
}

func TestMetadataStorageIndependentLifecycle(t *testing.T) {
	r, broker := metadataTestReconciler(t)
	ctx, log := context.Background(), logr.Discard()
	require.NoError(t, r.reconcileMetadataStorage(ctx, broker, broker.BrokerConfig, log))
	pvc, err := r.metadataPVC(ctx, broker.Id)
	require.NoError(t, err)
	require.NotNil(t, pvc)
	require.Equal(t, "test-1-metadata", pvc.Name)
	require.Equal(t, "true", pvc.Annotations[metadataFreshAnnotation])
	require.NoError(t, r.reconcileMetadataStorage(ctx, broker, broker.BrokerConfig, log))
	all := &corev1.PersistentVolumeClaimList{}
	require.NoError(t, r.List(ctx, all))
	require.Len(t, all.Items, 1)

	data, err := r.pvc(1, 0, broker.BrokerConfig.StorageConfigs[0], broker.BrokerConfig, true)
	require.NoError(t, err)
	data.Name, data.GenerateName = "data", ""
	data.Status.Phase = corev1.ClaimBound
	require.NoError(t, r.Create(ctx, data))
	dataPvcs, err := getCreatedPvcForBroker(ctx, r.Client, nil, 1, broker.BrokerConfig.StorageConfigs, "test", "test")
	require.NoError(t, err)
	require.Len(t, dataPvcs, 1)
	require.Equal(t, "data", dataPvcs[0].Name)
	desired, err := r.pvc(1, 0, broker.BrokerConfig.StorageConfigs[0], broker.BrokerConfig, true)
	require.NoError(t, err)
	require.NoError(t, r.reconcileKafkaPvc(ctx, log, map[string][]*corev1.PersistentVolumeClaim{"1": {desired}}, map[string]struct{}{"1": {}}))
	for _, state := range r.KafkaCluster.Status.BrokersState {
		require.NotContains(t, state.GracefulActionState.VolumeStates, broker.BrokerConfig.MetadataStorage.MountPath)
	}
	require.NoError(t, r.Get(ctx, client.ObjectKeyFromObject(pvc), &corev1.PersistentVolumeClaim{}))

	pod := r.pod(1, broker.BrokerConfig, append(dataPvcs, *pvc), log).(*corev1.Pod)
	require.Len(t, pod.Spec.InitContainers, 3)
	init := pod.Spec.InitContainers[2]
	require.Equal(t, "migrate-broker-metadata", init.Name)
	require.Equal(t, pod.Spec.Containers[0].Image, init.Image)
	require.Equal(t, pod.Spec.Containers[0].Resources, init.Resources)
	for _, env := range init.Env {
		if env.Name == "DATA_MOUNTS" {
			require.Equal(t, "/csi-kafka-logs1", env.Value)
		}
	}
	for _, env := range pod.Spec.Containers[0].Env {
		if env.Name == "LOG_DIRS" {
			require.Equal(t, "/csi-kafka-logs1", env.Value)
		}
	}
	require.Contains(t, pod.Spec.Containers[0].VolumeMounts, corev1.VolumeMount{Name: metadataVolumeName, MountPath: "/csi-kafka-metadata"})
	config := r.generateBrokerConfig(broker, broker.BrokerConfig, nil, nil, nil, nil, nil, "", nil, log)
	props, err := properties.NewFromString(config)
	require.NoError(t, err)
	logDirs, _ := props.Get("log.dirs")
	metadataDir, _ := props.Get("metadata.log.dir")
	require.Equal(t, "/csi-kafka-logs1/kafka", logDirs.Value())
	require.Equal(t, "/csi-kafka-metadata/kafka", metadataDir.Value())

	for _, change := range []func(*v1beta1.BrokerConfig){
		func(b *v1beta1.BrokerConfig) { b.MetadataStorage = nil },
		func(b *v1beta1.BrokerConfig) { b.MetadataStorage.MountPath = "/relocated" },
		func(b *v1beta1.BrokerConfig) {
			value := "different-class"
			b.MetadataStorage.PvcSpec.StorageClassName = &value
		},
		func(b *v1beta1.BrokerConfig) {
			b.MetadataStorage.PvcSpec.Resources.Requests[corev1.ResourceStorage] = resource.MustParse("1Gi")
		},
	} {
		next := broker.BrokerConfig.DeepCopy()
		change(next)
		require.Error(t, r.reconcileMetadataStorage(ctx, broker, next, log))
	}
	grown := broker.BrokerConfig.DeepCopy()
	grown.MetadataStorage.PvcSpec.Resources.Requests[corev1.ResourceStorage] = resource.MustParse("20Gi")
	require.NoError(t, r.reconcileMetadataStorage(ctx, broker, grown, log))
}

func TestMetadataStorageExistingSourcesAndDefault(t *testing.T) {
	r, broker := metadataTestReconciler(t)
	ctx, log := context.Background(), logr.Discard()
	existing := createPvc("old-data", "1", "/csi-kafka-logs1")
	existing.Namespace = "test"
	existing.Labels = apiutil.MergeLabels(existing.Labels, apiutil.LabelsForKafka("test"))
	require.NoError(t, r.Create(ctx, existing))
	require.NoError(t, r.reconcileMetadataStorage(ctx, broker, broker.BrokerConfig, log))
	pvc, err := r.metadataPVC(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, "false", pvc.Annotations[metadataFreshAnnotation])
	t.Run("existing pod without version or data PVC cannot be fresh", func(t *testing.T) {
		r, broker := metadataTestReconciler(t)
		pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Name: "existing", Namespace: "test",
			Labels: apiutil.MergeLabels(apiutil.LabelsForKafka("test"), map[string]string{v1beta1.BrokerIdLabelKey: "1"}),
		}}
		require.NoError(t, r.Create(ctx, pod))
		require.NoError(t, r.reconcileMetadataStorage(ctx, broker, broker.BrokerConfig, log))
		metadataPVC, err := r.metadataPVC(ctx, 1)
		require.NoError(t, err)
		require.Equal(t, "false", metadataPVC.Annotations[metadataFreshAnnotation])
	})

	for _, roles := range [][]string{{"broker"}, {"controller"}, {"broker", "controller"}} {
		b := broker.BrokerConfig.DeepCopy()
		b.MetadataStorage = nil
		b.Roles = roles
		pod := r.pod(1, b, []corev1.PersistentVolumeClaim{*existing}, log).(*corev1.Pod)
		require.Len(t, pod.Spec.InitContainers, 2)
		require.Contains(t, pod.Spec.Containers[0].Command[2], "QUORUM_STATE_FILE=")
		for _, volume := range pod.Spec.Volumes {
			require.NotEqual(t, metadataVolumeName, volume.Name)
		}
	}
}

func TestDataDiskRemovalRetainsMetadataPVC(t *testing.T) {
	r, broker := metadataTestReconciler(t)
	ctx, log := context.Background(), logr.Discard()
	second := broker.BrokerConfig.StorageConfigs[0].DeepCopy()
	second.MountPath = "/csi-kafka-logs2"
	broker.BrokerConfig.StorageConfigs = append(broker.BrokerConfig.StorageConfigs, *second)
	require.NoError(t, r.reconcileMetadataStorage(ctx, broker, broker.BrokerConfig, log))
	metadataPVC, err := r.metadataPVC(ctx, 1)
	require.NoError(t, err)
	for i, storage := range broker.BrokerConfig.StorageConfigs {
		pvc, err := r.pvc(1, i, storage, broker.BrokerConfig, true)
		require.NoError(t, err)
		pvc.Name = "data-" + string(rune('1'+i))
		pvc.GenerateName = ""
		pvc.Status.Phase = corev1.ClaimBound
		require.NoError(t, r.Create(ctx, pvc))
	}
	broker.BrokerConfig.StorageConfigs = []v1beta1.StorageConfig{*second}
	require.ErrorContains(t, r.reconcileMetadataStorage(ctx, broker, broker.BrokerConfig, log), "keep all existing data disks")
	require.Empty(t, r.KafkaCluster.Status.BrokersState["1"].MetadataStorageState)
	require.NoError(t, r.Get(ctx, client.ObjectKey{Namespace: "test", Name: "data-1"}, &corev1.PersistentVolumeClaim{}))
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "migrated", Namespace: "test",
			Labels: apiutil.MergeLabels(apiutil.LabelsForKafka("test"), map[string]string{v1beta1.BrokerIdLabelKey: "1"}),
		},
		Spec: corev1.PodSpec{Volumes: []corev1.Volume{{
			Name: metadataVolumeName,
			VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: metadataPVC.Name,
			}},
		}}},
		Status: corev1.PodStatus{
			Phase:      corev1.PodRunning,
			Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
			InitContainerStatuses: []corev1.ContainerStatus{{
				Name:  "migrate-broker-metadata",
				State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 0}},
			}},
		},
	}
	require.NoError(t, r.Create(ctx, pod))
	require.NoError(t, r.reconcileMetadataStorage(ctx, broker, broker.BrokerConfig, log))
	metadataPVC, err = r.metadataPVC(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, "true", metadataPVC.Annotations[metadataReadyAnnotation])
	stored := &v1beta1.KafkaCluster{}
	require.NoError(t, r.Get(ctx, client.ObjectKeyFromObject(r.KafkaCluster), stored))
	require.Equal(t, v1beta1.MetadataStorageReady, stored.Status.BrokersState["1"].MetadataStorageState)

	// The PVC annotation is durable: a lost status is projected again without the pod.
	require.NoError(t, r.Delete(ctx, pod))
	r.KafkaCluster.Status.BrokersState = nil
	require.NoError(t, r.Client.Status().Update(ctx, r.KafkaCluster))
	require.NoError(t, r.reconcileMetadataStorage(ctx, broker, broker.BrokerConfig, log))
	require.Equal(t, v1beta1.MetadataStorageReady, r.KafkaCluster.Status.BrokersState["1"].MetadataStorageState)

	r.KafkaCluster.Status.BrokersState = map[string]v1beta1.BrokerState{"1": {
		GracefulActionState: v1beta1.GracefulActionState{VolumeStates: map[string]v1beta1.VolumeState{
			"/csi-kafka-logs1": {CruiseControlVolumeState: v1beta1.GracefulDiskRemovalSucceeded},
			"/csi-kafka-logs2": {CruiseControlVolumeState: v1beta1.GracefulDiskRebalanceSucceeded},
		}},
	}}
	require.NoError(t, r.Client.Status().Update(ctx, r.KafkaCluster))
	desired, err := r.pvc(1, 1, *second, broker.BrokerConfig, true)
	require.NoError(t, err)
	require.NoError(t, r.reconcileKafkaPvc(ctx, log, map[string][]*corev1.PersistentVolumeClaim{"1": {desired}}, map[string]struct{}{"1": {}}))
	require.Error(t, r.Get(ctx, client.ObjectKey{Namespace: "test", Name: "data-1"}, &corev1.PersistentVolumeClaim{}))
	require.NoError(t, r.Get(ctx, client.ObjectKeyFromObject(metadataPVC), &corev1.PersistentVolumeClaim{}))
	pvcs, err := getCreatedPvcForBroker(ctx, r.Client, nil, 1, broker.BrokerConfig.StorageConfigs, "test", "test")
	require.NoError(t, err)
	require.Len(t, pvcs, 1)
	require.Equal(t, "data-2", pvcs[0].Name)
	require.NotContains(t, r.KafkaCluster.Status.BrokersState["1"].GracefulActionState.VolumeStates, "/csi-kafka-metadata")
}

func TestBrokerRemovalDeletesMetadataPVC(t *testing.T) {
	r, broker := metadataTestReconciler(t)
	ctx, log := context.Background(), logr.Discard()
	require.NoError(t, r.reconcileMetadataStorage(ctx, broker, broker.BrokerConfig, log))
	other := broker.DeepCopy()
	other.Id = 2
	require.NoError(t, r.reconcileMetadataStorage(ctx, *other, other.BrokerConfig, log))
	data, err := r.pvc(1, 0, broker.BrokerConfig.StorageConfigs[0], broker.BrokerConfig, true)
	require.NoError(t, err)
	data.Name, data.GenerateName = "data", ""
	require.NoError(t, r.Create(ctx, data))

	require.NoError(t, r.deleteMetadataPVC(ctx, "1", log))
	require.NoError(t, r.deleteMetadataPVC(ctx, "1", log))
	removed, err := r.metadataPVC(ctx, 1)
	require.NoError(t, err)
	require.Nil(t, removed)
	kept, err := r.metadataPVC(ctx, 2)
	require.NoError(t, err)
	require.NotNil(t, kept)
	require.NoError(t, r.Get(ctx, client.ObjectKey{Namespace: "test", Name: "data"}, &corev1.PersistentVolumeClaim{}))

	// A data PVC that is already gone must not prevent metadata PVC cleanup.
	require.NoError(t, r.reconcileMetadataStorage(ctx, broker, broker.BrokerConfig, log))
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "removed", Namespace: "test", Labels: map[string]string{v1beta1.BrokerIdLabelKey: "1"}},
		Spec: corev1.PodSpec{Volumes: []corev1.Volume{
			{Name: kafkaDataVolumeMount + "-0", VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "already-deleted"}}},
			{Name: kafkaDataVolumeMount + "-1", VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "data"}}},
		}},
	}
	require.NoError(t, r.deleteBrokerPVCs(ctx, pod, log))
	removed, err = r.metadataPVC(ctx, 1)
	require.NoError(t, err)
	require.Nil(t, removed)
	require.Error(t, r.Get(ctx, client.ObjectKey{Namespace: "test", Name: "data"}, &corev1.PersistentVolumeClaim{}))

	// The broker ID can be reused without metadata storage.
	withoutMetadata := broker.BrokerConfig.DeepCopy()
	withoutMetadata.MetadataStorage = nil
	require.NoError(t, r.reconcileMetadataStorage(ctx, broker, withoutMetadata, log))
}

func TestMetadataMigrationRecoveryGate(t *testing.T) {
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{Volumes: []corev1.Volume{{
			Name:         metadataVolumeName,
			VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "metadata"}},
		}}},
		Status: corev1.PodStatus{
			Phase:      corev1.PodRunning,
			Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
			InitContainerStatuses: []corev1.ContainerStatus{{
				Name:  "migrate-broker-metadata",
				State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 0}},
			}},
		},
	}
	require.True(t, metadataMigrationRecovered(pod, "metadata"))
	require.False(t, metadataMigrationRecovered(pod, "different-claim"))
	for _, change := range []func(*corev1.Pod){
		func(p *corev1.Pod) { p.Status.Phase = corev1.PodPending },
		func(p *corev1.Pod) { p.Status.Conditions = nil },
		func(p *corev1.Pod) { p.Status.InitContainerStatuses = nil },
		func(p *corev1.Pod) { p.Status.InitContainerStatuses[0].State.Terminated.ExitCode = 1 },
		func(p *corev1.Pod) { now := metav1.Now(); p.DeletionTimestamp = &now },
	} {
		next := pod.DeepCopy()
		change(next)
		require.False(t, metadataMigrationRecovered(next, "metadata"))
	}
}

func TestMetadataMigrationClusterIDEnvironment(t *testing.T) {
	r, broker := metadataTestReconciler(t)
	ctx, log := context.Background(), logr.Discard()
	require.NoError(t, r.reconcileMetadataStorage(ctx, broker, broker.BrokerConfig, log))
	pvc, err := r.metadataPVC(ctx, broker.Id)
	require.NoError(t, err)
	for _, clusterID := range []corev1.EnvVar{
		{Name: clusterIDEnvVarName, Value: "explicit-cluster"},
		{Name: clusterIDEnvVarName, ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "cluster-identity"},
				Key:                  "id",
			},
		}},
	} {
		r.KafkaCluster.Spec.Envs = []corev1.EnvVar{clusterID}
		pod := r.pod(broker.Id, broker.BrokerConfig, []corev1.PersistentVolumeClaim{*pvc}, log).(*corev1.Pod)
		require.Contains(t, pod.Spec.InitContainers[len(pod.Spec.InitContainers)-1].Env, clusterID)
	}
}

func TestMetadataLogDirDeferredWhilePodWithoutMetadataVolumeExists(t *testing.T) {
	r, broker := metadataTestReconciler(t)
	ctx, log := context.Background(), logr.Discard()
	metadataLogDir := func() (string, bool) {
		props, err := properties.NewFromString(r.generateBrokerConfig(broker, broker.BrokerConfig, nil, nil, nil, nil, nil, "", nil, log))
		require.NoError(t, err)
		value, found := props.Get(kafkautils.KafkaConfigMetadataLogDirectory)
		if !found {
			return "", false
		}
		return value.Value(), true
	}
	labels := apiutil.MergeLabels(apiutil.LabelsForKafka("test"), map[string]string{v1beta1.BrokerIdLabelKey: "1"})

	// The running pre-migration pod live-mounts the ConfigMap but has no metadata volume.
	oldPod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test-1-old", Namespace: "test", Labels: labels}}
	require.NoError(t, r.Create(ctx, oldPod))
	_, found := metadataLogDir()
	require.False(t, found)

	// No ConfigMap yet (rack awareness creates it after the pod): creation is allowed.
	require.NoError(t, r.ensureMetadataLogDirConfigured(ctx, "1", broker.BrokerConfig))
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "test-config-1", Namespace: "test"},
		Data:       map[string]string{kafkautils.ConfigPropertyName: "log.dirs=/csi-kafka-logs1/kafka\n"},
	}
	require.NoError(t, r.Create(ctx, configMap))
	err := r.ensureMetadataLogDirConfigured(ctx, "1", broker.BrokerConfig)
	require.Error(t, err)
	require.Contains(t, err.Error(), "metadata.log.dir")
	require.NoError(t, r.ensureMetadataLogDirConfigured(ctx, "1", &v1beta1.BrokerConfig{}))

	// Once the old pod is gone, the replacement is created with the setting.
	require.NoError(t, r.Delete(ctx, oldPod))
	value, found := metadataLogDir()
	require.True(t, found)
	require.Equal(t, "/csi-kafka-metadata/kafka", value)
	configMap.Data[kafkautils.ConfigPropertyName] = "log.dirs=/csi-kafka-logs1/kafka\nmetadata.log.dir=" + value + "\n"
	require.NoError(t, r.Update(ctx, configMap))
	require.NoError(t, r.ensureMetadataLogDirConfigured(ctx, "1", broker.BrokerConfig))

	// Once published it is never withdrawn, even if pod listing would defer it.
	stray := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test-1-stray", Namespace: "test", Labels: labels}}
	require.NoError(t, r.Create(ctx, stray))
	_, found = metadataLogDir()
	require.True(t, found)

	// A pod that mounts the metadata volume does not defer the setting.
	require.NoError(t, r.Delete(ctx, stray))
	require.NoError(t, r.Delete(ctx, configMap))
	newPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "test-1-new", Namespace: "test", Labels: labels},
		Spec: corev1.PodSpec{Volumes: []corev1.Volume{{Name: metadataVolumeName, VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "test-1-metadata"},
		}}}},
	}
	require.NoError(t, r.Create(ctx, newPod))
	_, found = metadataLogDir()
	require.True(t, found)
}

func TestMetadataMigrationGate(t *testing.T) {
	ctx := context.Background()
	labelsFor := func(id string) map[string]string {
		return apiutil.MergeLabels(apiutil.LabelsForKafka("test"), map[string]string{v1beta1.BrokerIdLabelKey: id})
	}
	oldPod := func(id string) *corev1.Pod {
		return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test-" + id, Namespace: "test", Labels: labelsFor(id)}}
	}
	migratedPod := func(id string) *corev1.Pod {
		pod := oldPod(id)
		pod.Spec.Volumes = []corev1.Volume{{Name: metadataVolumeName, VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "test-" + id + "-metadata"},
		}}}
		return pod
	}
	for _, tc := range []struct {
		name      string
		setup     func(*Reconciler)
		pods      []*corev1.Pod
		outOfSync []int32
		blocked   bool
	}{
		{name: "no migration in progress", pods: []*corev1.Pod{oldPod("2"), oldPod("3")}},
		{name: "migration already started in this pass", pods: []*corev1.Pod{oldPod("2"), oldPod("3")},
			setup: func(r *Reconciler) { r.metadataMigrationStarted = true }, blocked: true},
		{name: "replacement pod not Ready yet", pods: []*corev1.Pod{migratedPod("2"), oldPod("3")}, blocked: true},
		{name: "previous migration Ready", pods: []*corev1.Pod{migratedPod("2"), oldPod("3")},
			setup: func(r *Reconciler) {
				r.KafkaCluster.Status.BrokersState = map[string]v1beta1.BrokerState{"2": {MetadataStorageState: v1beta1.MetadataStorageReady}}
			}},
		{name: "old pod deleted, replacement missing", pods: []*corev1.Pod{oldPod("3")}, blocked: true},
		{name: "old pod terminating", pods: []*corev1.Pod{oldPod("2"), func() *corev1.Pod {
			pod := oldPod("3")
			pod.Finalizers = []string{"test"}
			pod.DeletionTimestamp = &metav1.Time{}
			return pod
		}()}, blocked: true},
		{name: "broker without metadata storage is ignored", pods: []*corev1.Pod{oldPod("2")},
			setup: func(r *Reconciler) {
				r.KafkaCluster.Spec.Brokers[2].BrokerConfig = &v1beta1.BrokerConfig{Roles: []string{"broker"}}
			}},
		{name: "another broker out of sync", pods: []*corev1.Pod{oldPod("2"), oldPod("3")}, outOfSync: []int32{2}, blocked: true},
		{name: "only the migrating broker out of sync", pods: []*corev1.Pod{oldPod("2"), oldPod("3")}, outOfSync: []int32{1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, broker := metadataTestReconciler(t)
			r.KafkaCluster.Spec.Brokers = []v1beta1.Broker{broker, {Id: 2, BrokerConfig: broker.BrokerConfig.DeepCopy()}, {Id: 3, BrokerConfig: broker.BrokerConfig.DeepCopy()}}
			if tc.setup != nil {
				tc.setup(r)
			}
			current := oldPod("1")
			require.NoError(t, r.Create(ctx, current))
			for _, pod := range tc.pods {
				deleting := pod.DeletionTimestamp
				pod.DeletionTimestamp = nil
				require.NoError(t, r.Create(ctx, pod))
				if deleting != nil {
					require.NoError(t, r.Delete(ctx, pod))
				}
			}
			kafkaClient := mocks.NewMockKafkaClient(gomock.NewController(t))
			kafkaClient.EXPECT().AllOfflineReplicas().Return(nil, nil).AnyTimes()
			kafkaClient.EXPECT().OutOfSyncReplicas().Return(tc.outOfSync, nil).AnyTimes()
			provider := new(kafkaclient.MockedProvider)
			provider.On("NewFromCluster", r.Client, r.KafkaCluster).Return(kafkaClient, func() {}, nil)
			r.kafkaClientProvider = provider

			err := r.metadataMigrationGate(ctx, current)
			if tc.blocked {
				require.Error(t, err)
				require.True(t, errors.As(err, &errorfactory.ReconcileRollingUpgrade{}))
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestMetadataMigrationRestartSerializedWithinPass(t *testing.T) {
	r, broker := metadataTestReconciler(t)
	ctx := context.Background()
	r.KafkaCluster.Spec.Brokers = []v1beta1.Broker{broker, {Id: 2, BrokerConfig: broker.BrokerConfig.DeepCopy()}}
	kafkaClient := mocks.NewMockKafkaClient(gomock.NewController(t))
	kafkaClient.EXPECT().AllOfflineReplicas().Return(nil, nil).AnyTimes()
	kafkaClient.EXPECT().OutOfSyncReplicas().Return(nil, nil).AnyTimes()
	provider := new(kafkaclient.MockedProvider)
	provider.On("NewFromCluster", r.Client, r.KafkaCluster).Return(kafkaClient, func() {}, nil)
	r.kafkaClientProvider = provider

	podFor := func(id string, metadata bool) *corev1.Pod {
		pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test-" + id, Namespace: "test",
			Labels: apiutil.MergeLabels(apiutil.LabelsForKafka("test"), map[string]string{v1beta1.BrokerIdLabelKey: id})}}
		if metadata {
			pod.Spec.Volumes = []corev1.Volume{{Name: metadataVolumeName, VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "test-" + id + "-metadata"},
			}}}
		}
		return pod
	}
	require.True(t, isMetadataMigrationRestart(podFor("1", false), podFor("1", true)))
	require.False(t, isMetadataMigrationRestart(podFor("1", true), podFor("1", true)))
	require.False(t, isMetadataMigrationRestart(podFor("1", false), podFor("1", false)))

	// Crashed containers bypass the generic rolling-upgrade gates, not the migration gate.
	crashed := func(pod *corev1.Pod) *corev1.Pod {
		pod.Status.ContainerStatuses = []corev1.ContainerStatus{{Name: "kafka", State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 1}}}}
		return pod
	}
	first, second := crashed(podFor("1", false)), crashed(podFor("2", false))
	require.NoError(t, r.Create(ctx, first))
	require.NoError(t, r.Create(ctx, second))
	require.NoError(t, r.handleRollingUpgrade(logr.Discard(), podFor("1", true), first, reflect.TypeOf(first)))
	require.True(t, r.metadataMigrationStarted)
	err := r.handleRollingUpgrade(logr.Discard(), podFor("2", true), second, reflect.TypeOf(second))
	require.Error(t, err)
	require.True(t, errors.As(err, &errorfactory.ReconcileRollingUpgrade{}))
	require.NoError(t, r.Get(ctx, client.ObjectKeyFromObject(second), &corev1.Pod{}))
}
