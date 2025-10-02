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
		ChartName:    "cert-manager",
		ChartVersion: "v1.18.2",
		ReleaseName:  "cert-manager",
		Namespace:    "cert-manager",
		SetValues: map[string]string{
			"installCRDs": "false",
		},
		RemoteCRDPathVersionTemplate: "https://github.com/jetstack/cert-manager/releases/download/v%s/cert-manager.crds.yaml",
		HelmExtraArguments: map[string][]string{
			"install": {"--timeout", "10m"},
		},
	}
	// contour ingress controller
	contourIngressControllerHelmDescriptor = helmDescriptor{
		Repository:   "https://projectcontour.github.io/helm-charts",
		ChartName:    "contour",
		ChartVersion: "0.1.0",
		ReleaseName:  "contour",
		Namespace:    "projectcontour",
		SetValues: map[string]string{
			"contour.manageCRDs": "true",
		},
		HelmExtraArguments: map[string][]string{
			"install": {"--timeout", "10m"},
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

			koperatorLocalHelmDescriptor.SetValues = map[string]string{
				"operator.image.repository": koperatorImageRepository,
				"operator.image.tag":        koperatorImageTag,
			}
		}

		return koperatorLocalHelmDescriptor
	}()

	// prometheusOperatorHelmDescriptor describes the prometheus-operator Helm
	// component.
	prometheusOperatorHelmDescriptor = helmDescriptor{
		Repository:   "https://prometheus-community.github.io/helm-charts",
		ChartName:    "kube-prometheus-stack",
		ChartVersion: "77.12.0",
		ReleaseName:  "prometheus-operator",
		Namespace:    "prometheus",
		SetValues: map[string]string{
			"crds.enabled":                  "true",
			"defaultRules.enabled":          "false",
			"alertmanager.enabled":          "false",
			"grafana.enabled":               "false",
			"kubeApiServer.enabled":         "false",
			"kubelet.enabled":               "false",
			"kubeControllerManager.enabled": "false",
			"coreDNS.enabled":               "false",
			"kubeEtcd.enabled":              "false",
			"kubeScheduler.enabled":         "false",
			"kubeProxy.enabled":             "false",
			"kubeStateMetrics.enabled":      "false",
			"nodeExporter.enabled":          "false",
			"prometheus.enabled":            "false",
		},
		HelmExtraArguments: map[string][]string{
			"install": {"--timeout", "10m"},
		},
	}

	// zookeeperOperatorHelmDescriptor describes the zookeeper-operator Helm
	// component.
	zookeeperOperatorHelmDescriptor = helmDescriptor{
		Repository:   "",
		ChartName:    "oci://ghcr.io/adobe/helm-charts/zookeeper-operator",
		ChartVersion: "0.2.15-adobe-20250923",
		ReleaseName:  "zookeeper-operator",
		Namespace:    "zookeeper",
		SetValues: map[string]string{
			"crd.create": "false",
		},
		RemoteCRDPathVersionTemplate: "https://raw.githubusercontent.com/adobe/zookeeper-operator/%s/config/crd/bases/zookeeper.pravega.io_zookeeperclusters.yaml",
		HelmExtraArguments: map[string][]string{
			"install": {"--timeout", "10m"},
		},
	}

	// dependencyCRDs storing the Koperator dependencies CRDs name
	// It should be initialized once with the Initialize() member function
	dependencyCRDs dependencyCRDsType

	// ErrorNotFound is for handling that error case when resource is not found
	ErrorNotFound = errors.New("not found")
)
