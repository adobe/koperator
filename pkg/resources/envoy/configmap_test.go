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
	"fmt"
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
//
// "broker_101" here is a literal copied from that artifact, not a claim about
// how chain names are built. Whether a name actually tracks its broker ID is
// checked by TestGenerateEnvoyConfigChainSamplers, which renders several
// brokers.
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
// testKafkaCluster is the smallest cluster that still renders a complete Envoy
// config: an anycast listener, a per-broker listener and the health-check
// listener.
//
// BrokerConfig must be non-nil. util.ShouldIncludeBroker returns false for a
// broker with a nil config and no status, and the broker's filter chain is then
// silently dropped from the render - leaving a config that looks plausible but
// exercises none of the per-broker code.
//
// tlsEnabled drives ExternalListenerConfig.TLSEnabled, which is
// ExternalStartingPort == -1. The two values select different chain builders
// (GenerateEnvoyTLSFilterChain vs GenerateEnvoyFilterChain), so anything
// attached to a chain has to be tested both ways.
//
// brokerIDs are variadic so a caller can ask for more than one broker. Prefer
// testBrokerIDs over a single broker: with one broker, anything derived from a
// broker ID is indistinguishable from a constant.
func testKafkaCluster(tlsEnabled bool, brokerIDs ...int32) (*v1beta1.KafkaCluster, v1beta1.ExternalListenerConfig) {
	externalStartingPort := int32(19090)
	if tlsEnabled {
		externalStartingPort = -1
	}

	brokers := make([]v1beta1.Broker, 0, len(brokerIDs))
	for _, brokerID := range brokerIDs {
		brokers = append(brokers, v1beta1.Broker{Id: brokerID, BrokerConfig: &v1beta1.BrokerConfig{}})
	}

	kafkaCluster := &v1beta1.KafkaCluster{
		Spec: v1beta1.KafkaClusterSpec{
			IngressController: "envoy",
			Brokers:           brokers,
			ListenersConfig: v1beta1.ListenersConfig{
				ExternalListeners: []v1beta1.ExternalListenerConfig{{
					CommonListenerSpec: v1beta1.CommonListenerSpec{
						Name:          "external",
						ContainerPort: 9094,
					},
					ExternalStartingPort: externalStartingPort,
				}},
			},
		},
	}

	return kafkaCluster, kafkaCluster.Spec.ListenersConfig.ExternalListeners[0]
}

// testBrokerIDs is the default broker set for render tests.
//
// It is deliberately not a single broker, and deliberately not 101 alone. The
// cluster this feature was validated against happens to run brokers 101 and
// 102, and a fixture that mirrors it cannot tell a per-broker value apart from
// a hard-coded one: a brokerFilterChainName that ignored its argument and
// always returned "broker_101" passed the entire suite.
//
// The IDs are non-contiguous and do not start at 1, so an off-by-one or an
// index-for-ID mix-up shows up as a wrong chain name rather than a coincidence.
var testBrokerIDs = []int32{0, 7, 102}

func renderEnvoyConfig(t *testing.T, tlsEnabled, enableAccesslogSampling bool) string {
	t.Helper()

	kafkaCluster, externalListener := testKafkaCluster(tlsEnabled, testBrokerIDs...)
	ingressConfig := v1beta1.IngressConfig{
		// Both hostnames are needed for the TLS path to render a config Envoy
		// will accept: they become the chains' SNI match, and Envoy rejects a
		// listener whose chains share identical matching rules.
		HostnameOverride: "kafka-all-brokers.test",
		EnvoyConfig: &v1beta1.EnvoyConfig{
			BrokerHostnameTemplate:  "kafka-%id.test",
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

		if got := renderEnvoyConfig(t, true, false); strings.Contains(got, "layeredRuntime") {
			t.Errorf("flag off rendered a runtime layer, which would change the config of every existing cluster:\n%s", got)
		}
	})

	t.Run("flag on renders the admin runtime layer", func(t *testing.T) {
		t.Parallel()

		got := renderEnvoyConfig(t, true, true)

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
}

// parseRendered unmarshals a rendered Envoy config so the tests below can
// assert on structure instead of on YAML formatting.
func parseRendered(t *testing.T, rendered string) map[string]any {
	t.Helper()

	var config map[string]any
	if err := yaml.Unmarshal([]byte(rendered), &config); err != nil {
		t.Fatalf("could not parse the rendered config: %v", err)
	}

	return config
}

// allFilterChains flattens every filter chain of every listener, which is where
// both the health-check listener and the broker listeners end up. Keeping them
// in one slice is deliberate: a sampler leaking onto the health-check chain
// would otherwise go unnoticed.
func allFilterChains(config map[string]any) []map[string]any {
	var chains []map[string]any

	staticResources, _ := config["staticResources"].(map[string]any)
	listeners, _ := staticResources["listeners"].([]any)

	for _, listener := range listeners {
		listenerMap, _ := listener.(map[string]any)
		filterChains, _ := listenerMap["filterChains"].([]any)

		for _, filterChain := range filterChains {
			if chain, ok := filterChain.(map[string]any); ok {
				chains = append(chains, chain)
			}
		}
	}

	return chains
}

// chainNetworkFilterConfig returns the typed config of a chain's first network
// filter, which is the TcpProxy on the broker and anycast chains.
func chainNetworkFilterConfig(chain map[string]any) map[string]any {
	filters, _ := chain["filters"].([]any)
	if len(filters) == 0 {
		return nil
	}

	filter, _ := filters[0].(map[string]any)
	typedConfig, _ := filter["typedConfig"].(map[string]any)

	return typedConfig
}

// chainHasRuntimeSampler reports whether a chain carries one of this feature's
// samplers.
//
// Mere presence of an accessLog is not enough to tell: the health-check chain
// already ships an unconditional stdout access log. The runtimeFilter is what
// makes a log ours, so that is what gets looked for.
func chainHasRuntimeSampler(chain map[string]any) bool {
	accessLogs, _ := chainNetworkFilterConfig(chain)["accessLog"].([]any)

	for _, entry := range accessLogs {
		accessLog, _ := entry.(map[string]any)
		filter, _ := accessLog["filter"].(map[string]any)
		if _, ok := filter["runtimeFilter"]; ok {
			return true
		}
	}

	return false
}

// chainSampler digs out the runtime key and the log line format of a chain's
// access log, so a test can check what the chain samples and what it writes.
func chainSampler(t *testing.T, chain map[string]any) (runtimeKey, logFormat string) {
	t.Helper()

	accessLogs, _ := chainNetworkFilterConfig(chain)["accessLog"].([]any)
	if len(accessLogs) != 1 {
		t.Fatalf("chain %v: expected exactly one access log, got %d", chain["name"], len(accessLogs))
	}

	accessLog, _ := accessLogs[0].(map[string]any)

	filter, _ := accessLog["filter"].(map[string]any)
	runtimeFilter, _ := filter["runtimeFilter"].(map[string]any)
	runtimeKey, _ = runtimeFilter["runtimeKey"].(string)

	typedConfig, _ := accessLog["typedConfig"].(map[string]any)
	format, _ := typedConfig["logFormat"].(map[string]any)
	textFormatSource, _ := format["textFormatSource"].(map[string]any)
	logFormat, _ = textFormatSource["inlineString"].(string)

	return runtimeKey, logFormat
}

// TestGenerateEnvoyConfigChainSamplers checks the chains an operator will reach
// for: which runtime key each one listens to, and that a chain's name matches
// the name it writes into its own log lines.
//
// Both TLS modes are covered because they are built by different functions
// (GenerateEnvoyTLSFilterChain and GenerateEnvoyFilterChain), and the sampler
// has to survive either path.
func TestGenerateEnvoyConfigChainSamplers(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name       string
		tlsEnabled bool
	}{
		{name: "tls listener", tlsEnabled: true},
		{name: "plaintext listener", tlsEnabled: false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			config := parseRendered(t, renderEnvoyConfig(t, testCase.tlsEnabled, true))

			// Every broker gets its own chain, and all of them share a single
			// runtime key: the knob is per chain *type*, not per broker, so one
			// admin call covers the whole cluster.
			want := map[string]string{anycastFilterChainName: AccessLogRuntimeKeyAnycast}
			for _, brokerID := range testBrokerIDs {
				want[fmt.Sprintf("broker_%d", brokerID)] = AccessLogRuntimeKeyBroker
			}
			got := make(map[string]string, len(want))

			for _, chain := range allFilterChains(config) {
				name, named := chain["name"].(string)
				if !named {
					// The health-check chain stays unnamed, and keeps its own
					// pre-existing stdout log rather than gaining a sampler.
					if chainHasRuntimeSampler(chain) {
						t.Error("an unnamed filter chain carries a sampler, so its log lines could not be attributed to a chain")
					}
					continue
				}

				runtimeKey, logFormat := chainSampler(t, chain)
				got[name] = runtimeKey

				// The chain name is the only thing tying a log line back to the
				// chain that emitted it, so the two must not drift apart.
				if !strings.Contains(logFormat, "chain="+name+" ") {
					t.Errorf("chain %q does not log its own name, making its lines untraceable: %q", name, logFormat)
				}
			}

			if len(got) != len(want) {
				t.Fatalf("expected the chains %v to be named and sampled, got %v", want, got)
			}
			for name, wantKey := range want {
				if got[name] != wantKey {
					t.Errorf("chain %q samples on %q, want %q", name, got[name], wantKey)
				}
			}
		})
	}
}

// TestGenerateEnvoyConfigFlagOffAddsNothing is the regression test that matters
// most here. koperator is a public fork, so a cluster that has not opted in has
// to render exactly what it rendered before.
//
// It works by removing everything the flag is allowed to add and requiring the
// remainder to equal the flag-off render. A presence check could not do this:
// it would pass even if the flag had also changed something elsewhere.
func TestGenerateEnvoyConfigFlagOffAddsNothing(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name       string
		tlsEnabled bool
	}{
		{name: "tls listener", tlsEnabled: true},
		{name: "plaintext listener", tlsEnabled: false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			off := parseRendered(t, renderEnvoyConfig(t, testCase.tlsEnabled, false))
			on := parseRendered(t, renderEnvoyConfig(t, testCase.tlsEnabled, true))

			for _, chain := range allFilterChains(off) {
				if name, named := chain["name"]; named {
					t.Errorf("flag off named a filter chain %q, which moves the rendered config of every existing cluster", name)
				}
				if chainHasRuntimeSampler(chain) {
					t.Error("flag off attached a sampler to a filter chain")
				}
			}

			delete(on, "layeredRuntime")
			for _, chain := range allFilterChains(on) {
				if _, named := chain["name"]; !named {
					continue
				}
				delete(chain, "name")
				delete(chainNetworkFilterConfig(chain), "accessLog")
			}

			if stripped, want := mustMarshal(t, on), mustMarshal(t, off); stripped != want {
				t.Errorf("enabling the flag changed more than the runtime layer, the chain names and the samplers\n--- flag off ---\n%s\n--- flag on, additions removed ---\n%s", want, stripped)
			}
		})
	}
}

func mustMarshal(t *testing.T, config map[string]any) string {
	t.Helper()

	marshalled, err := yaml.Marshal(config)
	if err != nil {
		t.Fatalf("could not marshal the config: %v", err)
	}

	return string(marshalled)
}
