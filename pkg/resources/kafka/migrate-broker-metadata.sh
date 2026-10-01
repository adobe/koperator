# Copyright 2026 Adobe. All rights reserved.
# SPDX-License-Identifier: Apache-2.0
#
# Runs only in a broker-only init container, after the previous pod terminates.
# Source topic replicas and data-directory meta.properties are never changed.
set -euo pipefail
KAFKA_HOME=${KAFKA_HOME:-/opt/kafka}
BROKER_CONFIG=${BROKER_CONFIG:-/config/broker-config}
WORK_DIR=${WORK_DIR:-/var/run/wait}

fail() { echo "KRaft metadata migration: $*" >&2; exit 1; }
property() {
  local key=$1 file=$2 line value="" count=0
  [[ -f "$file" && ! -L "$file" ]] || fail "missing or symlinked properties: $file"
  while IFS= read -r line || [[ -n "$line" ]]; do
    if [[ "$line" == "$key="* ]]; then
      value=${line#*=}
      count=$((count + 1))
    fi
  done < "$file"
  [[ $count == 1 && -n "$value" ]] || fail "missing/duplicate $key in $file"
  printf '%s' "$value"
}
identity() {
  [[ "$(property cluster.id "$1/meta.properties")" == "$CLUSTER_ID" ]] || fail "wrong cluster ID in $1"
  [[ "$(property node.id "$1/meta.properties")" == "$NODE_ID" ]] || fail "wrong node ID in $1"
  [[ "$(property version "$1/meta.properties")" == 1 ]] || fail "not KRaft storage: $1"
}
reject_links() {
  local entry
  for entry in "$1"/* "$1"/.[!.]* "$1"/..?*; do
    [[ ! -L "$entry" ]] || fail "symlinked metadata payload: $entry"
    if [[ -d "$entry" ]]; then reject_links "$entry"; fi
  done
}
safe_path() {
  [[ "$1" =~ ^/[a-zA-Z0-9_./-]+$ && "$1" != "/" && "$1" != */ && "$1" != *//* && "$1" != */../* && "$1" != */./* && "$1" != */.. && "$1" != */. ]] || fail "unsafe mount path: $1"
}
[[ -n "${CLUSTER_ID:-}" && "${NODE_ID:-}" =~ ^[0-9]+$ ]] || fail "missing cluster/node identity"
[[ "$(property process.roles "$BROKER_CONFIG")" == broker ]] || fail "not a broker-only configuration"
[[ "$(property node.id "$BROKER_CONFIG")" == "$NODE_ID" ]] || fail "node/config disagreement"
safe_path "$METADATA_MOUNT"
dest="$METADATA_MOUNT/kafka"
stage="$METADATA_MOUNT/.koperator-metadata-stage"
manifest="$METADATA_MOUNT/.koperator-metadata-migration"
complete="$METADATA_MOUNT/.koperator-metadata-complete"
[[ "$(property metadata.log.dir "$BROKER_CONFIG")" == "$dest" ]] || fail "metadata/config disagreement"
[[ -d "$METADATA_MOUNT" && ! -L "$METADATA_MOUNT" ]] || fail "metadata volume is not mounted"
IFS=',' read -ra mounts <<< "$DATA_MOUNTS"
[[ ${#mounts[@]} -gt 0 ]] || fail "no mounted data directories"
sources=()
formatted=0
directory_ids=("not-a-directory-id")
for mount in "${mounts[@]}"; do
  safe_path "$mount"
  [[ "$mount" != "$METADATA_MOUNT" && "$mount" != "$METADATA_MOUNT/"* && "$METADATA_MOUNT" != "$mount/"* ]] || fail "overlapping metadata/data mounts"
  [[ -d "$mount" && ! -L "$mount" ]] || fail "data volume is not mounted: $mount"
  root="$mount/kafka"
  [[ ! -L "$root" && ! -L "$root/__cluster_metadata-0" ]] || fail "symlinked source"
  if [[ -f "$root/meta.properties" ]]; then
    identity "$root"
    formatted=$((formatted + 1))
    # Older source directories may predate Kafka's directory.id field.
    while IFS= read -r line; do
      [[ "$line" != directory.id=* ]] || directory_ids+=("${line#*=}")
    done < "$root/meta.properties"
  fi
  if [[ -e "$root/__cluster_metadata-0" ]]; then
    [[ -d "$root/__cluster_metadata-0" ]] || fail "source is not a directory"
    identity "$root"
    sources+=("$mount")
  fi
done
[[ ${#sources[@]} -le 1 ]] || fail "multiple active metadata sources"

if [[ -f "$manifest" ]]; then
  [[ ! -L "$manifest" ]] || fail "symlinked migration manifest"
  [[ "$(property cluster.id "$manifest")" == "$CLUSTER_ID" && "$(property node.id "$manifest")" == "$NODE_ID" ]] || fail "migration identity conflict"
  source=$(property source "$manifest")
else
  [[ ! -e "$dest" && ! -e "$stage" && ! -e "$complete" ]] || fail "unowned destination/staging storage"
  source=fresh
  if [[ ${#sources[@]} == 1 ]]; then
    source=${sources[0]}
    [[ -f "$source/kafka/bootstrap.checkpoint" && ! -L "$source/kafka/bootstrap.checkpoint" ]] || fail "source bootstrap.checkpoint is missing or symlinked"
  elif [[ $formatted != 0 ]]; then
    fail "formatted broker has no metadata source; refusing empty recovery"
  elif [[ "${ALLOW_FRESH:-false}" != true ]]; then
    fail "existing broker has no metadata source; refusing empty recovery"
  fi
  printf 'cluster.id=%s\nnode.id=%s\nsource=%s\n' "$CLUSTER_ID" "$NODE_ID" "$source" > "$manifest.pending"
  sync
  mv "$manifest.pending" "$manifest"
  sync
fi

if [[ "$source" != fresh ]]; then
  safe_path "$source"
  backup="$source/.koperator-metadata-backup-$NODE_ID"
fi
if [[ ${#sources[@]} == 1 ]]; then
  [[ "$source" == "${sources[0]}" && ! -e "$complete" ]] || fail "active source conflicts with migration state"
  [[ ! -e "$backup" ]] || fail "both active metadata and backup exist"
fi

if [[ ! -d "$dest" ]]; then
  [[ ! -e "$dest" && ! -e "$complete" ]] || fail "destination missing or not a directory"
  if [[ "$source" != fresh ]]; then
    [[ ${#sources[@]} == 1 ]] || fail "source lost before publishing"
    reject_links "$source/kafka/__cluster_metadata-0"
    # A real cache has a segment or snapshot, not just an empty directory.
    payload=false
    for file in "$source/kafka/__cluster_metadata-0/"*.log "$source/kafka/__cluster_metadata-0/"*.checkpoint; do
      [[ ! -f "$file" ]] || payload=true
    done
    [[ "$payload" == true ]] || fail "empty source metadata log"
  else
    [[ ${#sources[@]} == 0 && $formatted == 0 ]] || fail "fresh migration conflicts with existing storage"
  fi
  mkdir -p "$stage" "$WORK_DIR"
  [[ ! -L "$stage" ]] || fail "symlinked staging directory"
  stage_owner="$stage/.koperator-metadata-stage-owner"
  if [[ ! -e "$stage_owner" ]]; then
    # An interruption before writing the first ownership marker can leave an
    # empty directory or this single pending file, but cannot leave payload.
    for entry in "$stage"/* "$stage"/.[!.]* "$stage"/..?*; do
      if [[ -e "$entry" || -L "$entry" ]]; then
        [[ "$entry" == "$stage_owner.pending" && ! -L "$entry" ]] || fail "unowned staging content"
      fi
    done
    cp "$manifest" "$stage_owner.pending"
    sync
    mv "$stage_owner.pending" "$stage_owner"
    sync
  fi
  [[ ! -L "$stage_owner" ]] || fail "symlinked staging ownership"
  cmp "$manifest" "$stage_owner" || fail "staging belongs to another migration"
  format_config="$WORK_DIR/metadata-format.config"
  trap 'rm -f "$format_config"' EXIT
  # Format only the staging directory. Kafka supplies a NEW directory.id.
  # The old data directory's meta.properties must never be copied.
  while IFS= read -r line || [[ -n "$line" ]]; do
    case "$line" in log.dirs=*|log.dir=*|metadata.log.dir=*) ;; *) printf '%s\n' "$line";; esac
  done < "$BROKER_CONFIG" > "$format_config"
  printf 'log.dirs=%s\nmetadata.log.dir=%s\n' "$stage" "$stage" >> "$format_config"
  if [[ -f "$stage/meta.properties" && ! -f "$stage/bootstrap.checkpoint" ]]; then
    # An interrupted formatter has not yet published anything or touched the source.
    rm "$stage/meta.properties"
  fi
  "$KAFKA_HOME/bin/kafka-storage.sh" format --cluster-id="$CLUSTER_ID" --ignore-formatted -c "$format_config"
  identity "$stage"
  [[ -f "$stage/bootstrap.checkpoint" ]] || fail "formatter did not write bootstrap.checkpoint"
  new_id=$(property directory.id "$stage/meta.properties")
  [[ "$new_id" =~ ^[a-zA-Z0-9_-]{22}$ ]] || fail "Kafka formatter lacks a valid directory.id (Kafka 3.7+ required)"
  for old_id in "${directory_ids[@]}"; do
    [[ "$new_id" != "$old_id" ]] || fail "destination reuses a data directory ID"
  done
  if [[ "$source" != fresh ]]; then
    mkdir -p "$stage/__cluster_metadata-0"
    cp -a "$source/kafka/__cluster_metadata-0/." "$stage/__cluster_metadata-0/"
    cp -a "$source/kafka/bootstrap.checkpoint" "$stage/bootstrap.checkpoint"
  fi
  cp "$manifest" "$stage/.koperator-metadata-owner"
  sync
  mv "$stage" "$dest"
  sync
fi
[[ ! -L "$dest" ]] || fail "symlinked destination"
[[ ! -e "$stage" ]] || fail "both staging and published destination exist"
identity "$dest"
[[ -f "$dest/.koperator-metadata-owner" && ! -L "$dest/.koperator-metadata-owner" ]] || fail "destination ownership missing or symlinked"
cmp "$manifest" "$dest/.koperator-metadata-owner" || fail "destination belongs to another migration"
new_id=$(property directory.id "$dest/meta.properties")
[[ "$new_id" =~ ^[a-zA-Z0-9_-]{22}$ ]] || fail "invalid destination directory ID"
for old_id in "${directory_ids[@]}"; do
  [[ "$new_id" != "$old_id" ]] || fail "destination/data directory ID conflict"
done
[[ -f "$dest/bootstrap.checkpoint" && ! -L "$dest/bootstrap.checkpoint" ]] || fail "destination bootstrap missing or symlinked"
if [[ "$source" != fresh ]]; then
  [[ -d "$dest/__cluster_metadata-0" && ! -L "$dest/__cluster_metadata-0" ]] || fail "published metadata log missing"
  reject_links "$dest/__cluster_metadata-0"
fi

if [[ ! -e "$complete" ]]; then
  if [[ "$source" != fresh ]]; then
    if [[ ${#sources[@]} == 1 ]]; then
      # Same-volume rename takes the old active metadata OUTSIDE all log.dirs.
      # No topic replicas or data-directory meta.properties are moved.
      mv "$source/kafka/__cluster_metadata-0" "$backup"
      sync
    else
      [[ -d "$backup" && ! -L "$backup" ]] || fail "source missing without a retained backup"
    fi
  fi
  cp "$manifest" "$complete.pending"
  sync
  mv "$complete.pending" "$complete"
  sync
fi
[[ -f "$complete" && ! -L "$complete" ]] || fail "completion marker missing or symlinked"
cmp "$manifest" "$complete" || fail "completion identity conflict"
for mount in "${mounts[@]}"; do
  [[ ! -e "$mount/kafka/__cluster_metadata-0" ]] || fail "active metadata remains in log.dirs"
done
echo "KRaft metadata storage ready for broker $NODE_ID"
