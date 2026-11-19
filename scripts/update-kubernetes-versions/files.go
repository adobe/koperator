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

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

const pinsPath = "config/kubernetes-versions.env"

func readPins(root string) (targets, error) {
	data, err := os.ReadFile(filepath.Join(root, pinsPath))
	if err != nil {
		return targets{}, err
	}
	values := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || (key != "KUBERNETES_MIN_VERSION" && key != "KUBERNETES_MAX_VERSION") ||
			values[key] != "" || numbers(value) == nil || strings.HasPrefix(value, "v") {
			return targets{}, fmt.Errorf("invalid Kubernetes pin %q", line)
		}
		values[key] = value
	}
	result := targets{Minimum: values["KUBERNETES_MIN_VERSION"], Latest: values["KUBERNETES_MAX_VERSION"]}
	if result.Minimum == "" || result.Latest == "" || compareVersion(result.Minimum, result.Latest) > 0 {
		return targets{}, fmt.Errorf("invalid Kubernetes boundary pins")
	}
	return result, nil
}

type replacement struct {
	pattern string
	value   string
}

func renderFiles(root string, pins targets) (map[string][]byte, error) {
	n := numbers(pins.Minimum)
	if n == nil || numbers(pins.Latest) == nil || compareVersion(pins.Minimum, pins.Latest) > 0 {
		return nil, fmt.Errorf("invalid discovered Kubernetes pins")
	}
	floor := fmt.Sprintf("%d.%d.0", n[0], n[1])
	supportText := fmt.Sprintf(
		"Koperator requires **Kubernetes %s or newer** and tracks upstream-maintained minor releases. "+
			"CI tests %s and %s using envtest and Kind. "+
			"Pins live in [config/kubernetes-versions.env](config/kubernetes-versions.env); "+
			"the weekly dependency updater selects the oldest/newest supported minors and the newest patches available for both test tools. "+
			"Kubernetes client-library versions do not change this policy.", floor, pins.Minimum, pins.Latest)
	chartDocs := []replacement{
		{`(?m)^- Kubernetes [0-9.]+\+$`, "- Kubernetes " + floor + "+"},
		{`(?m)^CI covers Kubernetes .*$`, fmt.Sprintf("CI covers Kubernetes %s and %s. The chart's `kubeVersion` constraint also accepts distribution-specific version suffixes.", pins.Minimum, pins.Latest)},
	}
	rules := map[string][]replacement{
		pinsPath: {
			{`(?m)^KUBERNETES_MIN_VERSION=.*$`, "KUBERNETES_MIN_VERSION=" + pins.Minimum},
			{`(?m)^KUBERNETES_MAX_VERSION=.*$`, "KUBERNETES_MAX_VERSION=" + pins.Latest},
		},
		"charts/kafka-operator/Chart.yaml": {
			{`(?m)^kubeVersion: .*$`, `kubeVersion: ">=` + floor + `-0"`},
		},
		"charts/kafka-operator/README.md.gotmpl": chartDocs,
		"charts/kafka-operator/README.md":        chartDocs,
		"README.md": {
			{`(?m)^Koperator requires \*\*Kubernetes .*$`, supportText},
			{`(?m)^To complete this guide, you will need a Kubernetes [0-9.]+\+ cluster`, "To complete this guide, you will need a Kubernetes " + floor + "+ cluster"},
		},
		"AGENTS.md": {
			{`(?m)^- \*\*Minimum Kubernetes server version\*\*:.*$`, fmt.Sprintf("- **Minimum Kubernetes server version**: %s; CI targets: %s and %s, managed in config/kubernetes-versions.env", floor, pins.Minimum, pins.Latest)},
			{`(?m)^- ` + "`ENVTEST_K8S_VERSION`" + ` - .*$`, fmt.Sprintf("- `ENVTEST_K8S_VERSION` - Defaults to KUBERNETES_MAX_VERSION from config/kubernetes-versions.env; use `make test ENVTEST_K8S_VERSION=%s` for the lower CI boundary", pins.Minimum)},
		},
	}
	result := make(map[string][]byte, len(rules))
	for name, replacements := range rules {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			return nil, err
		}
		for _, r := range replacements {
			pattern := regexp.MustCompile(r.pattern)
			if len(pattern.FindAllIndex(data, -1)) != 1 {
				return nil, fmt.Errorf("expected one managed field matching %q in %s", r.pattern, name)
			}
			data = pattern.ReplaceAllLiteral(data, []byte(r.value))
		}
		result[name] = data
	}
	return result, nil
}

func update(root string, discover func() (targets, error)) error {
	pins, err := discover()
	if err != nil {
		return err
	}
	files, err := renderFiles(root, pins)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, files[name], 0o644); err != nil {
			return err
		}
	}
	fmt.Printf("Pinned Kubernetes test boundaries: %s and %s\n", pins.Minimum, pins.Latest)
	return nil
}

func check(root string) error {
	pins, err := readPins(root)
	if err != nil {
		return err
	}
	expected, err := renderFiles(root, pins)
	if err != nil {
		return err
	}
	for name, want := range expected {
		actual, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			return err
		}
		if string(actual) != string(want) {
			return fmt.Errorf("%s differs from managed Kubernetes pins; run make update-kubernetes-versions", name)
		}
	}
	return nil
}
