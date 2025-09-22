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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	"emperror.dev/errors"
	"github.com/gruntwork-io/terratest/modules/helm"
	"github.com/gruntwork-io/terratest/modules/k8s"
	ginkgo "github.com/onsi/ginkgo/v2"
	"sigs.k8s.io/yaml"
)

// HelmDescriptor describes a component that can be operated on by Helm.
// Repository+ChartName and ChartPath are mutually exclusive.
type helmDescriptor struct {
	Repository                   string
	ChartName                    string
	ChartVersion                 string
	ReleaseName                  string
	Namespace                    string
	SetValues                    map[string]string
	HelmExtraArguments           map[string][]string
	RemoteCRDPathVersionTemplate string
	LocalCRDSubpaths             []string
	LocalCRDTemplateRenderValues map[string]string
}

// crdPath returns the path of the CRD belonging to the Helm descriptor based on
// the chart version and local/remote Helm chart.
func (helmDescriptor *helmDescriptor) crdPath() (string, error) { //nolint:unused // Note: this might come in handy for manual CRD operations such as too long CRDs.
	if helmDescriptor == nil {
		return "", errors.Errorf("invalid nil Helm descriptor")
	}

	if helmDescriptor.IsRemote() {
		return fmt.Sprintf(
			helmDescriptor.RemoteCRDPathVersionTemplate,
			strings.TrimPrefix(helmDescriptor.ChartVersion, "v"),
		), nil
	}

	localCRDsBytes := []byte(helm.RenderTemplate(
		ginkgo.GinkgoT(),
		&helm.Options{
			SetValues: helmDescriptor.LocalCRDTemplateRenderValues,
		},
		helmDescriptor.Repository,
		helmDescriptor.ReleaseName,
		[]string{
			"crds/cruisecontroloperations.yaml",
			"crds/kafkaclusters.yaml",
			"crds/kafkatopics.yaml",
			"crds/kafkausers.yaml",
		},
	))

	return createTempFileFromBytes(localCRDsBytes, "", "", 0)
}

// downloadAndInstallRemoteCRDs downloads CRDs from RemoteCRDPathVersionTemplate and installs them
func (helmDescriptor *helmDescriptor) downloadAndInstallRemoteCRDs(kubectlOptions k8s.KubectlOptions) error {
	if helmDescriptor.RemoteCRDPathVersionTemplate == "" {
		return nil // No remote CRDs to install
	}

	// Generate the CRD URL using the version template
	crdURL := fmt.Sprintf(
		helmDescriptor.RemoteCRDPathVersionTemplate,
		strings.TrimPrefix(helmDescriptor.ChartVersion, "v"),
	)

	ginkgo.By(fmt.Sprintf("Downloading CRD from %s", crdURL))

	// Download the CRD content with retry logic
	var resp *http.Response
	var err error
	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		ginkgo.By(fmt.Sprintf("Downloading attempt %d/%d", i+1, maxRetries))
		resp, err = http.Get(crdURL)
		if err == nil && resp.StatusCode == http.StatusOK {
			break
		}
		if resp != nil {
			resp.Body.Close()
		}
		if i < maxRetries-1 {
			ginkgo.By(fmt.Sprintf("Download failed, retrying in 2 seconds... Error: %v", err))
			time.Sleep(2 * time.Second)
		}
	}

	if err != nil {
		return errors.WrapIfWithDetails(err, "downloading remote CRD failed after retries", "url", crdURL)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.NewWithDetails("remote CRD download failed", "url", crdURL, "status", resp.StatusCode)
	}

	crdContent, err := io.ReadAll(resp.Body)
	if err != nil {
		return errors.WrapIfWithDetails(err, "reading remote CRD content failed", "url", crdURL)
	}

	ginkgo.By("Installing downloaded CRD")

	// Install the CRD
	return installK8sCRD(kubectlOptions, crdContent, false)
}

// installHelmChart checks whether the specified named Helm release exists in
// the provided kubectl context and namespace, logs it if it does and returns or
// alternatively deploys a Helm chart to the specified kubectl context and
// namespace using the specified info, extra arguments can be any of the helm
// CLI install flag arguments, flag keys and values must be provided separately.
func (helmDescriptor *helmDescriptor) installHelmChart(kubectlOptions k8s.KubectlOptions) error {
	if helmDescriptor == nil {
		return errors.Errorf("invalid nil Helm descriptor")
	}

	kubectlOptions.Namespace = helmDescriptor.Namespace

	if !helmDescriptor.IsRemote() { // Note: local chart with directory path in helmDescriptor.Repository.
		ginkgo.By("Discovering local chart name and version")

		chartYAMLPath := path.Join(helmDescriptor.Repository, "Chart.yaml")
		chartYAMLBytes, err := os.ReadFile(chartYAMLPath)
		if err != nil {
			return errors.WrapIfWithDetails(err, "reading local chart YAML failed", "path", chartYAMLPath)
		}

		var chartYAML map[string]interface{}
		err = yaml.Unmarshal(chartYAMLBytes, &chartYAML)
		if err != nil {
			return errors.WrapIfWithDetails(
				err,
				"parsing local chart YAML failed",
				"path", chartYAMLPath,
				"content", string(chartYAMLBytes),
			)
		}

		var isOk bool

		helmDescriptor.ChartName, isOk = chartYAML["name"].(string)
		if !isOk {
			return errors.NewWithDetails("chartYAML contains no string chart name", "chartYAML", chartYAML)
		}

		helmDescriptor.ChartVersion, isOk = chartYAML["version"].(string)
		if !isOk {
			return errors.NewWithDetails("chartYAML contains no string chart version", "chartYAML", chartYAML)
		}
	}

	ginkgo.By(fmt.Sprintf("Checking for existing Helm release named %s", helmDescriptor.ReleaseName))
	helmRelease, isInstalled, err := lookUpInstalledHelmReleaseByName(kubectlOptions, helmDescriptor.ReleaseName)
	if err != nil {
		return errors.WrapIfWithDetails(
			err,
			"looking up Helm release failed",
			"releaseName", helmDescriptor.ReleaseName,
		)
	}

	switch {
	case isInstalled:
		installedChartName, installedChartVersion := helmRelease.chartNameAndVersion()

		if installedChartName != helmDescriptor.ChartName {
			return errors.Errorf(
				"Installed Helm chart name '%s' mismatches Helm descriptor chart name to be installed '%s'",
				installedChartName, helmDescriptor.ChartName,
			)
		}

		if installedChartVersion != helmDescriptor.ChartVersion {
			return errors.Errorf(
				"Installed Helm chart version '%s' mismatches Helm descriptor chart version to be installed '%s'",
				installedChartVersion, helmDescriptor.ChartVersion,
			)
		}

		ginkgo.By(fmt.Sprintf(
			"Skipping the installation of existing Helm release %s, with the same chart name (%s) and version (%s)",
			helmDescriptor.ReleaseName, helmDescriptor.ChartName, helmDescriptor.ChartVersion,
		))

		return nil
	case !isInstalled:
		// Install remote CRDs if specified
		if helmDescriptor.RemoteCRDPathVersionTemplate != "" {
			ginkgo.By("Installing remote CRDs before Helm chart installation")
			err := helmDescriptor.downloadAndInstallRemoteCRDs(kubectlOptions)
			if err != nil {
				return errors.WrapIfWithDetails(
					err,
					"installing remote CRDs failed",
					"releaseName", helmDescriptor.ReleaseName,
				)
			}
		}

		ginkgo.By(
			fmt.Sprintf(
				"Installing Helm chart %s from %s with version %s by name %s",
				helmDescriptor.ChartName,
				helmDescriptor.Repository,
				helmDescriptor.ChartVersion,
				helmDescriptor.ReleaseName,
			),
		)

		fixedArguments := []string{
			"--create-namespace",
			"--atomic",
			"--debug",
		}

		helmChartNameOrLocalPath := helmDescriptor.ChartName

		if !helmDescriptor.IsRemote() {
			helmChartNameOrLocalPath = helmDescriptor.Repository
		} else if helmDescriptor.Repository != "" { // && helmDescriptor.IsRemote() {
			fixedArguments = append([]string{"--repo", helmDescriptor.Repository}, fixedArguments...)
		}

		helm.Install(
			ginkgo.GinkgoT(),
			&helm.Options{
				SetValues:      helmDescriptor.SetValues,
				KubectlOptions: &kubectlOptions,
				Version:        helmDescriptor.ChartVersion,
				ExtraArgs: map[string][]string{
					"install": append(fixedArguments, helmDescriptor.HelmExtraArguments["install"]...),
				},
			},
			helmChartNameOrLocalPath,
			helmDescriptor.ReleaseName,
		)
	}

	return nil
}

// uninstallHelmChart checks whether the specified named Helm release exists in
// the provided kubectl context and namespace, logs it if it does not and when noErrorNotFound is false then it returns error.
// if the Helm chart present then it uninstalls it from the specified kubectl context
// and namespace using the specified info, extra arguments can be any of the helm
// CLI install flag arguments, flag keys and values must be provided separately.
func (helmDescriptor *helmDescriptor) uninstallHelmChart(kubectlOptions k8s.KubectlOptions, noErrorNotFound bool) error { //nolint:unparam // Note: library function with noErrorNotFound argument currently always receiving true.
	if helmDescriptor == nil {
		return errors.Errorf("invalid nil Helm descriptor")
	}

	kubectlOptions.Namespace = helmDescriptor.Namespace

	ginkgo.By(fmt.Sprintf("Checking for existing Helm release named %s", helmDescriptor.ReleaseName))
	_, isInstalled, err := lookUpInstalledHelmReleaseByName(kubectlOptions, helmDescriptor.ReleaseName)
	if err != nil {
		return errors.WrapIfWithDetails(
			err,
			"looking up Helm release failed",
			"releaseName", helmDescriptor.ReleaseName,
		)
	}

	if !isInstalled {
		if !noErrorNotFound {
			return errors.Errorf("Helm release: '%s' not found", helmDescriptor.ReleaseName)
		}

		ginkgo.By(fmt.Sprintf(
			"skipping the uninstallation of %s, because the Helm release is not present.",
			helmDescriptor.ReleaseName,
		))
		return nil
	}
	ginkgo.By(
		fmt.Sprintf(
			"uninstalling Helm chart by name %s",
			helmDescriptor.ReleaseName,
		),
	)

	fixedArguments := []string{
		"--debug",
		"--wait",
		"--cascade=foreground",
	}
	purge := true

	return helm.DeleteE(
		ginkgo.GinkgoT(),
		&helm.Options{
			KubectlOptions: &kubectlOptions,
			ExtraArgs: map[string][]string{
				"delete": append(fixedArguments, helmDescriptor.HelmExtraArguments["delete"]...),
			},
		},
		helmDescriptor.ReleaseName,
		purge,
	)
}

// IsRemote returns true when the Helm descriptor uses a remote chart path as
// location. In any other case the repository is considered a remote Helm
// repository URL.
func (helmDescriptor *helmDescriptor) IsRemote() bool {
	return helmDescriptor.Repository == "" || // Note: default repository.
		strings.HasPrefix(helmDescriptor.Repository, "https://") || // Note: explicit repository.
		strings.HasPrefix(helmDescriptor.Repository, "oci://") // Note: OCI registry.
}

// HelmReleaseStatus describes the possible states of a Helm release.
type helmReleaseStatus string

const (
	// HelmReleaseDeployed is the Helm release state where the deployment is
	// successfully applied to the cluster.
	HelmReleaseDeployed helmReleaseStatus = "deployed"

	// HelmReleaseFailed is the Helm release state where the deployment
	// encountered an error and couldn't be applied successfully to the cluster.
	HelmReleaseFailed helmReleaseStatus = "failed"
)

// HelmRelease describes a Helm release that can be listed by the Helm CLI.
type HelmRelease struct {
	ReleaseName string            `json:"name" yaml:"name"`
	Namespace   string            `json:"namespace" yaml:"namespace"`
	Revision    string            `json:"revision" yaml:"revision"`
	UpdatedTime string            `json:"updated" yaml:"updated"` // Note: not parsable implicitly.
	Status      helmReleaseStatus `json:"status" yaml:"status"`
	Chart       string            `json:"chart" yaml:"chart"`
	AppVersion  string            `json:"app_version" yaml:"app_version"`
}

// ChartVersion returns the version of the chart in the Helm release.
func (helmRelease *HelmRelease) chartNameAndVersion() (string, string) {
	if helmRelease == nil {
		return "", ""
	}

	semverRawRegex := `(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-((?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?(?:\+([0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?` // Note: https://semver.org/, https://regex101.com/r/vkijKf/1/
	chartRegex := regexp.MustCompile(`(.+)-(v?` + semverRawRegex + `)`)
	groups := chartRegex.FindStringSubmatch(helmRelease.Chart)
	if len(groups) < 3 {
		return "", ""
	}

	return groups[1], groups[2]
}

// listHelmReleases returns a slice of Helm releases retrieved from the cluster
// using the specified kubectl context and namespace.
func listHelmReleases(kubectlOptions k8s.KubectlOptions) ([]*HelmRelease, error) {
	ginkgo.By("Listing Helm releases")
	output, err := helm.RunHelmCommandAndGetOutputE(
		ginkgo.GinkgoT(),
		&helm.Options{
			KubectlOptions: &kubectlOptions,
		},
		"list",
		"--output", "json",
	)

	if err != nil {
		return nil, errors.WrapIf(err, "listing Helm releases failed")
	}

	var releases []*HelmRelease
	err = json.Unmarshal([]byte(output), &releases)
	if err != nil {
		return nil, errors.WrapIfWithDetails(err, "parsing Helm releases failed", "output", output)
	}

	return releases, nil
}

// lookUpInstalledHelmReleaseByName returns a Helm release and an indicator
// whether the Helm release is installed to the specified kubectl context
// and namespace by the provided Helm release name.
func lookUpInstalledHelmReleaseByName(
	kubectlOptions k8s.KubectlOptions,
	helmReleaseName string,
) (*HelmRelease, bool, error) {
	releases, err := listHelmReleases(kubectlOptions)
	if err != nil {
		if err != nil {
			return nil, false, errors.WrapIfWithDetails(err, "listing Helm releases failed")
		}
	}

	for _, release := range releases {
		if release.ReleaseName == helmReleaseName {
			if release.Status != HelmReleaseDeployed {
				return nil, false, errors.Errorf("Helm release found with not deployed status %s", release.Status)
			}

			return release, true, nil
		}
	}

	return nil, false, nil
}
