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
	"fmt"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/gruntwork-io/terratest/modules/k8s"
	ginkgo "github.com/onsi/ginkgo/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// kclImage is the upstream kcl release image. Its base is distroless/static:nonroot, so the
// container has NO shell: every invocation must exec kcl directly (the image's entrypoint) and
// nothing can be wrapped in "/bin/sh -c". This is also why kcl runs as a short-lived pod per
// operation instead of a long-lived pod that is exec'd into - there is no sleep/tail binary to hold
// such a pod open. See https://github.com/twmb/kcl#getting-started, which documents exactly this
// "kubectl run" usage for the image.
func kclImage() string {
	return fmt.Sprintf("%s:%s", kclImageRepository, KclVersion)
}

// kclTLSOptions returns the kcl -X config options that enable mTLS against the Kafka cluster using
// the certificates mounted from the test's TLS secret.
//
// kcl turns TLS on as soon as any tls.* key is set, so there is no separate security.protocol
// switch to flip (kcat needed -X security.protocol=SSL).
func kclTLSOptions() []string {
	return []string{
		"-X", fmt.Sprintf("tls.ca_cert_path=%s/ca.crt", kclTLSMountPath),
		"-X", fmt.Sprintf("tls.client_cert_path=%s/tls.crt", kclTLSMountPath),
		"-X", fmt.Sprintf("tls.client_key_path=%s/tls.key", kclTLSMountPath),
	}
}

// kclPodOverrides builds the JSON pod-spec override for a one-shot kcl pod.
//
// The override carries the whole container (not just the extra bits) on purpose: kubectl applies
// --overrides as a JSON merge patch, which replaces the containers list wholesale rather than
// merging into it, so anything omitted here - args included - would be dropped from the pod kubectl
// generates.
//
// withStdin must be set for "kcl produce", which reads its records from stdin (kcl has no flag to
// pass a record value on the command line); it is deliberately left off for consuming so that path
// does not depend on an attached stdin stream at all.
func kclPodOverrides(args []string, tlsSecretName string, withStdin bool) (string, error) {
	container := corev1.Container{
		Name:  kclName,
		Image: kclImage(),
		Args:  args,
	}
	if withStdin {
		container.Stdin = true
		container.StdinOnce = true
	}

	podSpec := corev1.PodSpec{
		RestartPolicy: corev1.RestartPolicyNever,
	}

	if tlsSecretName != "" {
		container.VolumeMounts = []corev1.VolumeMount{{
			Name:      kclTLSVolumeName,
			MountPath: kclTLSMountPath,
		}}
		podSpec.Volumes = []corev1.Volume{{
			Name: kclTLSVolumeName,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{SecretName: tlsSecretName},
			},
		}}
	}

	podSpec.Containers = []corev1.Container{container}

	// kubectl requires the override document to carry a valid apiVersion.
	overrides := map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata":   metav1.ObjectMeta{},
		"spec":       podSpec,
	}

	rawOverrides, err := json.Marshal(overrides)
	if err != nil {
		return "", err
	}
	return string(rawOverrides), nil
}

// kclRunArgs assembles the full kubectl argument list that starts a single kcl command in a
// short-lived pod.
//
// Every argument is placed explicitly rather than appended by the caller: kubectl run forwards
// anything after a bare "--" to the container, so a trailing --namespace/--context would silently
// become a kcl argument. The kcl arguments travel inside --overrides instead of after "--" for the
// same reason.
//
// attach controls whether kubectl stays connected to the pod. It is only used for producing, where
// an attached stdin is the sole way to hand kcl a record. Consuming deliberately does NOT attach:
// `kubectl run --attach` races between streaming the attach session and dumping the container log
// once it exits, which duplicates the output roughly two runs in three - harmless for producing,
// where stdout is discarded, but corrupting for consuming, whose stdout IS the result.
func kclRunArgs(kubectlOptions k8s.KubectlOptions, podName, overrides string, attach bool) []string {
	args := []string{"run", podName}

	if kubectlOptions.ContextName != "" {
		args = append(args, "--context", kubectlOptions.ContextName)
	}
	if kubectlOptions.Namespace != "" {
		args = append(args, "--namespace", kubectlOptions.Namespace)
	}

	args = append(args,
		"--image", kclImage(),
		"--restart", "Never",
		// Without --quiet, kubectl prints its own notices (for example --rm's `pod "..." deleted`)
		// to STDOUT, where they would be mixed into the records a pod returns to the caller.
		"--quiet",
		"--pod-running-timeout", kclPodRunningTimeout.String(),
		"--overrides", overrides,
	)

	if attach {
		// --rm needs the client to stay attached to be able to clean the pod up afterwards.
		args = append(args, "--rm", "--stdin")
	}

	return args
}

// runKclProduce runs a producing kcl command in a one-shot pod, piping the record into it through
// an attached stdin (kcl produce reads records from stdin; it has no flag to pass a value on the
// command line). Its stdout is not a result, so attaching is safe here.
func runKclProduce(kubectlOptions k8s.KubectlOptions, podName, stdin, tlsSecretName string, kclArgs ...string) error {
	overrides, err := kclPodOverrides(withGlobalKclArgs(kclArgs), tlsSecretName, true)
	if err != nil {
		return err
	}

	_, err = runKubectlWithStdin(kubectlOptions, stdin, kclRunArgs(kubectlOptions, podName, overrides, true)...)

	return err
}

// runKclConsume runs a consuming kcl command in a one-shot pod and returns exactly what kcl wrote to
// stdout.
//
// The pod is started detached and its output is read back from the container log once it has
// finished, rather than streamed over an attach session, because `kubectl run --attach` racily
// emits the output twice (see kclRunArgs). Reading the log after completion yields each record
// exactly once. The pod is then deleted explicitly, since --rm only works for an attached client.
func runKclConsume(kubectlOptions k8s.KubectlOptions, podName, tlsSecretName string, kclArgs ...string) (string, error) {
	overrides, err := kclPodOverrides(withGlobalKclArgs(kclArgs), tlsSecretName, false)
	if err != nil {
		return "", err
	}

	if _, err := runKubectlWithStdin(kubectlOptions, "", kclRunArgs(kubectlOptions, podName, overrides, false)...); err != nil {
		return "", err
	}
	defer func() {
		// Best effort: a leaked pod cannot break a later run, as every pod name is unique.
		_ = deleteK8sResourceNoErrNotFound(kubectlOptions, kclPodDeletionTimeout, podsResource, podName)
	}()

	// kcl exits on its own once it has the records it was asked for or its --timeout elapses, so the
	// pod reaching a terminal phase is the signal its output is complete.
	phase, err := waitKclPodTerminated(kubectlOptions, podName)

	// Read the log either way: on failure kcl's own message ("unable to dial ...") is far more
	// diagnostic than a bare wait timeout.
	consumedMessages, logsErr := runKubectlWithStdin(kubectlOptions, "", kclLogsArgs(kubectlOptions, podName)...)

	if err != nil {
		if logsErr != nil {
			return "", err
		}
		return "", fmt.Errorf("%w: kcl output was: %s", err, consumedMessages)
	}
	if phase != string(corev1.PodSucceeded) {
		return "", fmt.Errorf("kcl pod %s terminated in phase %s: %s", podName, phase, consumedMessages)
	}

	return consumedMessages, logsErr
}

// waitKclPodTerminated blocks until the one-shot kcl pod reaches a terminal phase and reports that
// phase.
//
// It polls rather than using `kubectl wait --for=jsonpath={.status.phase}=Succeeded`, which can only
// watch for one value: a pod that fails (an unreachable broker, say) never becomes Succeeded, so the
// wait would burn the entire timeout before reporting a failure that was already decided in seconds.
func waitKclPodTerminated(kubectlOptions k8s.KubectlOptions, podName string) (string, error) {
	deadline := time.Now().Add(kclPodCompletionTimeout)

	var phase string
	for time.Now().Before(deadline) {
		// A transient error here (the pod is not registered yet) is retried until the deadline.
		rawPhase, err := runKubectlWithStdin(kubectlOptions, "", kclPodPhaseArgs(kubectlOptions, podName)...)
		if err == nil {
			phase = strings.TrimSpace(rawPhase)
			if phase == string(corev1.PodSucceeded) || phase == string(corev1.PodFailed) {
				return phase, nil
			}
		}
		time.Sleep(kclPodPhasePollInterval)
	}

	return phase, fmt.Errorf("timed out after %s waiting for kcl pod %s to finish, last phase was %q",
		kclPodCompletionTimeout, podName, phase)
}

// kclPodPhaseArgs builds the kubectl invocation that reads a kcl pod's current phase.
func kclPodPhaseArgs(kubectlOptions k8s.KubectlOptions, podName string) []string {
	args := []string{"get", podsResource, podName, "-o", "jsonpath={.status.phase}"}

	if kubectlOptions.ContextName != "" {
		args = append(args, "--context", kubectlOptions.ContextName)
	}
	if kubectlOptions.Namespace != "" {
		args = append(args, "--namespace", kubectlOptions.Namespace)
	}

	return args
}

// kclLogsArgs builds the kubectl invocation that reads a finished kcl pod's output back.
func kclLogsArgs(kubectlOptions k8s.KubectlOptions, podName string) []string {
	args := []string{"logs", podName}

	if kubectlOptions.ContextName != "" {
		args = append(args, "--context", kubectlOptions.ContextName)
	}
	if kubectlOptions.Namespace != "" {
		args = append(args, "--namespace", kubectlOptions.Namespace)
	}

	return args
}

// withGlobalKclArgs prefixes the kcl global flags every invocation needs. The image has no config
// file (and no home directory to hold one), so kcl is told not to look for one.
func withGlobalKclArgs(kclArgs []string) []string {
	return append([]string{"--no-config-file"}, kclArgs...)
}

// uniqueKclPodName gives each one-shot pod its own name so a leftover pod from an interrupted run
// (--rm cannot clean up if the test process is killed) cannot make the next run fail with
// AlreadyExists, and so parallel specs never collide.
func uniqueKclPodName(role string) string {
	return fmt.Sprintf("%s-%s-%d", kclName, role, rand.IntN(1_000_000)) //nolint:gosec // Note: pod name uniqueness only, not security relevant.
}

// consumingMessagesInternally consumes messages based on parameters from the Kafka cluster.
// It returns the consumed messages in a single string.
func consumingMessagesInternally(kubectlOptions k8s.KubectlOptions, internalKafkaAddress string, topicName string, tlsSecretName string) (string, error) {
	ginkgo.By(fmt.Sprintf("Consuming messages from internalKafkaAddress: '%s' topicName: '%s'", internalKafkaAddress, topicName))

	args := []string{"-B", internalKafkaAddress}
	if tlsSecretName != "" {
		args = append(args, kclTLSOptions()...)
	}

	// --timeout bounds the wait for a record so a missing message fails the assertion with an empty
	// result instead of hanging the suite; -o start replays the topic from its beginning, which is
	// what makes an already-produced record observable here.
	args = append(args, "consume", topicName,
		"-o", "start",
		"--num", "1",
		"--timeout", kclConsumeTimeout.String(),
	)

	return runKclConsume(kubectlOptions, uniqueKclPodName("consumer"), tlsSecretName, args...)
}

// producingMessagesInternally produces messages based on the parameters into the Kafka cluster.
func producingMessagesInternally(kubectlOptions k8s.KubectlOptions, internalKafkaAddress string, topicName string, message string, tlsSecretName string) error {
	ginkgo.By(fmt.Sprintf("Producing messages: '%s' to internalKafkaAddress: '%s' topicName: '%s'", message, internalKafkaAddress, topicName))

	args := []string{"-B", internalKafkaAddress}
	if tlsSecretName != "" {
		args = append(args, kclTLSOptions()...)
	}
	args = append(args, "produce", topicName)

	// The record is piped in as stdin rather than interpolated into a shell command, so a message
	// containing spaces or shell metacharacters (the tests produce a timestamp) cannot be mangled.
	return runKclProduce(kubectlOptions, uniqueKclPodName("producer"), message+"\n", tlsSecretName, args...)
}
