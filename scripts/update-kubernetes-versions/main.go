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
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"sigs.k8s.io/yaml"
)

const (
	scheduleURL = "https://raw.githubusercontent.com/kubernetes/website/main/data/releases/schedule.yaml"
	envtestURL  = "https://raw.githubusercontent.com/kubernetes-sigs/controller-tools/HEAD/envtest-releases.yaml"
	kindTagsURL = "https://hub.docker.com/v2/repositories/kindest/node/tags?"
)

var (
	stableVersion = regexp.MustCompile(`^v?([0-9]+)\.([0-9]+)\.([0-9]+)$`)
	platforms     = []string{"linux-amd64", "linux-arm64", "darwin-amd64", "darwin-arm64"}
)

type asset struct {
	Hash     string `json:"hash"`
	SelfLink string `json:"selfLink"`
}

type targets struct {
	Minimum string
	Latest  string
}

func numbers(value string) []int {
	match := stableVersion.FindStringSubmatch(value)
	if match == nil {
		return nil
	}
	result := make([]int, 3)
	for i := range result {
		n, err := strconv.Atoi(match[i+1])
		if err != nil {
			return nil
		}
		result[i] = n
	}
	return result
}

func compareVersion(a, b string) int {
	left, right := numbers(a), numbers(b)
	for i := range left {
		if left[i] < right[i] {
			return -1
		}
		if left[i] > right[i] {
			return 1
		}
	}
	return 0
}

func supportedMinors(data []byte, now time.Time) ([]string, error) {
	var metadata struct {
		Schedules []struct {
			Release     string `json:"release"`
			ReleaseDate string `json:"releaseDate"`
			EOL         string `json:"endOfLifeDate"`
		} `json:"schedules"`
	}
	if err := yaml.Unmarshal(data, &metadata); err != nil {
		return nil, fmt.Errorf("decode Kubernetes lifecycle metadata: %w", err)
	}
	today := now.UTC().Truncate(24 * time.Hour)
	var result []string
	for _, release := range metadata.Schedules {
		if numbers(release.Release+".0") == nil {
			continue
		}
		start, startErr := time.Parse(time.DateOnly, release.ReleaseDate)
		end, endErr := time.Parse(time.DateOnly, release.EOL)
		if startErr != nil || endErr != nil || end.Before(start) {
			return nil, fmt.Errorf("invalid lifecycle dates for Kubernetes %s", release.Release)
		}
		if !today.Before(start) && !today.After(end) {
			result = append(result, release.Release+".0")
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("no released, maintained Kubernetes minors found")
	}
	slices.SortFunc(result, compareVersion)
	result = slices.Compact(result)
	for i := range result {
		result[i] = strings.TrimSuffix(result[i], ".0")
	}
	return result, nil
}

func selectVersion(minor string, kindTags []string, releases map[string]map[string]asset) (string, error) {
	var candidates []string
	for _, tag := range kindTags {
		value := strings.TrimPrefix(tag, "v")
		n := numbers(value)
		if n == nil || fmt.Sprintf("%d.%d", n[0], n[1]) != minor {
			continue
		}
		available := true
		for _, platform := range platforms {
			a := releases["v"+value]["envtest-v"+value+"-"+platform+".tar.gz"]
			if a.Hash == "" || a.SelfLink == "" {
				available = false
				break
			}
		}
		if available {
			candidates = append(candidates, value)
		}
	}
	if len(candidates) == 0 {
		return "", fmt.Errorf("no common stable Kind/envtest assets for Kubernetes %s", minor)
	}
	slices.SortFunc(candidates, compareVersion)
	return candidates[len(candidates)-1], nil
}

type fetcher struct {
	client *http.Client
	delay  func(context.Context, time.Duration) error
}

func (f fetcher) get(ctx context.Context, address string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			if err := f.delay(ctx, time.Second<<uint(attempt-1)); err != nil {
				return nil, err
			}
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
		if err != nil {
			return nil, err
		}
		response, err := f.client.Do(request)
		if err != nil {
			lastErr = err
			continue
		}
		data, readErr := io.ReadAll(io.LimitReader(response.Body, 10<<20))
		_ = response.Body.Close()
		if response.StatusCode == http.StatusOK && readErr == nil {
			return data, nil
		}
		lastErr = fmt.Errorf("GET %s: %s (read error: %v)", address, response.Status, readErr)
		if readErr == nil && response.StatusCode != http.StatusTooManyRequests && response.StatusCode < 500 {
			return nil, lastErr
		}
	}
	return nil, fmt.Errorf("metadata retrieval exhausted retries: %w", lastErr)
}

func (f fetcher) kindTags(ctx context.Context, minor string) ([]string, error) {
	address := kindTagsURL + "page_size=100&name=" + url.QueryEscape("v"+minor+".")
	var tags []string
	for page := 0; address != "" && page < 10; page++ {
		if !strings.HasPrefix(address, kindTagsURL) {
			return nil, fmt.Errorf("unexpected Kind tags pagination URL %q", address)
		}
		data, err := f.get(ctx, address)
		if err != nil {
			return nil, err
		}
		var metadata struct {
			Next    string `json:"next"`
			Results []struct {
				Name   string `json:"name"`
				Images []struct {
					OS           string `json:"os"`
					Architecture string `json:"architecture"`
					Status       string `json:"status"`
				} `json:"images"`
			} `json:"results"`
		}
		if err := json.Unmarshal(data, &metadata); err != nil {
			return nil, err
		}
		for _, tag := range metadata.Results {
			architectures := map[string]bool{}
			for _, image := range tag.Images {
				if image.OS == "linux" && image.Status == "active" {
					architectures[image.Architecture] = true
				}
			}
			if architectures["amd64"] && architectures["arm64"] {
				tags = append(tags, tag.Name)
			}
		}
		address = metadata.Next
	}
	if address != "" {
		return nil, fmt.Errorf("kind tags pagination exceeded its limit")
	}
	return tags, nil
}

func (f fetcher) discover(ctx context.Context, now time.Time) (targets, error) {
	data, err := f.get(ctx, scheduleURL)
	if err != nil {
		return targets{}, err
	}
	minors, err := supportedMinors(data, now)
	if err != nil {
		return targets{}, err
	}
	data, err = f.get(ctx, envtestURL)
	if err != nil {
		return targets{}, err
	}
	var index struct {
		Releases map[string]map[string]asset `json:"releases"`
	}
	if err := yaml.Unmarshal(data, &index); err != nil {
		return targets{}, err
	}
	selected := make([]string, 0, 2)
	for _, minor := range []string{minors[0], minors[len(minors)-1]} {
		tags, err := f.kindTags(ctx, minor)
		if err != nil {
			return targets{}, err
		}
		value, err := selectVersion(minor, tags, index.Releases)
		if err != nil {
			return targets{}, err
		}
		selected = append(selected, value)
	}
	return targets{Minimum: selected[0], Latest: selected[1]}, nil
}

func wait(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func run(root string, updating, checking bool) error {
	switch {
	case updating && !checking:
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		f := fetcher{client: &http.Client{Timeout: 20 * time.Second}, delay: wait}
		return update(root, func() (targets, error) { return f.discover(ctx, time.Now()) })
	case checking && !updating:
		return check(root)
	default:
		return fmt.Errorf("specify exactly one of -update or -check")
	}
}

func main() {
	updateFlag := flag.Bool("update", false, "Discover supported targets and update committed pins and derived files")
	checkFlag := flag.Bool("check", false, "Check derived files against committed pins without network requests")
	root := flag.String("root", ".", "Repository root")
	flag.Parse()
	if err := run(*root, *updateFlag, *checkFlag); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
