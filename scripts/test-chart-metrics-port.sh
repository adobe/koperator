#!/usr/bin/env bash
# Copyright 2026 Adobe. All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Run from the repository root with Helm and yq v4 on PATH.
set -euo pipefail

expect() {
  local expected=$1 query=$2 actual
  actual=$(yq eval-all "$query" - <<< "$rendered")
  if [[ "$actual" != "$expected" ]]; then
    printf 'proxy=%s port=%s: %s\nexpected %s, got %s\n' "$proxy" "$port" "$query" "$expected" "$actual" >&2
    exit 1
  fi
}

containers='select(.kind == "Deployment") | .spec.template.spec.containers[]'
manager="$containers | select(.name == \"manager\")"
monitor='select(.kind == "PodMonitor") | .spec.podMetricsEndpoints[0]'
raw_service='select(.kind == "Service" and .metadata.name == "kafka-operator-operator")'

for proxy in true false; do
  for port in 9090 8080; do
    args=(--set prometheusMetrics.podMonitor.enabled=true --set prometheusMetrics.authProxy.enabled="$proxy")
    if [[ "$port" != 8080 ]]; then args+=(--set metricEndpoint.port="$port"); fi
    rendered=$(helm template kafka-operator charts/kafka-operator "${args[@]}" "$@")

    expect "--metrics-addr=:$port" "$manager | .args[] | select(. == \"--metrics-addr=*\")"
    expect "$port" "$manager | .ports[] | select(.name == \"metrics\") | .containerPort"
    expect metrics "$monitor | .port"
    expect http "$monitor | .scheme // \"http\""
    if [[ "$proxy" == true ]]; then
      expect "--upstream=http://127.0.0.1:$port/" "$containers | select(.name == \"kube-rbac-proxy\") | .args[] | select(. == \"--upstream=*\")"
    else
      expect 8080 "$raw_service | .metadata.annotations[\"prometheus.io/port\"]"
      expect 8080 "$raw_service | .spec.ports[] | select(.targetPort == \"metrics\") | .port"
    fi
  done
done

echo "chart metrics port checks passed"
