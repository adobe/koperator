#!/bin/bash

set -xeuo pipefail

# Clean up any stale data from previous runs
rm -rf /var/lib/kafka/data*/*
rm -rf /tmp/kafka-logs/*

until test -f "/metrics/cruise-control-metrics-reporter.jar"; do
  >&2 echo "Waiting for bootstrap - sleeping"
  sleep 3
done

exec kafka-server-start.sh /opt/kafka/config/server.properties --override zookeeper.connect=zookeeper:2181 --override broker.id=${KAFKA_BROKER_ID} --override log.dirs=${KAFKA_LOG_DIRS}