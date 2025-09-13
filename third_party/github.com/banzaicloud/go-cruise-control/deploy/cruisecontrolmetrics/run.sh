#!/bin/bash

set -xeuo pipefail

readonly METRICS_REPORTER_FILE="${METRICS_REPORTER_FILE:?}"
readonly DST_DIR="${DST_DIR:-/metrics}"

copy() {
  if [[ -d "${DST_DIR}" ]]
  then
    cp "${METRICS_REPORTER_FILE}" ${DST_DIR}
    return 0
  fi
  return 1
}

main() {
  copy
  sleep infinity
}

main