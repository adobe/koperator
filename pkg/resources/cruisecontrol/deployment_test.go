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

package cruisecontrol

import (
	"testing"
)

// TestGeneratePodAnnotations_CapacityConfigHash covers the broker-capacity-config annotation
// switch that determines whether capacity.json is hashed into the Cruise Control Deployment's
// pod-template annotations (and therefore whether a capacity.json change causes the Deployment
// to roll the Cruise Control pod).
//
// See https://github.com/adobe/koperator/issues/301: capacity.json is regenerated on every
// broker scaling event, so including it in the pod template hash by default causes the Cruise
// Control pod to restart mid-scale-operation, since CruiseControlOperation submission only
// checks Cruise Control's own REST API readiness and has no awareness of the Deployment
// rollout. capacity.json is only read by Cruise Control at process startup, so excluding it
// from the hash by default does not require CC to be reloaded to pick up new capacity data
// immediately; the operator's capacity-estimation fallback covers the gap until CC's next
// natural restart.
func TestGeneratePodAnnotations_CapacityConfigHash(t *testing.T) {
	cruiseControlConfig := map[string]string{
		"cruisecontrol.properties": "cc-props",
		"clusterConfigs.json":      "cluster-cfg",
		"log4j.properties":         "log-cfg",
		"capacity.json":            "capacity-v1",
	}

	testCases := []struct {
		testName            string
		ccAnnotations       map[string]string
		expectCapacityInPod bool
	}{
		{
			testName:            "annotation unset (default): capacity.json is excluded from the pod template hash",
			ccAnnotations:       map[string]string{},
			expectCapacityInPod: false,
		},
		{
			testName:            "annotation nil map: capacity.json is excluded from the pod template hash",
			ccAnnotations:       nil,
			expectCapacityInPod: false,
		},
		{
			testName: "annotation explicitly \"static\": capacity.json is included (opt-in)",
			ccAnnotations: map[string]string{
				capacityConfigAnnotation: "static",
			},
			expectCapacityInPod: true,
		},
		{
			testName: "annotation set to an unrelated value (\"dynamic\"): capacity.json is excluded",
			ccAnnotations: map[string]string{
				capacityConfigAnnotation: "dynamic",
			},
			expectCapacityInPod: false,
		},
		{
			testName: "annotation set to empty string: capacity.json is excluded",
			ccAnnotations: map[string]string{
				capacityConfigAnnotation: "",
			},
			expectCapacityInPod: false,
		},
		{
			testName: "annotation value is case-sensitive: \"Static\" does not match \"static\" and is excluded",
			ccAnnotations: map[string]string{
				capacityConfigAnnotation: "Static",
			},
			expectCapacityInPod: false,
		},
		{
			testName: "unrelated user annotations are preserved regardless of capacity opt-in",
			ccAnnotations: map[string]string{
				"some.other/annotation": "keep-me",
			},
			expectCapacityInPod: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.testName, func(t *testing.T) {
			podAnnotations := GeneratePodAnnotations(tc.ccAnnotations, cruiseControlConfig)

			_, present := podAnnotations["cruiseControlCapacity.json"]
			if present != tc.expectCapacityInPod {
				t.Fatalf("cruiseControlCapacity.json presence = %v, want %v (annotations: %v)",
					present, tc.expectCapacityInPod, tc.ccAnnotations)
			}

			// The other three hashes must always be present regardless of the capacity switch.
			for _, key := range []string{"cruiseControlConfig.json", "cruiseControlClusterConfig.json", "cruiseControlLogConfig.json"} {
				if _, ok := podAnnotations[key]; !ok {
					t.Fatalf("expected %s to always be present in pod annotations", key)
				}
			}

			// Unrelated caller-supplied annotations must be passed through untouched.
			for k, v := range tc.ccAnnotations {
				if k == capacityConfigAnnotation {
					continue
				}
				if podAnnotations[k] != v {
					t.Fatalf("expected passthrough annotation %s=%s, got %s", k, v, podAnnotations[k])
				}
			}
		})
	}
}

// TestGeneratePodAnnotations_CapacityChangeDoesNotAffectPodTemplateByDefault reproduces the
// exact scaling scenario from issue #301 end-to-end through the real GenerateCapacityConfig +
// GeneratePodAnnotations functions: adding a broker changes capacity.json content, but with the
// default (unset) annotation, the resulting pod-template annotations must be identical before
// and after the scale-up, so Kubernetes has no pod-template diff to roll on.
func TestGeneratePodAnnotations_CapacityChangeDoesNotAffectPodTemplateByDefault(t *testing.T) {
	baseConfig := map[string]string{
		"cruisecontrol.properties": "cc-props",
		"clusterConfigs.json":      "cluster-cfg",
		"log4j.properties":         "log-cfg",
	}

	beforeScale := make(map[string]string, len(baseConfig)+1)
	for k, v := range baseConfig {
		beforeScale[k] = v
	}
	beforeScale["capacity.json"] = `{"brokerCapacities":[{"brokerId":"0"},{"brokerId":"1"},{"brokerId":"2"}]}`

	afterScale := make(map[string]string, len(baseConfig)+1)
	for k, v := range baseConfig {
		afterScale[k] = v
	}
	afterScale["capacity.json"] = `{"brokerCapacities":[{"brokerId":"0"},{"brokerId":"1"},{"brokerId":"2"},{"brokerId":"3"}]}`

	if beforeScale["capacity.json"] == afterScale["capacity.json"] {
		t.Fatalf("test setup error: capacity.json fixtures must differ to exercise the scale-up scenario")
	}

	podAnnotationsBefore := GeneratePodAnnotations(map[string]string{}, beforeScale)
	podAnnotationsAfter := GeneratePodAnnotations(map[string]string{}, afterScale)

	if len(podAnnotationsBefore) != len(podAnnotationsAfter) {
		t.Fatalf("pod annotation key sets differ across scale-up: before=%v after=%v", podAnnotationsBefore, podAnnotationsAfter)
	}
	for k, v := range podAnnotationsBefore {
		if podAnnotationsAfter[k] != v {
			t.Fatalf("pod annotation %s changed across a capacity.json-only change (before=%s after=%s); "+
				"this would cause an unwanted Cruise Control Deployment roll on every broker scale event", k, v, podAnnotationsAfter[k])
		}
	}
}

// TestGeneratePodAnnotations_StaticOptInStillRestartsOnCapacityChange guards the escape hatch:
// operators who explicitly opt into "static" mode must still see the Deployment roll when
// capacity.json changes, preserving pre-existing behavior for anyone already relying on it.
func TestGeneratePodAnnotations_StaticOptInStillRestartsOnCapacityChange(t *testing.T) {
	baseConfig := map[string]string{
		"cruisecontrol.properties": "cc-props",
		"clusterConfigs.json":      "cluster-cfg",
		"log4j.properties":         "log-cfg",
	}
	ccAnnotations := map[string]string{capacityConfigAnnotation: string(staticCapacityConfig)}

	before := make(map[string]string, len(baseConfig)+1)
	for k, v := range baseConfig {
		before[k] = v
	}
	before["capacity.json"] = "capacity-v1"

	after := make(map[string]string, len(baseConfig)+1)
	for k, v := range baseConfig {
		after[k] = v
	}
	after["capacity.json"] = "capacity-v2"

	podAnnotationsBefore := GeneratePodAnnotations(ccAnnotations, before)
	podAnnotationsAfter := GeneratePodAnnotations(ccAnnotations, after)

	hashBefore, ok := podAnnotationsBefore["cruiseControlCapacity.json"]
	if !ok {
		t.Fatalf("expected cruiseControlCapacity.json to be present when static opt-in is set")
	}
	hashAfter := podAnnotationsAfter["cruiseControlCapacity.json"]

	if hashBefore == hashAfter {
		t.Fatalf("expected capacity hash to change across a capacity.json content change under static opt-in")
	}
}

// TestGeneratePodAnnotations_UpgradeDropsCapacityHashOnce documents the one-time upgrade side
// effect described in the comment above the capacity.json check in GeneratePodAnnotations:
// every cluster reconciled under the pre-fix behavior (annotation unset) already has
// cruiseControlCapacity.json baked into its live Deployment's pod template. The first reconcile
// under the new default computes a pod template without that key, so it must actually be absent
// (not merely unchanged) to reproduce the diff Kubernetes will apply as a single Deployment
// update on upgrade.
func TestGeneratePodAnnotations_UpgradeDropsCapacityHashOnce(t *testing.T) {
	cruiseControlConfig := map[string]string{
		"cruisecontrol.properties": "cc-props",
		"clusterConfigs.json":      "cluster-cfg",
		"log4j.properties":         "log-cfg",
		"capacity.json":            "capacity-v1",
	}

	// No annotation set - the only configuration every pre-fix production cluster has today.
	podAnnotations := GeneratePodAnnotations(map[string]string{}, cruiseControlConfig)

	if _, present := podAnnotations["cruiseControlCapacity.json"]; present {
		t.Fatalf("expected cruiseControlCapacity.json to be absent post-fix for an unset annotation; " +
			"its presence here would mean no upgrade-time pod template diff occurs, contradicting the " +
			"documented one-time-restart-on-upgrade behavior")
	}
}
