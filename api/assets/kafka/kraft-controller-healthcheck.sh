#!/bin/bash

# This script returns a successful exit code (0) if the controller is a follower or leader.  For any other state, it returns a failure exit code (1).

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
        echo "Warning: The controller is not in a healthy state for this check."
        exit 1
    fi
else
    echo "Failure: No active Kraft controller state found with a value of 1.0."
    exit 1
fi
