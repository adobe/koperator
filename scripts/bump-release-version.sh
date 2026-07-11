#!/usr/bin/env bash
# Copyright 2026 Cisco Systems, Inc. and/or its affiliates
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

# Bumps every release version reference across the Helm chart and top-level
# docs to the given tag. Run from the repository root. Used by the release
# workflow both to package the chart (ephemeral checkout) and to sync the
# same references back into git for canonical dated releases.

set -euo pipefail

: ${1?"Usage: $0 <release-tag> e.g. 0.28.0-adobe-20260622"}
TAG="$1"

CHART_DIR="charts/kafka-operator"

sed -i "s/^version:.*/version: \"${TAG}\"/" "${CHART_DIR}/Chart.yaml"
sed -i "s/^appVersion:.*/appVersion: \"${TAG}\"/" "${CHART_DIR}/Chart.yaml"

# Ensure consistent space-based indentation before patching values.yaml
sed -i 's/\t/  /g' "${CHART_DIR}/values.yaml"
sed -i "/# -- Operator container image tag/{n;s/^[[:space:]]*tag:.*/    tag: \"${TAG}\"/}" "${CHART_DIR}/values.yaml"

for f in "${CHART_DIR}/README.md" "${CHART_DIR}/README.md.gotmpl"; do
  sed -i "s/--version [0-9]\+\.[0-9]\+\.[0-9]\+-[a-zA-Z0-9-]\+/--version ${TAG}/g" "$f"
  sed -i "s/\`\"\?[0-9]\+\.[0-9]\+\.[0-9]\+-[a-zA-Z0-9-]\+\"\?\`/\`\"${TAG}\"\`/g" "$f"
done

sed -i "s/--version [0-9]\+\.[0-9]\+\.[0-9]\+-[a-zA-Z0-9-]\+/--version ${TAG}/g" README.md
sed -i "s#img.shields.io/github/go-mod/go-version/adobe/koperator/[0-9]\+\.[0-9]\+\.[0-9]\+-[a-zA-Z0-9-]\+#img.shields.io/github/go-mod/go-version/adobe/koperator/${TAG}#" README.md
sed -i "s/kafka-operator-[0-9]\+\.[0-9]\+\.[0-9]\+-[a-zA-Z0-9-]\+\.tgz/kafka-operator-${TAG}.tgz/g" README.md
