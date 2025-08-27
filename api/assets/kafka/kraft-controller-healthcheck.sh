#!/bin/bash
# Copyright 2025 Cisco Systems, Inc. and/or its affiliates
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


# This script returns a successful exit code (0) if the controller is a follower or leader.  For any other state, it returns a failure exit code (1).
# In addition, if the environment variable KRAFT_HEALTH_CHECK_SKIP is set to "true" (case insensitive), the script will exit successfully without performing any checks.

skip_check=$(echo "$KRAFT_HEALTH_CHECK_SKIP" | tr '[:upper:]' '[:lower:]')

if [ "$skip_check" = "true" ]; then
    echo "KRAFT_HEALTH_CHECK_SKIP is set to TRUE. Exiting health check."
    exit 0
fi

JMX_ENDPOINT="http://localhost:9020/metrics"
METRIC_PREFIX="kafka_server_raft_metrics_current_state_"

# Fetch the matching current-state metric with value of 1.0 from the JMX endpoint
MATCHING_METRIC=$(curl -s "$JMX_ENDPOINT" | grep "^${METRIC_PREFIX}" | awk '$2 == 1.0 {print $1}')

# If it's not empty, it means we found a metric with a value of 1.0.
if [ -n "$MATCHING_METRIC" ]; then
    # Determine the state of the controller using the last field name of the metric 
    # Possible values are leader, candidate, voted, follower, unattached, observer
    STATE=$(echo "$MATCHING_METRIC" | rev | cut -d'_' -f1 | rev)

    # Check if the extracted state is 'leader' or 'follower'
    if [ "$STATE" == "leader" ] || [ "$STATE" == "follower" ]; then
        echo "The controller is in a healthy quorum state."
        exit 0
    else
        # Any other state (e.g., 'candidate', 'unattached', 'observer') is not considered healthy
        echo "Failure: The controller is in an unexpected state: $STATE. Expecting 'leader' or 'follower'."
        exit 1
    fi
else
    echo "JMX Exporter endpoint is not avaiable or kafka_server_raft_metrics_current_state_ was not found."
    exit 0
fi
