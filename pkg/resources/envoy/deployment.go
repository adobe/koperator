// Copyright © 2019 Banzai Cloud
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

package envoy

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/banzaicloud/kafka-operator/api/v1beta1"
	"github.com/banzaicloud/kafka-operator/pkg/resources/templates"
	"github.com/banzaicloud/kafka-operator/pkg/util"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (r *Reconciler) deployment(log logr.Logger, envoyConfig *v1beta1.EnvoyConfig) runtime.Object {
	return &appsv1.Deployment{
		ObjectMeta: templates.ObjectMeta(deploymentName(envoyConfig), labelSelector(envoyConfig), r.KafkaCluster),
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labelSelector(envoyConfig),
			},
			Replicas: util.Int32Pointer(envoyConfig.GetReplicas()),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      labelSelector(envoyConfig),
					Annotations: generatePodAnnotations(r.KafkaCluster, envoyConfig, log),
				},
				Spec: getPodSpec(log, envoyConfig, r.KafkaCluster),
			},
		},
	}
}

func deploymentName(envoyConfig *v1beta1.EnvoyConfig) string {
	if envoyConfig.Id == envoyGlobal {
		return envoyDeploymentName
	} else {
		return fmt.Sprintf("%s-%s", envoyDeploymentName, envoyConfig.Id)
	}
}

func getPodSpec(log logr.Logger, envoyConfig *v1beta1.EnvoyConfig, kafkaCluster *v1beta1.KafkaCluster) corev1.PodSpec {
	exposedPorts := getExposedContainerPorts(envoyConfig, kafkaCluster.Spec.ListenersConfig.ExternalListeners, kafkaCluster.Spec)
	volumeMounts := []corev1.VolumeMount{
		{
			Name:      configName(envoyConfig),
			MountPath: "/etc/envoy",
			ReadOnly:  true,
		},
	}
	volumes := []corev1.Volume{
		{
			Name: configName(envoyConfig),
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: configName(envoyConfig)},
					DefaultMode:          util.Int32Pointer(0644),
				},
			},
		},
	}

	podSpec := corev1.PodSpec{
		ServiceAccountName: kafkaCluster.Spec.EnvoyConfig.GetServiceAccount(),
		ImagePullSecrets:   kafkaCluster.Spec.EnvoyConfig.GetImagePullSecrets(),
		Tolerations:        kafkaCluster.Spec.EnvoyConfig.GetTolerations(),
		NodeSelector:       kafkaCluster.Spec.EnvoyConfig.GetNodeSelector(),
		Containers: []corev1.Container{
			{
				Name:  "envoy",
				Image: kafkaCluster.Spec.EnvoyConfig.GetEnvoyImage(),
				Ports: append(exposedPorts, []corev1.ContainerPort{
					{Name: "envoy-admin", ContainerPort: 9901, Protocol: corev1.ProtocolTCP}}...),
				VolumeMounts: volumeMounts,
				Resources:    *kafkaCluster.Spec.EnvoyConfig.GetResources(),
			},
		},
		Volumes: volumes,
	}
	if envoyConfig.NodeAffinity != nil {
		podSpec.Affinity = &corev1.Affinity{
			NodeAffinity: envoyConfig.NodeAffinity,
		}
	}
	return podSpec
}

func getExposedContainerPorts(envoyConfig *v1beta1.EnvoyConfig, extListeners []v1beta1.ExternalListenerConfig, clusterSpec v1beta1.KafkaClusterSpec) []corev1.ContainerPort {
	var exposedPorts []corev1.ContainerPort

	for _, eListener := range extListeners {
		for _, broker := range clusterSpec.Brokers {
			if envoyConfig.Id == envoyGlobal || envoyConfig.Id == broker.BrokerConfigGroup {
				exposedPorts = append(exposedPorts, corev1.ContainerPort{
					Name:          fmt.Sprintf("broker-%d", broker.Id),
					ContainerPort: eListener.ExternalStartingPort + broker.Id,
					Protocol:      corev1.ProtocolTCP,
				})
			}
		}
	}
	return exposedPorts
}

func generatePodAnnotations(kafkaCluster *v1beta1.KafkaCluster, envoyConfig *v1beta1.EnvoyConfig, log logr.Logger) map[string]string {
	hashedEnvoyConfig := sha256.Sum256([]byte(GenerateEnvoyConfig(kafkaCluster, envoyConfig, log)))
	annotations := map[string]string{
		"envoy.yaml.hash": hex.EncodeToString(hashedEnvoyConfig[:]),
	}
	return annotations
}
