// Copyright © 2020 Cisco Systems, Inc. and/or its affiliates
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

package kafka

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestWaitForEnvoySidecarExitCodes verifies that the wait-for-envoy-sidecar.sh script
// correctly translates exit code 143 (SIGTERM controlled shutdown) to 0, and propagates
// all other exit codes unchanged.
func TestWaitForEnvoySidecarExitCodes(t *testing.T) {
	tests := []struct {
		name         string
		kafkaExit    int
		expectedExit int
	}{
		{
			name:         "graceful shutdown (SIGTERM → exit 0)",
			kafkaExit:    143,
			expectedExit: 0,
		},
		{
			name:         "OOM kill (SIGKILL) propagated",
			kafkaExit:    137,
			expectedExit: 137,
		},
		{
			name:         "generic crash propagated",
			kafkaExit:    1,
			expectedExit: 1,
		},
		{
			name:         "normal exit propagated",
			kafkaExit:    0,
			expectedExit: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmp := t.TempDir()

			// Create mock kafka-server-start.sh that exits with the desired code.
			binDir := filepath.Join(tmp, "bin")
			if err := os.MkdirAll(binDir, 0o755); err != nil {
				t.Fatalf("create bin dir: %v", err)
			}
			stub := filepath.Join(binDir, "kafka-server-start.sh")
			stubContent := fmt.Sprintf("#!/bin/bash\nexit %d\n", tc.kafkaExit)
			if err := os.WriteFile(stub, []byte(stubContent), 0o755); err != nil {
				t.Fatalf("write stub: %v", err)
			}

			waitDir := filepath.Join(tmp, "wait")
			if err := os.MkdirAll(waitDir, 0o755); err != nil {
				t.Fatalf("create wait dir: %v", err)
			}

			cmd := exec.Command("bash", "-c", envoySidecarScript)
			cmd.Env = []string{
				"KAFKA_HOME=" + tmp,
				"WAIT_DIR=" + waitDir,
				// Leave ENVOY_SIDECAR_STATUS and CLUSTER_ID unset to skip those blocks.
			}

			err := cmd.Run()

			got := 0
			if err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					got = exitErr.ExitCode()
				} else {
					t.Fatalf("unexpected error running script: %v", err)
				}
			}

			if got != tc.expectedExit {
				t.Errorf("kafka exit %d: want script exit %d, got %d", tc.kafkaExit, tc.expectedExit, got)
			}
		})
	}
}
