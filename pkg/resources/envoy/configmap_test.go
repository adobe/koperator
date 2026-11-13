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

	envoyfileaccesslog "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/file/v3"
	envoytypes "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"google.golang.org/protobuf/encoding/protojson"
	"sigs.k8s.io/yaml"
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
