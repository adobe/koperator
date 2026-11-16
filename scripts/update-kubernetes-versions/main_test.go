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
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestSupportedMinors(t *testing.T) {
	data := []byte(`schedules:
- {release: "1.34", releaseDate: "2025-08-27", endOfLifeDate: "2026-10-27"}
- {release: "1.37", releaseDate: "2026-08-26", endOfLifeDate: "2027-10-28"}
- {release: "1.38", releaseDate: "2026-12-01", endOfLifeDate: "2028-01-01"}
- {release: "1.35", releaseDate: "2025-12-17", endOfLifeDate: "2027-02-28"}
- {release: "1.36", releaseDate: "2026-04-22", endOfLifeDate: "2027-06-28"}
`)
	now := time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC)
	got, err := supportedMinors(data, now)
	if err != nil || !reflect.DeepEqual(got, []string{"1.35", "1.36", "1.37"}) {
		t.Fatalf("got %v, %v", got, err)
	}
	for _, invalid := range []string{"bad: [", "schedules: []", `schedules: [{release: "1.35"}]`} {
		if _, err := supportedMinors([]byte(invalid), now); err == nil {
			t.Errorf("accepted invalid lifecycle metadata %q", invalid)
		}
	}
}

func TestSelectVersionRequiresCommonStableAssets(t *testing.T) {
	releases := map[string]map[string]asset{}
	for _, version := range []string{"1.35.0", "1.35.9", "1.35.10", "1.35.11", "1.35.12-rc.1", "1.36.0"} {
		releases["v"+version] = map[string]asset{}
		for _, platform := range platforms {
			releases["v"+version]["envtest-v"+version+"-"+platform+".tar.gz"] = asset{Hash: "hash", SelfLink: "https://example.test/asset"}
		}
	}
	delete(releases["v1.35.11"], "envtest-v1.35.11-linux-amd64.tar.gz")
	tags := []string{"v1.35.9", "v1.35.10", "v1.35.11", "v1.35.12-rc.1", "v1.36.0"}
	got, err := selectVersion("1.35", tags, releases)
	if err != nil || got != "1.35.10" {
		t.Fatalf("want 1.35.10, got %q, %v", got, err)
	}
	if _, err := selectVersion("1.34", tags, releases); err == nil {
		t.Fatal("accepted a boundary minor with no common assets")
	}
}

func TestMetadataRetrievalRetriesOnlyTransientFailures(t *testing.T) {
	for _, status := range []int{http.StatusServiceUnavailable, http.StatusNotFound} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls++
				if calls < 3 {
					w.WriteHeader(status)
					return
				}
				_, _ = w.Write([]byte("metadata"))
			}))
			defer server.Close()
			f := fetcher{client: server.Client(), delay: func(context.Context, time.Duration) error { return nil }}
			data, err := f.get(context.Background(), server.URL)
			if status == http.StatusNotFound {
				if err == nil || calls != 1 {
					t.Fatalf("404: calls=%d err=%v", calls, err)
				}
			} else if err != nil || string(data) != "metadata" || calls != 3 {
				t.Fatalf("503 recovery: calls=%d data=%q err=%v", calls, data, err)
			}
		})
	}
}

func TestDiscoveryFailureLeavesPinsUntouched(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config/kubernetes-versions.env")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}

	before := []byte("KUBERNETES_MIN_VERSION=1.35.0\nKUBERNETES_MAX_VERSION=1.37.0\n")
	if err := os.WriteFile(path, before, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := update(root, func() (targets, error) { return targets{}, errors.New("metadata unavailable") }); err == nil {
		t.Fatal("ignored discovery failure")
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(before) {
		t.Fatalf("pins changed after discovery failure: %q, %v", after, err)
	}
}

func TestDerivedFilesAndOfflineDriftCheck(t *testing.T) {
	root := t.TempDir()
	fixtures := map[string]string{
		pinsPath:                                 "KUBERNETES_MIN_VERSION=1.34.0\nKUBERNETES_MAX_VERSION=1.37.0\n",
		"charts/kafka-operator/Chart.yaml":       "kubeVersion: \">=1.34.0-0\"\n",
		"charts/kafka-operator/README.md":        "- Kubernetes 1.34.0+\nCI covers Kubernetes 1.34.0 and 1.37.0.\n",
		"charts/kafka-operator/README.md.gotmpl": "- Kubernetes 1.34.0+\nCI covers Kubernetes 1.34.0 and 1.37.0.\n",
		"README.md":                              "Koperator requires **Kubernetes 1.34.0 or newer**.\nTo complete this guide, you will need a Kubernetes 1.34.0+ cluster.\n",
		"AGENTS.md":                              "- **Minimum Kubernetes server version**: 1.34.0\n- `ENVTEST_K8S_VERSION` - Kubernetes version for envtest assets\n",
	}

	for name, text := range fixtures {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	discover := func() (targets, error) { return targets{Minimum: "1.35.9", Latest: "1.37.1"}, nil }
	if err := update(root, discover); err != nil {
		t.Fatal(err)
	}
	if err := check(root); err != nil {
		t.Fatal(err)
	}
	chart := filepath.Join(root, "charts/kafka-operator/Chart.yaml")
	data, err := os.ReadFile(chart)
	if err != nil || string(data) != "kubeVersion: \">=1.35.0-0\"\n" {
		t.Fatalf("floor must follow the minor, not the test patch: %q, %v", data, err)
	}
	if err := os.WriteFile(chart, []byte(fixtures["charts/kafka-operator/Chart.yaml"]), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := check(root); err == nil {
		t.Fatal("offline check accepted drifted chart metadata")
	}
}

func TestPinsRejectPrefixedVersions(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, pinsPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("KUBERNETES_MIN_VERSION=v1.35.0\nKUBERNETES_MAX_VERSION=1.37.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readPins(root); err == nil {
		t.Fatal("accepted a v-prefixed pin that would produce an invalid vv-prefixed Kind target")
	}
}
