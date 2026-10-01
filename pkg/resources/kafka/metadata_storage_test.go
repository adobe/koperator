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
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	apiutil "github.com/banzaicloud/koperator/api/util"
	"github.com/banzaicloud/koperator/api/v1beta1"
	"github.com/banzaicloud/koperator/pkg/resources"
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
