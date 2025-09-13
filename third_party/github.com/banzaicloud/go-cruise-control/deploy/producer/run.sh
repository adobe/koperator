#!/bin/bash

set -xeuo pipefail

readonly DATA_URL="${DATA_URL:?}"
readonly DATA_FILE="${DATA_FILE:-/tmp/data.csv}"
readonly KAFKA_BROKERS="${KAFKA_BROKERS:?'KAFKA_BROKERS environment variable must be set!'}"
readonly PRODUCER_CONFIG="${PRODUCER_CONFIG:-/tmp/producer.properties}"
readonly TOPIC="${TOPIC:?'TOPIC environment variable must be set!'}"
readonly PID_FILE="/tmp/producer.pid"


cleanup() {
  rm -f "${PID_FILE}"
}
trap cleanup EXIT


create_topic() {
  kafka-topics \
    --bootstrap-server "${KAFKA_BROKERS}" \
    --create \
    --if-not-exists \
    --partitions 30 \
    --replication-factor 2 \
    --topic "${TOPIC}"
}


download_data() {
  curl -sSfL -4 -o "${DATA_FILE}" \
  "${DATA_URL}"
}


produce_from_csv() {
  while :; do
    tail -n +2 "${DATA_FILE}" \
    | awk -F ',' '{print $1"::"$0}' \
    | kafka-console-producer \
      --bootstrap-server "${KAFKA_BROKERS}" \
      --producer.config "${PRODUCER_CONFIG}" \
      --property parse.key=true \
      --property key.separator='::' \
      --sync \
      --topic "${TOPIC}"
    sleep .2
  done
}


main() {
  printf "%s" "$$" > "${PID_FILE}"
  create_topic
  download_data
  produce_from_csv
}

main
