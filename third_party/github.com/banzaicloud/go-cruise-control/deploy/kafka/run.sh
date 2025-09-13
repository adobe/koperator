#!/bin/bash

set -xeuo pipefail

until test -f "/metrics/cruise-control-metrics-reporter.jar"; do
  >&2 echo "Waiting for bootstrap - sleeping"
  sleep 3
done

exec /etc/confluent/docker/run