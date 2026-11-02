// Copyright © 2023 Cisco Systems, Inc. and/or its affiliates
// Copyright 2025 Adobe. All rights reserved.
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

package scale

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPartitionReplicasByBroker(t *testing.T) {
	for _, tc := range []struct {
		name       string
		statusCode int
		body       string
		wantErr    bool
	}{
		{
			name:       "replica counts rather than leader counts",
			statusCode: http.StatusOK,
			body:       `{"KafkaBrokerState":{"ReplicaCountByBrokerId":{"101":18,"102":9,"303":0},"LeaderCountByBrokerId":{"101":6,"102":3,"303":0}}}`,
		},
		{
			name:       "lookup failure",
			statusCode: http.StatusInternalServerError,
			body:       `{"errorMessage":"cluster state unavailable"}`,
			wantErr:    true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mux := http.NewServeMux()
			mux.HandleFunc("GET /kafkacruisecontrol/kafka_cluster_state", func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.statusCode)
				_, _ = w.Write([]byte(tc.body))
			})
			server := httptest.NewServer(mux)
			defer server.Close()

			cc, err := NewCruiseControlScaler(context.Background(), server.URL+"/kafkacruisecontrol")
			require.NoError(t, err)

			replicas, err := cc.PartitionReplicasByBroker(context.Background())
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, map[string]int32{"101": 18, "102": 9, "303": 0}, replicas)
		})
	}
}

func TestBrokerWithLeastPartitionReplicas(t *testing.T) {
	for _, tc := range []struct {
		name    string
		body    string
		wantErr bool
	}{
		{
			name: "zero replica broker",
			body: `{"KafkaBrokerState":{"ReplicaCountByBrokerId":{"101":18,"102":9,"303":0}}}`,
		},
		{
			name:    "empty replica map",
			body:    `{"KafkaBrokerState":{"ReplicaCountByBrokerId":{}}}`,
			wantErr: true,
		},
		{
			name:    "missing replica map",
			body:    `{"KafkaBrokerState":{}}`,
			wantErr: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mux := http.NewServeMux()
			mux.HandleFunc("GET /kafkacruisecontrol/kafka_cluster_state", func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.body))
			})
			server := httptest.NewServer(mux)
			defer server.Close()

			cc, err := NewCruiseControlScaler(context.Background(), server.URL+"/kafkacruisecontrol")
			require.NoError(t, err)

			brokerID, err := cc.BrokerWithLeastPartitionReplicas(context.Background())
			if tc.wantErr {
				require.Error(t, err)
				require.Empty(t, brokerID)
				return
			}
			require.NoError(t, err)
			require.Equal(t, "303", brokerID)
		})
	}
}

func TestParseBrokerIDsAndLogDirToMap(t *testing.T) {
	testCases := []struct {
		testName            string
		brokerIDsAndLogDirs string
		want                map[int32][]string
		wantErr             bool
	}{
		{
			testName:            "valid input",
			brokerIDsAndLogDirs: "102-/kafka-logs3/kafka,101-/kafka-logs3/kafka,101-/kafka-logs2/kafka",
			want: map[int32][]string{
				101: {"/kafka-logs3/kafka", "/kafka-logs2/kafka"},
				102: {"/kafka-logs3/kafka"},
			},
			wantErr: false,
		},
		{
			testName:            "empty input",
			brokerIDsAndLogDirs: "",
			want:                map[int32][]string{},
			wantErr:             false,
		},
		{
			testName:            "invalid format",
			brokerIDsAndLogDirs: "1-dirA,2-dirB,1",
			want:                nil,
			wantErr:             true,
		},
		{
			testName:            "invalid broker ID",
			brokerIDsAndLogDirs: "1-dirA,abc-dirB,1-dirC",
			want:                nil,
			wantErr:             true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.testName, func(t *testing.T) {
			got, err := parseBrokerIDsAndLogDirsToMap(tc.brokerIDsAndLogDirs)
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.want, got)
			}
		})
	}
}
