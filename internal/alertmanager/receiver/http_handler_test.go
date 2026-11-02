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

package receiver

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/banzaicloud/koperator/api/v1beta1"
	"github.com/banzaicloud/koperator/internal/alertmanager/currentalert"
)

func TestHTTPReceiverPayloads(t *testing.T) {
	for _, tc := range []struct {
		name       string
		envelope   bool
		invalid    string
		wantStatus int
	}{
		{name: "Prometheus array", wantStatus: http.StatusAccepted},
		{name: "Alertmanager webhook", envelope: true, wantStatus: http.StatusAccepted},
		{name: "malformed JSON", invalid: "{", wantStatus: http.StatusBadRequest},
		{name: "missing webhook alerts", invalid: `{"receiver":"actions"}`, wantStatus: http.StatusBadRequest},
		{name: "invalid webhook alerts", invalid: `{"alerts":{}}`, wantStatus: http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			require.NoError(t, v1beta1.AddToScheme(scheme))
			cluster := &v1beta1.KafkaCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "kafka", Namespace: "kafka"},
				Spec: v1beta1.KafkaClusterSpec{
					Brokers: []v1beta1.Broker{{Id: 100}, {Id: 101}, {Id: 102}, {Id: 103}},
				},
				Status: v1beta1.KafkaClusterStatus{State: v1beta1.KafkaClusterRunning},
			}
			kubeClient := fake.NewClientBuilder().WithScheme(scheme).
				WithStatusSubresource(cluster).WithObjects(cluster).Build()
			labels := model.LabelSet{
				"alertname": model.LabelValue(tc.name), "namespace": "kafka",
				"kafka_cr": "kafka", "brokerId": "103",
			}
			t.Cleanup(func() {
				require.NoError(t, currentalert.GetCurrentAlerts().DeleteAlert(labels.Fingerprint()))
			})
			alerts := []model.Alert{{
				Labels:      labels,
				Annotations: model.LabelSet{"command": "downScale"},
				StartsAt:    time.Now(),
				EndsAt:      time.Now().Add(time.Hour),
			}}
			var payload interface{} = alerts
			if tc.envelope {
				payload = map[string]interface{}{
					"version": "4", "receiver": "actions", "status": "firing", "alerts": alerts,
				}
			}
			body, err := json.Marshal(payload)
			require.NoError(t, err)
			if tc.invalid != "" {
				body = []byte(tc.invalid)
			}
			request := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
			response := httptest.NewRecorder()
			NewHTTPHandler(logr.Discard(), kubeClient).ServeHTTP(response, request)
			require.Equal(t, tc.wantStatus, response.Code, response.Body.String())

			actual := &v1beta1.KafkaCluster{}
			require.NoError(t, kubeClient.Get(context.Background(),
				types.NamespacedName{Name: "kafka", Namespace: "kafka"}, actual))
			want := []int32{100, 101, 102, 103}
			if tc.wantStatus == http.StatusAccepted {
				want = []int32{100, 101, 102}
			}
			var ids []int32
			for _, broker := range actual.Spec.Brokers {
				ids = append(ids, broker.Id)
			}
			require.Equal(t, want, ids)
			if tc.wantStatus == http.StatusAccepted {
				alerts[0].EndsAt = time.Now().Add(-time.Minute)
				var resolved interface{} = alerts
				if tc.envelope {
					resolved = map[string]interface{}{"status": "resolved", "alerts": alerts}
				}
				body, err := json.Marshal(resolved)
				require.NoError(t, err)
				response = httptest.NewRecorder()
				NewHTTPHandler(logr.Discard(), kubeClient).ServeHTTP(response,
					httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body)))
				require.Equal(t, http.StatusAccepted, response.Code)
				require.NotContains(t, currentalert.GetCurrentAlerts().ListAlerts(), labels.Fingerprint())
			}
		})
	}
}
