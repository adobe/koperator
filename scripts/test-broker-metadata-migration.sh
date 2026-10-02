#!/usr/bin/env bash
# Copyright 2026 Adobe. All rights reserved.
# SPDX-License-Identifier: Apache-2.0
set -euo pipefail

KAFKA_HOME=${KAFKA_HOME:-/opt/kafka}
MIGRATOR=${MIGRATOR:-/test/migrate-broker-metadata.sh}
export KAFKA_HEAP_OPTS="-Xms128m -Xmx256m"
controller_pid=""
broker_pid=""

cleanup() {
  for pid in "$broker_pid" "$controller_pid"; do
    if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
      kill "$pid"
      wait "$pid" || echo "Kafka process $pid terminated during cleanup" >&2
    fi
  done
}
trap cleanup EXIT

stop_broker() {
  kill "$broker_pid"
  if wait "$broker_pid"; then
    :
  else
    code=$?
    [[ "$code" == 143 ]] || return "$code"
  fi
  broker_pid=""
}

write_broker_config() {
  cat > "$root/broker.properties" <<EOF
process.roles=broker
node.id=1
controller.quorum.voters=1000@127.0.0.1:19093
controller.listener.names=CONTROLLER
listener.security.protocol.map=CONTROLLER:PLAINTEXT,PLAINTEXT:PLAINTEXT
listeners=PLAINTEXT://127.0.0.1:19092
advertised.listeners=PLAINTEXT://127.0.0.1:19092
inter.broker.listener.name=PLAINTEXT
log.dirs=$root/logs1/kafka,$root/logs2/kafka
metadata.log.dir=$1
num.network.threads=1
num.io.threads=2
offsets.topic.replication.factor=1
offsets.topic.num.partitions=1
transaction.state.log.replication.factor=1
transaction.state.log.min.isr=1
group.initial.rebalance.delay.ms=0
EOF
}

wait_for_broker() {
  for attempt in {1..30}; do
    if "$KAFKA_HOME/bin/kafka-topics.sh" --bootstrap-server 127.0.0.1:19092 \
      --command-config "$root/client.properties" --list > "$root/topics" 2> "$root/client-error"; then
      return
    fi
    if ! kill -0 "$broker_pid" 2>/dev/null; then
      cat "$root/broker.log" >&2
      return 1
    fi
    sleep 1
  done
  cat "$root/broker.log" "$root/client-error" >&2
  return 1
}

for source in logs1 logs2; do
  root=$(mktemp -d /tmp/koperator-metadata-smoke.XXXXXX)
  mkdir -p "$root/logs1" "$root/logs2" "$root/metadata" "$root/wait"
  cluster_id=$("$KAFKA_HOME/bin/kafka-storage.sh" random-uuid)
  printf 'request.timeout.ms=3000\ndefault.api.timeout.ms=5000\n' > "$root/client.properties"
  cat > "$root/controller.properties" <<EOF
process.roles=controller
node.id=1000
controller.quorum.voters=1000@127.0.0.1:19093
controller.listener.names=CONTROLLER
listener.security.protocol.map=CONTROLLER:PLAINTEXT
listeners=CONTROLLER://127.0.0.1:19093
log.dirs=$root/controller
EOF
  write_broker_config "$root/$source/kafka"
  "$KAFKA_HOME/bin/kafka-storage.sh" format -t "$cluster_id" -c "$root/controller.properties"
  "$KAFKA_HOME/bin/kafka-storage.sh" format -t "$cluster_id" -c "$root/broker.properties"
  "$KAFKA_HOME/bin/kafka-server-start.sh" "$root/controller.properties" > "$root/controller.log" 2>&1 &
  controller_pid=$!
  "$KAFKA_HOME/bin/kafka-server-start.sh" "$root/broker.properties" > "$root/broker.log" 2>&1 &
  broker_pid=$!
  wait_for_broker
  "$KAFKA_HOME/bin/kafka-topics.sh" --bootstrap-server 127.0.0.1:19092 \
    --create --topic migration-smoke --partitions 1 --replication-factor 1
  printf 'before-migration\n' | "$KAFKA_HOME/bin/kafka-console-producer.sh" \
    --bootstrap-server 127.0.0.1:19092 --topic migration-smoke
  stop_broker

  write_broker_config "$root/metadata/kafka"
  CLUSTER_ID="$cluster_id" NODE_ID=1 METADATA_MOUNT="$root/metadata" \
    DATA_MOUNTS="$root/logs1,$root/logs2" BROKER_CONFIG="$root/broker.properties" \
    WORK_DIR="$root/wait" ALLOW_FRESH=false bash "$MIGRATOR"
  [[ -d "$root/$source/.koperator-metadata-backup-1" ]]
  [[ ! -e "$root/logs1/kafka/__cluster_metadata-0" && ! -e "$root/logs2/kafka/__cluster_metadata-0" ]]

  for restart in 1 2; do
    CLUSTER_ID="$cluster_id" NODE_ID=1 METADATA_MOUNT="$root/metadata" \
      DATA_MOUNTS="$root/logs1,$root/logs2" BROKER_CONFIG="$root/broker.properties" \
      WORK_DIR="$root/wait" ALLOW_FRESH=false bash "$MIGRATOR"
    "$KAFKA_HOME/bin/kafka-server-start.sh" "$root/broker.properties" > "$root/broker.log" 2>&1 &
    broker_pid=$!
    wait_for_broker
    "$KAFKA_HOME/bin/kafka-console-consumer.sh" --bootstrap-server 127.0.0.1:19092 \
      --topic migration-smoke --from-beginning --max-messages 1 --timeout-ms 15000 \
      > "$root/consumed"
    [[ "$(cat "$root/consumed")" == before-migration ]]
    "$KAFKA_HOME/bin/kafka-metadata-quorum.sh" --bootstrap-server 127.0.0.1:19092 describe --status
    stop_broker
  done
  kill "$controller_pid"
  if wait "$controller_pid"; then
    :
  else
    code=$?
    [[ "$code" == 143 ]] || exit "$code"
  fi
  controller_pid=""
  echo "Kafka metadata migration smoke test passed: $source"
done
