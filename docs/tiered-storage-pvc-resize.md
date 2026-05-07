# Tiered Storage Cache PVC Resize

Kubernetes does not support shrinking a PVC in-place. Because tiered storage cache
data is ephemeral (repopulated from remote storage on broker restart), koperator
implements a **delete-and-recreate** strategy for cache PVC shrinks, coordinated
with the rolling upgrade machinery so only one broker is affected at a time.

---

## State tracking

Resize state is stored in the `KafkaCluster` CR status under
`status.brokersState[<brokerId>].tieredCacheVolumes`, keyed by mount path.
This keeps the KafkaCluster CR the single source of truth for all in-flight
broker operations and avoids a second, parallel state store on PVC objects.

| Field | Value | Meaning |
|-------|-------|---------|
| `status.brokersState[N].tieredCacheVolumes[<mountPath>]` | `active` | The mount path is a tiered storage cache volume. No resize is in progress. |
| `status.brokersState[N].tieredCacheVolumes[<mountPath>]` | `pending-deletion` | A resize is in flight for this mount path. The old PVC (larger size) is waiting to be deleted once the broker pod stops; the replacement PVC (desired smaller size) has already been created. |
| *(absent)* | — | The mount path is not a tiered storage cache volume (or the PVC has been removed). |

The entry transitions from `pending-deletion` back to `active` once the old PVC
has been deleted and the resize is complete. An absent entry means no cache PVC
exists at that path.

Two PVC annotations that describe what a PVC **is** (not operational state) are
always present on cache PVCs:

| Annotation | Value | Purpose |
|------------|-------|---------|
| `mountPath` | `<path>` | Used throughout reconcile logic to match PVCs to storage configs |
| `tieredStorageCache` | `"true"` | Identifies cache PVCs for special handling: skipped from `log.dirs` and CC capacity config |

---

## Resize flow

### Cycle N — resize detected, pod running

1. `status.brokersState[N].tieredCacheVolumes[<mountPath>]` is set to `pending-deletion`
   in the KafkaCluster CR status. This is the durable record that a resize is in flight.
2. A replacement PVC with the new (smaller) size is created. Provisioning starts immediately.
3. The broker's `ConfigurationState` is set to `ConfigOutOfSync` to trigger a rolling restart
   via `handleRollingUpgrade`.
4. `handleRollingUpgrade` evaluates health gates (replica health, concurrent restart limit,
   rack awareness). If all pass the broker pod is deleted and the cycle requeues. If any gate
   fails the state persists in the CR and is retried next cycle.

### Cycle N+1 — pod is absent

A pod is considered absent when it either does not exist or has a non-nil
`DeletionTimestamp` (Terminating). Treating a Terminating pod as absent allows
cleanup to start during the pod's Terminating window rather than waiting for it
to fully disappear from etcd.

1. The old PVC (the one whose size differs from the desired size at that mount path)
   is deleted.
2. The `tieredCacheVolumes` entry for that mount path is set to `active` in the CR status.
3. A new broker pod is created referencing the replacement PVC. Because provisioning
   started in cycle N the PVC is likely already `Bound`, minimising startup latency.

### Cycle N+2 — pod is present again

1. No `pending-deletion` entry remains for the mount path → resize is complete.
2. The replacement PVC is now an ordinary cache PVC with `tieredCacheVolumes[<mountPath>] = active`.

---

## Grow vs shrink

A cache PVC **grow** takes the normal Kubernetes in-place expansion path: the PVC
spec is updated with the larger size and Kubernetes expands the volume without a
pod restart (requires `allowVolumeExpansion: true` on the StorageClass). No
`tieredCacheVolumes` state change is made and no rolling restart is triggered.

A cache PVC **shrink** uses the delete-and-recreate flow described above.
Shrinking is only supported for tiered storage cache volumes — regular Kafka log
volumes reject any size decrease with an error.

---

## Properties of this design

| Property | Value |
|----------|-------|
| State survives reconciler crash | Yes — `tieredCacheVolumes` is written to the KafkaCluster CR (etcd) before the replacement PVC is created; every step is re-entrant |
| Single source of truth | Yes — all broker state (configuration, graceful actions, cache resize) lives in `status.brokersState` |
| Atomicity gap | Eliminated — replacement PVC is created before old is deleted |
| Provisioning overlaps gate evaluation | Yes — replacement PVC created in cycle N, not N+1 |
| Observable via kubectl | Yes — `kubectl get kafkacluster <name> -o jsonpath='{.status.brokersState}'` shows resize state; `pending-deletion` entries indicate an in-flight resize |
| CC disk rebalance for cache PVCs | Excluded — tiered cache PVCs are explicitly skipped from `GracefulDiskRebalanceRequired` and CC capacity config |
| `log.dirs` for cache PVCs | Excluded — `generateStorageConfig` skips volumes with `TieredStorageCache: true` |

---

## Sequence diagram

```
Cycle N  (pod UP, resize detected)
  ├─ set tieredCacheVolumes[mountPath] = pending-deletion in CR status
  ├─ create replacement PVC (provisioning starts)
  ├─ set ConfigOutOfSync
  └─ handleRollingUpgrade
     ├─ [gates fail] → requeue 15s, retry next cycle
     └─ [gates pass] → delete pod → requeue 15s

Cycle N+k  (pod UP, gates failing — any number of cycles)
  └─ ensure ConfigOutOfSync, requeue

Cycle N+k+1  (pod ABSENT — gone or Terminating)
  ├─ delete old PVC (identified as the PVC at mountPath whose size ≠ desired)
  ├─ set tieredCacheVolumes[mountPath] = active in CR status
  └─ create new pod bound to replacement PVC

Cycle N+k+2  (pod PRESENT — non-Terminating, not necessarily Running)
  └─ tieredCacheVolumes[mountPath] = active → resize complete, no further action
```
