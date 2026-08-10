#!/bin/bash
# Copyright 2025 Cisco Systems, Inc. and/or its affiliates
# Copyright 2025 Adobe. All rights reserved.
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


# Health check for a KRaft controller, driven by the Prometheus JMX exporter's raft current-state gauge.
# The jmx-exporter maps the "raft-metrics current-state=<state>" MBean attribute to a single gauge named
# kafka_server_raft_metrics_current_state_<state> with value 1, so exactly one such gauge exists at a time
# and its name is the controller's current raft state (leader, follower, candidate, unattached, ...).
#
# Behaviour is selected by KRAFT_HEALTH_CHECK_MODE:
#   readiness        -> fail-closed: succeeds only when the controller is a leader or follower (a
#                       functioning quorum member). Koperator's rolling upgrade waits on this readiness
#                       signal so it never restarts the next controller before the previously restarted
#                       one has rejoined the metadata quorum.
#   liveness (default) -> fails only when the controller is reachable and reporting a state that is not
#                       leader/follower; a not-yet-emitted state (startup/catch-up) or an unreachable
#                       metrics endpoint is treated as healthy (fail-open) so a slow-starting or briefly
#                       unavailable JMX exporter does not restart an otherwise healthy controller.
# If KRAFT_HEALTH_CHECK_SKIP is set to "true" (case insensitive) all checks are skipped.

skip_check=$(echo "$KRAFT_HEALTH_CHECK_SKIP" | tr '[:upper:]' '[:lower:]')
mode=$(echo "${KRAFT_HEALTH_CHECK_MODE:-liveness}" | tr '[:upper:]' '[:lower:]')

if [ "$skip_check" = "true" ]; then
    echo "KRAFT_HEALTH_CHECK_SKIP is set to TRUE. Skipping health check."
    exit 0
fi

METRIC_PREFIX="kafka_server_raft_metrics_current_state_"

# Request only the raft current-state gauges via the Prometheus name[] query filter, so the exporter
# returns a few lines instead of the whole /metrics exposition (which on a combined broker+controller
# node is large). Only the node's current state is ever present, but we list all known raft states so we
# can still tell "reporting a non-leader/follower state" apart from "no state emitted yet". If the
# exporter version ignores name[] it returns the full exposition and the same greps below still work.
STATES="unattached voted prospective candidate leader follower observer resigned"
FILTER=""
for state in ${STATES}; do
    FILTER="${FILTER}&name[]=${METRIC_PREFIX}${state}"
done
URL="http://localhost:9020/metrics?${FILTER#&}"

if ! METRICS=$(curl -s --max-time 4 "${URL}"); then
    echo "JMX exporter metrics endpoint is not reachable."
    # Unreachable: not a confirmed quorum member (not ready), but do not restart on a transient blip.
    [ "${mode}" = "readiness" ] && exit 1
    exit 0
fi

# Healthy quorum member: the leader or follower current-state gauge is present.
if echo "${METRICS}" | grep -Eq "^${METRIC_PREFIX}(leader|follower) 1(\.[0-9]+)?$"; then
    echo "The controller is in a healthy quorum state (leader or follower)."
    exit 0
fi

# Reachable and reporting some other raft state (e.g. candidate, unattached, observer).
if echo "${METRICS}" | grep -q "^${METRIC_PREFIX}"; then
    STATE=$(echo "${METRICS}" | grep "^${METRIC_PREFIX}" | head -n 1 | sed -E "s/^${METRIC_PREFIX}([a-z]+).*/\1/")
    echo "Failure: the controller is in an unexpected state: ${STATE}. Expecting 'leader' or 'follower'."
    exit 1
fi

# Reachable but no raft current-state gauge yet (startup / not caught up).
echo "kafka_server_raft_metrics_current_state_ was not found (controller not caught up yet)."
[ "${mode}" = "readiness" ] && exit 1
exit 0
