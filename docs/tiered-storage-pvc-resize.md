# Tiered Storage Cache PVC Resize: Reconciliation Flow

This document explains how koperator handles resizing (shrinking) a tiered storage
cache PVC, why the flow is more involved than a regular PVC update, and the
design decisions behind the current implementation.

---

## Background: why cache PVCs are different

Kubernetes does not allow shrinking a PVC — the only supported in-place change is
growing it. For regular Kafka log volumes koperator enforces the same constraint
(`isDesiredStorageValueInvalid` returns an error on any size decrease).

Tiered storage cache volumes are different: the data they hold is ephemeral. When a
broker restarts it will repopulate the cache from remote storage. Losing the cache
does not cause data loss; it only causes a temporary increase in read latency.
Because of this koperator implements a **delete-and-recreate** strategy for cache
PVC shrinks — but it must do so safely, coordinating with the rolling upgrade
machinery so that only one broker is down at a time and cluster health gates are
respected.

---

## Components involved

| Component | File | Role |
|-----------|------|------|
| `reconcileKafkaPvc` | `pkg/resources/kafka/kafka.go` | Creates / updates / deletes PVCs |
| `reconcileKafkaPod` | `pkg/resources/kafka/kafka.go` | Creates pods or hands off to rolling upgrade |
| `handleRollingUpgrade` | `pkg/resources/kafka/kafka.go` | Deletes a pod after checking health gates |
| `getCreatedPvcForBroker` | `pkg/resources/kafka/kafka.go` | Looks up which PVCs exist for a broker and feeds them into the pod spec |
| `generateDataVolumeAndVolumeMount` | `pkg/resources/kafka/pod.go` | Translates PVC list → pod Volume + VolumeMount entries using `ClaimName` |

---

## PVC annotations used during resize

Two annotations are written directly on PVC objects to carry state through the
resize. Because annotations live on the Kubernetes object they survive reconciler
restarts, making every step re-entrant.

| Annotation | Value | Written on | Meaning |
|------------|-------|------------|---------|
| `koperator.adobe.com/cache-resize-state` | `pending-deletion` | Old PVC | This PVC is being replaced; skip it in pod spec generation and delete it once the broker pod stops |
| `koperator.adobe.com/cache-resize-state` | `replacement` | New PVC | This is the replacement PVC; defer to the rolling upgrade before treating it as normal |
| `koperator.adobe.com/replaces-pvc` | `<old-pvc-name>` | New PVC | Traceability — records which PVC is being replaced |

---

## Reconciliation loop order

Within a single `Reconcile()` call the kafka sub-reconciler runs in this order:

```
1. Services
2. PodDisruptionBudgets
3. reconcileKafkaPodDelete          ← graceful downscale via Cruise Control
4. Listener statuses / PKI
5. reconcileKafkaPvc                ← ALL PVC work happens here, before any pod work
6. Discover running pods + bound PVCs
7. reorderBrokers                   ← priority sort
8. FOR each broker:
   a. ConfigMap
   b. per-broker Service
   c. reconcileKafkaPod             ← pod create OR handleRollingUpgrade
   d. reconcilePerBrokerDynamicConfig
9. reconcileClusterWideDynamicConfig
```

PVC reconciliation always runs before pod reconciliation. The new PVC is therefore
created (and starts provisioning) before `handleRollingUpgrade` is evaluated for
the same broker in the same cycle.

---

## Full flow: shrinking a tiered storage cache PVC

### Cycle N — resize detected (pod running)

**`reconcileKafkaPvc` — per-broker setup**

```
r.List(brokerPodList)
brokerPodExists = true

No pending-deletion PVCs exist yet  →  no cleanup
No replacement PVCs exist yet       →  no resize-complete strip
effectivePvcCount == len(desiredPvcs) →  no CC disk removal triggered
```

**`reconcileKafkaPvc` — per-desired-PVC loop**

```
CheckIfObjectUpdated: currentSize > desiredSize  →  enters resize branch

  1. Annotate old PVC:
       koperator.adobe.com/cache-resize-state: pending-deletion
     (r.Update — durable immediately in etcd)

  2. Create replacement PVC with desiredSize:
       koperator.adobe.com/cache-resize-state: replacement
       koperator.adobe.com/replaces-pvc: <old-pvc-name>
     (r.Create — provisioning starts now, in parallel with gate evaluation)

  3. Set broker ConfigurationState = ConfigOutOfSync
     (triggers handleRollingUpgrade on every cycle until pod restarts)

  continue
```

**`reconcileKafkaPod` — same cycle**

`handleRollingUpgrade` sees `ConfigOutOfSync`, evaluates gates:

| Gate | Required to pass |
|------|-----------------|
| Pod count | All expected pods exist |
| Concurrent restart limit | Terminating/Pending pods < `ConcurrentBrokerRestartCountPerRack` |
| Rack awareness | Only pods from same AZ as restarting pods |
| Replica health | `offline + out-of-sync < FailureThreshold` |

If all pass → **broker pod is deleted** → requeue 15 s.
If any fail → requeue 15 s, try again next cycle (state is fully durable via PVC annotations).

---

### Between cycles (rolling upgrade gates blocking)

Each reconcile cycle sees:

```
reconcileKafkaPvc:
  brokerPodExists = true
  No pending-deletion PVCs → no cleanup
  Replacement PVC exists, no pending-deletion → resize-complete strip
    … but pod is still up → strip does NOT fire (pod must be up AND no pending-deletion)
  alreadyCreated loop: skips pending-deletion PVC, finds replacement PVC
  replacement PVC guard: ensure ConfigOutOfSync, continue

reconcileKafkaPod:
  handleRollingUpgrade re-evaluates gates → requeue if blocked
```

State is preserved entirely in PVC annotations — no in-memory or status-field
bookkeeping required. Reconciler can crash and restart at any point.

---

### Cycle N+1 — pod is gone

**`reconcileKafkaPvc` — per-broker setup**

```
r.List(brokerPodList)
brokerPodExists = false

Cleanup loop: finds PVC with pending-deletion annotation
  → r.Delete(oldPvc)   ← safe now, broker is not running
  → r.List(pvcList)    ← re-list; only replacement PVC remains
```

**`getCreatedPvcForBroker` (called later to build pod spec)**

Filters out any PVC with `cache-resize-state: pending-deletion` before matching
mount paths. Returns the replacement PVC for the mount path. The pod spec is built
with `ClaimName` pointing to the new PVC.

**`reconcileKafkaPod`**

No pod exists → creates new pod referencing the replacement PVC. Kubernetes holds
the pod in `Pending` until the replacement PVC reaches `Bound` phase.

Because provisioning started in cycle N (when the replacement PVC was created),
the PVC is likely already `Bound` by now, minimising pod startup latency.

---

### Cycle N+2 — pod is running again

**`reconcileKafkaPvc` — resize-complete strip**

```
brokerPodExists = true
No pending-deletion PVCs (deleted in N+1)
Replacement PVC exists → resize complete → strip annotations:
  delete koperator.adobe.com/cache-resize-state
  delete koperator.adobe.com/replaces-pvc
  r.Update(replacementPvc)
```

The PVC is now an ordinary PVC. Subsequent reconciles treat it normally.

---

## Sequence diagram

```
Cycle N  (pod UP, resize detected)
  reconcileKafkaPvc
    ├─ r.Update(oldPvc)   annotate pending-deletion
    ├─ r.Create(newPvc)   replacement PVC, provisioning starts
    └─ ConfigOutOfSync set
  reconcileKafkaPod
    └─ handleRollingUpgrade
       ├─ [gates fail] → requeue 15s
       └─ [gates pass] → delete pod → requeue 15s

Cycle N+k  (pod UP, gates failing — any number of cycles)
  reconcileKafkaPvc
    └─ replacement PVC guard: ensure ConfigOutOfSync, continue
  reconcileKafkaPod
    └─ handleRollingUpgrade → requeue

Cycle N+k+1  (pod GONE)
  reconcileKafkaPvc
    ├─ delete pending-deletion PVC
    └─ re-list: only replacement PVC remains
  reconcileKafkaPod
    └─ no pod → create pod (ClaimName = replacement PVC)
    └─ pod Pending until PVC bound (likely already bound)

Cycle N+k+2  (pod RUNNING)
  reconcileKafkaPvc
    └─ no pending-deletion PVC + replacement PVC exists → strip annotations
    └─ replacement PVC becomes ordinary PVC
```

---

## Properties of this design

| Property | Value |
|----------|-------|
| State survives reconciler crash | Yes — PVC annotations are durable in etcd |
| Idempotent re-entry | Yes — each phase checks annotations and resumes correctly |
| Atomicity gap | Eliminated — new PVC is created before old is deleted |
| Provisioning overlaps gate evaluation | Yes — new PVC created in cycle N, not N+1 |
| Observable via kubectl | Yes — `kubectl get pvc -o yaml` shows resize state directly |
| ConfigOutOfSync overloading | Reduced — `ConfigOutOfSync` still used, but the *reason* is legible in PVC annotations |
| CC disk rebalance for cache PVCs | Fixed — tiered cache PVCs are explicitly excluded from `GracefulDiskRebalanceRequired` logic |

---

## Interaction with disk removal detection

`reconcileKafkaPvc` detects regular disk removal by comparing PVC counts:

```go
if effectivePvcCount > len(desiredPvcs) { handleDiskRemoval(...) }
```

The count uses `effectivePvcCount` which **excludes replacement PVCs**. During a
resize the old (pending-deletion) + new (replacement) PVCs temporarily co-exist for
the same mount path. Without this exclusion the count check would incorrectly
trigger a Cruise Control disk-removal operation.

Inside `handleDiskRemoval` itself, the `pending-deletion` PVC is also safe: its
mount path still matches the desired spec, so `foundInDesired = true` and it is
skipped by the disk-removal state machine.

---

## Interaction with GracefulDiskRebalanceRequired

When a new PVC first becomes `Bound`, koperator normally sets
`GracefulDiskRebalanceRequired` to ask Cruise Control to rebalance data across
the broker's disks. This is correct for Kafka log volumes but wrong for tiered
storage cache volumes — CC must not account for ephemeral cache storage.

The `alreadyCreated` loop now explicitly skips `GracefulDiskRebalanceRequired` for
any PVC annotated `tieredStorageCache: true`, including the replacement PVC.

---

## Known limitations

### ConfigOutOfSync still shared with config changes

`ConfigOutOfSync` is set to trigger the rolling upgrade. It is the same bit used
for Kafka property changes. An observer cannot distinguish "resize pending" from
"config change" by status alone — the PVC annotations must be inspected.

### Concurrent resize + complete storage-config removal

If the tiered storage cache storage config is removed entirely from the spec while
a resize is in progress, both the `pending-deletion` and `replacement` PVCs have a
mount path that is no longer present in the desired spec. Without special handling
this would route them into the Cruise Control `remove_disks` path — which fails with
"log dir not found" because CC has no knowledge of ephemeral cache paths — leaving
the operator stuck in a 20-second requeue loop.

koperator handles this as follows:

**Pod DOWN**: both the `pending-deletion` and the orphaned `replacement` PVC are
deleted directly in `reconcileKafkaPvc` (bypassing Cruise Control). The operator
proceeds normally on the next cycle.

**Pod UP**: koperator sets `ConfigOutOfSync` to trigger a rolling restart via
`handleRollingUpgrade`, then skips `handleDiskRemoval` for this broker entirely
(`continue` in the broker loop). Once the pod stops, the next cycle falls into the
Pod DOWN path above and cleans up both PVCs.
