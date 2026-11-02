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

set -euo pipefail

root="$(cd "$(dirname "$0")/../.." && pwd)"
sample="$root/config/samples/kafkacluster-prometheus.yaml"
tmp="$root/.prometheus-test.$$"
mkdir -m 700 "$tmp"
trap 'rm -f "$tmp/rules.yaml" "$tmp/alertmanager.yaml" "$tmp/tests.yaml"; rmdir "$tmp"' EXIT

yq 'select(.kind == "PrometheusRule") | {"groups": .spec.groups}' "$sample" > "$tmp/rules.yaml"
yq -r 'select(.kind == "Secret" and .metadata.name == "alertmanager-kafka-alertmanager") | .stringData."alertmanager.yaml"' "$sample" > "$tmp/alertmanager.yaml"
cp "$root/tests/prometheus/rules.test.yaml" "$tmp/tests.yaml"

docker run --rm --entrypoint /bin/promtool \
  -v "$tmp:/work:ro" -w /work \
  quay.io/prometheus/prometheus:v3.15.1-distroless test rules tests.yaml

docker run --rm --entrypoint /bin/amtool -v "$tmp:/work:ro" \
  quay.io/prometheus/alertmanager:v0.34.2 check-config /work/alertmanager.yaml

for alert in BrokerRequestRateHigh BrokerUnderReplicated PartitionCountHigh PartitionCountLow; do
  docker run --rm --entrypoint /bin/amtool -v "$tmp:/work:ro" \
    quay.io/prometheus/alertmanager:v0.34.2 config routes test \
    --config.file=/work/alertmanager.yaml --verify.receivers=monitoring \
    "alertname=$alert" alertGroup=kafka
done

docker run --rm --entrypoint /bin/amtool -v "$tmp:/work:ro" \
  quay.io/prometheus/alertmanager:v0.34.2 config routes test \
  --config.file=/work/alertmanager.yaml --verify.receivers=koperator-storage \
  alertname=RemainingDiskSpaceLow alertGroup=kafka
