#!/bin/bash

set -xeuo pipefail

readonly KAFKA_BROKERS="${KAFKA_BROKERS:?'KAFKA_BROKERS environment variable must be set!'}"
readonly CONSUMER_CONFIG="${CONSUMER_CONFIG:-/tmp/consumer.properties}"
readonly TOPIC="${TOPIC:?'TOPIC environment variable must be set!'}"
readonly PID_FILE="/tmp/consumer.pid"

cleanup() {
  rm -f "${PID_FILE}"
}
trap cleanup EXIT

consume() {
  kafka-console-consumer \
    --bootstrap-server "${KAFKA_BROKERS}" \
    --consumer.config "${CONSUMER_CONFIG}" \
    --topic "${TOPIC}"
}

main() {
  printf "%s" "$$" > "${PID_FILE}"
  consume
}

main