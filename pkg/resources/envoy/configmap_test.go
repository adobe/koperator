// Copyright 2026 Cisco Systems, Inc. and/or its affiliates
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

package envoy

import (
	"strings"
	"testing"

	envoybootstrap "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	envoyfileaccesslog "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/file/v3"
	envoytypes "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/go-logr/logr"
	"google.golang.org/protobuf/encoding/protojson"
	"sigs.k8s.io/yaml"

	"github.com/banzaicloud/koperator/api/v1beta1"
	"github.com/banzaicloud/koperator/pkg/util"
)

func TestNewRuntimeSampledAccessLog(t *testing.T) {
	t.Parallel()

	accessLog, err := newRuntimeSampledAccessLog("broker_101", AccessLogRuntimeKeyBroker)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if accessLog.GetName() != wellknown.FileAccessLog {
		t.Errorf("name = %q, want %q", accessLog.GetName(), wellknown.FileAccessLog)
	}

	runtimeFilter := accessLog.GetFilter().GetRuntimeFilter()
	if runtimeFilter == nil {
		t.Fatal("filter is not a runtime filter")
	}

	if runtimeFilter.GetRuntimeKey() != AccessLogRuntimeKeyBroker {
		t.Errorf("runtimeKey = %q, want %q", runtimeFilter.GetRuntimeKey(), AccessLogRuntimeKeyBroker)
	}

	if !runtimeFilter.GetUseIndependentRandomness() {
		t.Error("useIndependentRandomness = false, want true")
	}

	percentSampled := runtimeFilter.GetPercentSampled()
	if percentSampled == nil {
		t.Fatal("percentSampled is nil, want a denominator-only fraction")
	}

	if percentSampled.GetDenominator() != envoytypes.FractionalPercent_TEN_THOUSAND {
		t.Errorf("denominator = %v, want TEN_THOUSAND", percentSampled.GetDenominator())
	}

	// The numerator must never be set. protojson omits zero scalars, so an
	// unset numerator is what makes the rendered sampler inert - the knob only
	// starts logging once an operator raises it at runtime.
	if percentSampled.Numerator != 0 {
		t.Errorf("numerator = %d, want 0 (unset)", percentSampled.Numerator)
	}
}

func TestNewRuntimeSampledAccessLogFormat(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		testName   string
		chainName  string
		runtimeKey string
	}{
		{testName: "per-broker chain", chainName: "broker_101", runtimeKey: AccessLogRuntimeKeyBroker},
		{testName: "anycast chain", chainName: "anycast", runtimeKey: AccessLogRuntimeKeyAnycast},
		{testName: "catch-all chain", chainName: "catchall_unmatched", runtimeKey: AccessLogRuntimeKeyUnmatched},
	}

	for _, testCase := range testCases {
		t.Run(testCase.testName, func(t *testing.T) {
			t.Parallel()

			accessLog, err := newRuntimeSampledAccessLog(testCase.chainName, testCase.runtimeKey)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			fileAccessLog := &envoyfileaccesslog.FileAccessLog{}
			if err := accessLog.GetTypedConfig().UnmarshalTo(fileAccessLog); err != nil {
				t.Fatalf("typedConfig is not a FileAccessLog: %v", err)
			}

			if fileAccessLog.GetPath() != "/dev/stdout" {
				t.Errorf("path = %q, want /dev/stdout", fileAccessLog.GetPath())
			}

			format := fileAccessLog.GetLogFormat().GetTextFormatSource().GetInlineString()

			if !strings.Contains(format, "chain="+testCase.chainName+" ") {
				t.Errorf("format does not identify the chain: %q", format)
			}

			// Envoy does not append one, so without it log lines would run together.
			if !strings.HasSuffix(format, "\n") {
				t.Errorf("format does not end with a newline: %q", format)
			}
		})
	}
}

// TestNewRuntimeSampledAccessLogRendersAsValidated is the important one: it
// pins the helper's output against bytes that were validated against a live
// Envoy (v1.36.2 and v1.40.2) and toggled through the admin API on a dev
// cluster, rather than against expectations derived from this implementation.
//
// The expected block is lifted verbatim from the per-broker chain of
// work/envoy configs/envoy-dev-az1-cm.debug.yaml, and is rendered through the
// same protojson -> YAML path GenerateEnvoyConfig uses, so a mismatch here
// means the reconciler would emit something other than what was proven to work.
func TestNewRuntimeSampledAccessLogRendersAsValidated(t *testing.T) {
	t.Parallel()

	const want = `filter:
  runtimeFilter:
    percentSampled:
      denominator: TEN_THOUSAND
    runtimeKey: access_log.broker_sample_rate
    useIndependentRandomness: true
name: envoy.access_loggers.file
typedConfig:
  '@type': type.googleapis.com/envoy.extensions.access_loggers.file.v3.FileAccessLog
  logFormat:
    textFormatSource:
      inlineString: |
        [%START_TIME%] chain=broker_101 src=%DOWNSTREAM_REMOTE_ADDRESS% local=%DOWNSTREAM_LOCAL_ADDRESS% sni="%REQUESTED_SERVER_NAME%" bytes_recv=%BYTES_RECEIVED% bytes_sent=%BYTES_SENT% duration=%DURATION% flags=%RESPONSE_FLAGS%
  path: /dev/stdout
`

	accessLog, err := newRuntimeSampledAccessLog("broker_101", AccessLogRuntimeKeyBroker)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	marshalledJSON, err := (&protojson.MarshalOptions{}).Marshal(accessLog)
	if err != nil {
		t.Fatalf("could not marshal access log: %v", err)
	}

	got, err := yaml.JSONToYAML(marshalledJSON)
	if err != nil {
		t.Fatalf("could not convert access log to YAML: %v", err)
	}

	if string(got) != want {
		t.Errorf("rendered access log does not match the validated config\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestNewAccessLogRuntimeLayer(t *testing.T) {
	t.Parallel()

	layeredRuntime := newAccessLogRuntimeLayer()

	layers := layeredRuntime.GetLayers()
	if len(layers) != 1 {
		t.Fatalf("layers = %d, want exactly 1", len(layers))
	}

	if layers[0].GetName() != "admin" {
		t.Errorf("layer name = %q, want admin", layers[0].GetName())
	}

	// Without an admin layer specifically, /runtime_modify has nowhere to write
	// and the sampling keys can never be raised.
	if layers[0].GetAdminLayer() == nil {
		t.Error("layer is not an admin layer, so runtime_modify would have nowhere to write")
	}
}

// TestNewAccessLogRuntimeLayerRendersAsValidated pins the bootstrap block
// against work/envoy configs/envoy-dev-az1-cm.debug.yaml, the config that was
// booted on a live Envoy and whose knobs were toggled through the admin API.
func TestNewAccessLogRuntimeLayerRendersAsValidated(t *testing.T) {
	t.Parallel()

	const want = `layeredRuntime:
  layers:
  - adminLayer: {}
    name: admin
`

	marshalledJSON, err := (&protojson.MarshalOptions{}).Marshal(&envoybootstrap.Bootstrap{
		LayeredRuntime: newAccessLogRuntimeLayer(),
	})
	if err != nil {
		t.Fatalf("could not marshal bootstrap: %v", err)
	}

	got, err := yaml.JSONToYAML(marshalledJSON)
	if err != nil {
		t.Fatalf("could not convert bootstrap to YAML: %v", err)
	}

	if string(got) != want {
		t.Errorf("rendered runtime layer does not match the validated config\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// testKafkaCluster is the smallest cluster that renders a complete Envoy
// config: one broker, one external listener, envoy as the ingress controller.
func testKafkaCluster() (*v1beta1.KafkaCluster, v1beta1.ExternalListenerConfig) {
	kafkaCluster := &v1beta1.KafkaCluster{
		Spec: v1beta1.KafkaClusterSpec{
			IngressController: "envoy",
			Brokers:           []v1beta1.Broker{{Id: 101}},
			ListenersConfig: v1beta1.ListenersConfig{
				ExternalListeners: []v1beta1.ExternalListenerConfig{{
					CommonListenerSpec: v1beta1.CommonListenerSpec{
						Name:          "external",
						ContainerPort: 9094,
					},
					ExternalStartingPort: 19090,
				}},
			},
		},
	}

	return kafkaCluster, kafkaCluster.Spec.ListenersConfig.ExternalListeners[0]
}

func renderEnvoyConfig(t *testing.T, enableAccesslogSampling bool) string {
	t.Helper()

	kafkaCluster, externalListener := testKafkaCluster()
	ingressConfig := v1beta1.IngressConfig{
		EnvoyConfig: &v1beta1.EnvoyConfig{
			EnableAccesslogSampling: enableAccesslogSampling,
		},
	}

	rendered := GenerateEnvoyConfig(kafkaCluster, externalListener, ingressConfig,
		util.IngressConfigGlobalName, util.IngressConfigGlobalName, logr.Discard())
	if rendered == "" {
		t.Fatal("GenerateEnvoyConfig returned an empty config")
	}

	return rendered
}

// TestGenerateEnvoyConfigRuntimeLayerIsGated is the safety net for this feature:
// koperator is a public fork, so the rendered config must not move for clusters
// that have not opted in.
func TestGenerateEnvoyConfigRuntimeLayerIsGated(t *testing.T) {
	t.Parallel()

	t.Run("flag off omits the runtime layer", func(t *testing.T) {
		t.Parallel()

		if got := renderEnvoyConfig(t, false); strings.Contains(got, "layeredRuntime") {
			t.Errorf("flag off rendered a runtime layer, which would change the config of every existing cluster:\n%s", got)
		}
	})

	t.Run("flag on renders the admin runtime layer", func(t *testing.T) {
		t.Parallel()

		got := renderEnvoyConfig(t, true)

		// Indented exactly as it appears at the top level of the bootstrap,
		// between admin and staticResources.
		const want = `layeredRuntime:
  layers:
  - adminLayer: {}
    name: admin
`

		if !strings.Contains(got, want) {
			t.Errorf("flag on did not render the expected runtime layer\n--- want ---\n%s\n--- got ---\n%s", want, got)
		}
	})

	t.Run("the flag changes nothing else", func(t *testing.T) {
		t.Parallel()

		off, on := renderEnvoyConfig(t, false), renderEnvoyConfig(t, true)

		const runtimeLayer = `layeredRuntime:
  layers:
  - adminLayer: {}
    name: admin
`

		// Removing the runtime layer from the flag-on render must give back the
		// flag-off render exactly. Anything else means the flag leaked into a
		// part of the config it has no business touching.
		if stripped := strings.Replace(on, runtimeLayer, "", 1); stripped != off {
			t.Errorf("enabling the flag changed more than the runtime layer\n--- flag off ---\n%s\n--- flag on, layer removed ---\n%s", off, stripped)
		}
	})
}
