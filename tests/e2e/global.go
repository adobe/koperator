// Copyright © 2023 Cisco Systems, Inc. and/or its affiliates
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
	"errors"
	"os"
	"strings"
)

// HelmDescriptors.
var (
	// certManagerHelmDescriptor describes the cert-manager Helm component.
	certManagerHelmDescriptor = helmDescriptor{
		Repository:   "https://charts.jetstack.io",
		ChartName:    certManagerName,
		ChartVersion: CertManagerVersion,
		ReleaseName:  certManagerName,
		Namespace:    certManagerName,
		SetValues: map[string]string{
			"installCRDs": falseString,
		},
		RemoteCRDPathVersionTemplate: "https://github.com/jetstack/cert-manager/releases/download/v%s/cert-manager.crds.yaml",
		HelmExtraArguments: map[string][]string{
			installAction: {timeoutFlag, timeoutValue},
		},
	}
	// contour ingress controller
	// Envoy service is set to NodePort so Helm --atomic does not wait for a LoadBalancer
	// ingress IP (which never comes on kind without MetalLB).
	contourIngressControllerHelmDescriptor = helmDescriptor{
		Repository:   "https://projectcontour.github.io/helm-charts",
		ChartName:    contourName,
		ChartVersion: ContourVersion,
		ReleaseName:  contourName,
		Namespace:    "projectcontour",
		SetValues: map[string]string{
			"contour.manageCRDs": verboseLoggingEnabled,
			"envoy.service.type": "NodePort",
		},
		HelmExtraArguments: map[string][]string{
			installAction: {timeoutFlag, timeoutValue},
		},
	}

	// koperatorLocalHelmDescriptor describes the Koperator Helm component with
	// a local chart and version.
	koperatorLocalHelmDescriptor = func() helmDescriptor {
		koperatorLocalHelmDescriptor := helmDescriptor{
			Repository:   "../../charts/kafka-operator",
			ChartVersion: LocalVersion,
			ReleaseName:  "kafka-operator",
			Namespace:    "kafka",
			LocalCRDSubpaths: []string{
				"crds/cruisecontroloperations.yaml",
				"crds/kafkaclusters.yaml",
				"crds/kafkatopics.yaml",
				"crds/kafkausers.yaml",
			},
			// Contour is installed as a dependency above (with its HTTPProxy
			// CRD), so enable the operator's Contour integration to exercise it.
			SetValues: map[string]string{
				"contour.enabled": trueString,
			},
		}
		// Set helm chart values for Koperator to be able to use custom image
		koperatorImagePath := os.Getenv("IMG_E2E")
		if koperatorImagePath != "" {
			koperatorImagePathSplit := strings.Split(koperatorImagePath, ":")

			koperatorImageRepository := koperatorImagePathSplit[0]
			koperatorImageTag := "latest"

			if len(koperatorImagePathSplit) == 2 {
				koperatorImageTag = koperatorImagePathSplit[1]
			}

			koperatorLocalHelmDescriptor.SetValues["operator.image.repository"] = koperatorImageRepository
			koperatorLocalHelmDescriptor.SetValues["operator.image.tag"] = koperatorImageTag
		}

		return koperatorLocalHelmDescriptor
	}()

	// prometheusOperatorHelmDescriptor describes the prometheus-operator Helm
	// component.
	prometheusOperatorHelmDescriptor = helmDescriptor{
		Repository:   "https://prometheus-community.github.io/helm-charts",
		ChartName:    "kube-prometheus-stack",
		ChartVersion: PrometheusOperatorVersion,
		ReleaseName:  "prometheus-operator",
		Namespace:    "prometheus",
		SetValues: map[string]string{
			"crds.enabled":                  verboseLoggingEnabled,
			"defaultRules.enabled":          falseString,
			"alertmanager.enabled":          falseString,
			"grafana.enabled":               falseString,
			"kubeApiServer.enabled":         falseString,
			"kubelet.enabled":               falseString,
			"kubeControllerManager.enabled": falseString,
			"coreDNS.enabled":               falseString,
			"kubeEtcd.enabled":              falseString,
			"kubeScheduler.enabled":         falseString,
			"kubeProxy.enabled":             falseString,
			"kubeStateMetrics.enabled":      falseString,
			"nodeExporter.enabled":          falseString,
			"prometheus.enabled":            falseString,
		},
		HelmExtraArguments: map[string][]string{
			installAction: {timeoutFlag, timeoutValue},
		},
	}

	// zookeeperOperatorHelmDescriptor describes the zookeeper-operator Helm
	// component.
	zookeeperOperatorHelmDescriptor = helmDescriptor{
		Repository:   "",
		ChartName:    "oci://ghcr.io/adobe/helm-charts/zookeeper-operator",
		ChartVersion: ZookeeperOperatorVersion,
		ReleaseName:  "zookeeper-operator",
		Namespace:    "zookeeper",
		SetValues: map[string]string{
			"crd.create":       falseString,
			"image.repository": "ghcr.io/adobe/zookeeper-operator",
			"image.tag":        ZookeeperOperatorVersion,
		},
		RemoteCRDPathVersionTemplate: "https://raw.githubusercontent.com/adobe/zookeeper-operator/%s/config/crd/bases/zookeeper.pravega.io_zookeeperclusters.yaml",
		HelmExtraArguments: map[string][]string{
			installAction: {timeoutFlag, timeoutValue},
		},
	}

	// dependencyCRDs storing the Koperator dependencies CRDs name
	// It should be initialized once with the Initialize() member function
	dependencyCRDs dependencyCRDsType

	// ErrorNotFound is for handling that error case when resource is not found
	ErrorNotFound = errors.New("not found")
)
