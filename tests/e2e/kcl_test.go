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

package e2e

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/gruntwork-io/terratest/modules/k8s"
	corev1 "k8s.io/api/core/v1"
)

// TestKclPodOverridesCarriesWholeContainer guards the reason the override contains a full container
// spec: kubectl applies --overrides as a JSON merge patch, so the containers list is replaced rather
// than merged. Anything dropped from here silently disappears from the pod kubectl runs - most
// damagingly the args, which would leave the pod running kcl with no command at all.
func TestKclPodOverridesCarriesWholeContainer(t *testing.T) {
	rawOverrides, err := kclPodOverrides([]string{"--no-config-file", "-B", "kafka:29092", "produce", "topic"}, "", true)
	if err != nil {
		t.Fatal("unexpected error:", err)
	}

	var pod corev1.Pod
	if err := json.Unmarshal([]byte(rawOverrides), &pod); err != nil {
		t.Fatal("overrides must be a valid pod document:", err)
	}

	if len(pod.Spec.Containers) != 1 {
		t.Fatalf("expected exactly 1 container, got %d", len(pod.Spec.Containers))
	}
	container := pod.Spec.Containers[0]

	if container.Image != kclImage() {
		t.Errorf("expected image %q, got %q", kclImage(), container.Image)
	}
	if !slices.Equal(container.Args, []string{"--no-config-file", "-B", "kafka:29092", "produce", "topic"}) {
		t.Errorf("args must survive into the override, got %v", container.Args)
	}
	// The pod must not outlive the single command it exists to run.
	if pod.Spec.RestartPolicy != corev1.RestartPolicyNever {
		t.Errorf("expected restartPolicy Never, got %q", pod.Spec.RestartPolicy)
	}
	// kcl produce reads its records from stdin, so the container has to accept one.
	if !container.Stdin || !container.StdinOnce {
		t.Errorf("expected stdin and stdinOnce for a producing pod, got stdin=%v stdinOnce=%v", container.Stdin, container.StdinOnce)
	}
}

// TestKclPodOverridesMountsTLSSecret asserts the certificates kcl is pointed at by kclTLSOptions are
// actually mounted where those -X paths expect them; a mismatch would only surface as a confusing
// "unable to read CA file" deep inside an e2e run.
func TestKclPodOverridesMountsTLSSecret(t *testing.T) {
	rawOverrides, err := kclPodOverrides([]string{"consume", "topic"}, defaultTLSSecretName, false)
	if err != nil {
		t.Fatal("unexpected error:", err)
	}

	var pod corev1.Pod
	if err := json.Unmarshal([]byte(rawOverrides), &pod); err != nil {
		t.Fatal("overrides must be a valid pod document:", err)
	}

	if len(pod.Spec.Volumes) != 1 || pod.Spec.Volumes[0].Secret == nil {
		t.Fatalf("expected exactly 1 secret volume, got %+v", pod.Spec.Volumes)
	}
	if pod.Spec.Volumes[0].Secret.SecretName != defaultTLSSecretName {
		t.Errorf("expected secret %q, got %q", defaultTLSSecretName, pod.Spec.Volumes[0].Secret.SecretName)
	}

	mounts := pod.Spec.Containers[0].VolumeMounts
	if len(mounts) != 1 || mounts[0].MountPath != kclTLSMountPath {
		t.Fatalf("expected the secret mounted at %q, got %+v", kclTLSMountPath, mounts)
	}
	if mounts[0].Name != pod.Spec.Volumes[0].Name {
		t.Errorf("volume mount %q does not reference volume %q", mounts[0].Name, pod.Spec.Volumes[0].Name)
	}

	// Every path kcl is told to read must live under the mount.
	for _, opt := range kclTLSOptions() {
		if strings.HasPrefix(opt, "tls.") && !strings.Contains(opt, "="+kclTLSMountPath+"/") {
			t.Errorf("TLS option %q points outside the mounted secret at %q", opt, kclTLSMountPath)
		}
	}

	// Consuming needs no stdin; it must not ask for a stream it never writes to.
	if pod.Spec.Containers[0].Stdin {
		t.Error("expected no stdin for a consuming pod")
	}
}

// TestKclPodOverridesOmitsTLSWhenUnset keeps the plaintext path free of an empty secret reference,
// which the API server would reject.
func TestKclPodOverridesOmitsTLSWhenUnset(t *testing.T) {
	rawOverrides, err := kclPodOverrides([]string{"consume", "topic"}, "", false)
	if err != nil {
		t.Fatal("unexpected error:", err)
	}

	var pod corev1.Pod
	if err := json.Unmarshal([]byte(rawOverrides), &pod); err != nil {
		t.Fatal("overrides must be a valid pod document:", err)
	}
	if len(pod.Spec.Volumes) != 0 {
		t.Errorf("expected no volumes without a TLS secret, got %+v", pod.Spec.Volumes)
	}
	if len(pod.Spec.Containers[0].VolumeMounts) != 0 {
		t.Errorf("expected no volume mounts without a TLS secret, got %+v", pod.Spec.Containers[0].VolumeMounts)
	}
}

// TestKclRunArgsKeepArgumentsAwayFromTheContainer pins the ordering hazard of kubectl run: anything
// after a bare "--" is handed to the container rather than to kubectl.
//
// It also pins the attach asymmetry. `kubectl run --attach` races between streaming the attach
// session and dumping the container log after it exits, duplicating the output about two runs in
// three - measured against a live cluster. Producing tolerates that (its stdout is discarded) and
// needs the attach for stdin; consuming must not attach at all, because its stdout is the result.
func TestKclRunArgsKeepArgumentsAwayFromTheContainer(t *testing.T) {
	kubectlOptions := k8s.KubectlOptions{ContextName: "kind-kind", Namespace: "kafka"}

	for _, test := range []struct {
		name   string
		attach bool
	}{
		{name: "producing attaches for stdin", attach: true},
		{name: "consuming stays detached", attach: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			args := kclRunArgs(kubectlOptions, "kcl-producer-1", `{"apiVersion":"v1"}`, test.attach)

			if slices.Contains(args, "--") {
				t.Errorf("a bare -- would forward the remaining flags to the container, got %v", args)
			}

			// --rm requires an attached client, so the two must appear together or not at all.
			hasStdin := slices.Contains(args, "--stdin")
			hasRm := slices.Contains(args, "--rm")
			if hasStdin != test.attach || hasRm != test.attach {
				t.Errorf("expected attach=%v to imply --stdin and --rm, got --stdin=%v --rm=%v", test.attach, hasStdin, hasRm)
			}
			if slices.Contains(args, "--attach") {
				t.Error("--attach duplicates the pod output; stdin-attach is only acceptable where stdout is discarded")
			}

			// --quiet keeps kubectl's own notices (such as --rm's `pod "..." deleted`) out of
			// stdout, which is the same stream the consumed records are read from.
			if !slices.Contains(args, "--quiet") {
				t.Errorf("expected --quiet so kubectl cannot pollute stdout, got %v", args)
			}

			// The namespace and context must reach kubectl itself.
			for _, want := range []string{"kafka", "kind-kind", "--overrides"} {
				if !slices.Contains(args, want) {
					t.Errorf("expected %q in the kubectl args, got %v", want, args)
				}
			}
			if args[0] != "run" || args[1] != "kcl-producer-1" {
				t.Errorf("expected 'run <podName>' to lead the args, got %v", args[:2])
			}
		})
	}
}

// TestKclLogsArgsTargetTheRightPod keeps the log read - which is how a consuming pod's records come
// back - pointed at the same namespace and context the pod was created in.
func TestKclLogsArgsTargetTheRightPod(t *testing.T) {
	args := kclLogsArgs(k8s.KubectlOptions{ContextName: "kind-kind", Namespace: "kafka"}, "kcl-consumer-1")

	want := []string{"logs", "kcl-consumer-1", "--context", "kind-kind", "--namespace", "kafka"}
	if !slices.Equal(args, want) {
		t.Errorf("expected %v, got %v", want, args)
	}
}

// TestKclConsumeOutlastsItsOwnTimeout guards an ordering dependency between two constants: the pod
// is waited on until it completes, and kcl only completes once its own --timeout has elapsed. If the
// wait were the shorter of the two, every consume with no records would fail as a wait timeout
// instead of as an empty result.
func TestKclConsumeOutlastsItsOwnTimeout(t *testing.T) {
	if kclPodCompletionTimeout <= kclConsumeTimeout {
		t.Errorf("kclPodCompletionTimeout (%s) must exceed kclConsumeTimeout (%s)", kclPodCompletionTimeout, kclConsumeTimeout)
	}
}

// TestUniqueKclPodNames keeps a leftover pod from an interrupted run (where --rm never got to clean
// up) from failing the next run with AlreadyExists.
func TestUniqueKclPodNames(t *testing.T) {
	seen := make(map[string]struct{})
	for range 100 {
		name := uniqueKclPodName("producer")
		if !strings.HasPrefix(name, kclName+"-producer-") {
			t.Fatalf("unexpected pod name shape: %q", name)
		}
		seen[name] = struct{}{}
	}
	if len(seen) < 90 {
		t.Errorf("pod names are not sufficiently unique: %d distinct out of 100", len(seen))
	}
}
