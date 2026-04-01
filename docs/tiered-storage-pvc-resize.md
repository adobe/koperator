# Tiered Storage Cache PVC Resize

Kubernetes does not support shrinking a PVC in-place. Because tiered storage cache
data is ephemeral (repopulated from remote storage on broker restart), koperator
implements a **delete-and-recreate** strategy for cache PVC shrinks, coordinated
with the rolling upgrade machinery so only one broker is affected at a time.

---

## Annotations

Two annotations are written on PVC objects to carry state across reconcile cycles.
They survive reconciler restarts, making every step re-entrant.

| Annotation | Value | Written on | Meaning |
|------------|-------|------------|---------|
| `koperator.adobe.com/cache-resize-state` | `pending-deletion` | Old PVC | Being replaced; excluded from pod spec; deleted once broker pod stops |
| `koperator.adobe.com/cache-resize-state` | `replacement` | New PVC | Replacement PVC; rolling upgrade must complete before annotations are stripped |
| `koperator.adobe.com/replaces-pvc` | `<old-pvc-name>` | New PVC | Traceability — records which PVC is being replaced |

---

## Resize flow

### Cycle N — resize detected, pod running

1. The old PVC is annotated `pending-deletion`.
2. A replacement PVC with the new (smaller) size is created and annotated `replacement`. Provisioning starts immediately.
3. The broker's `ConfigurationState` is set to `ConfigOutOfSync` to trigger a rolling restart via `handleRollingUpgrade`.
4. `handleRollingUpgrade` evaluates health gates (replica health, concurrent restart limit, rack awareness). If all pass the broker pod is deleted and the cycle requeues. If any gate fails the state is preserved in PVC annotations and retried next cycle.

### Cycle N+1 — pod is absent

A pod is considered absent when it either does not exist or has a non-nil `DeletionTimestamp` (Terminating). Treating a Terminating pod as absent allows cleanup to start during the pod's Terminating window rather than waiting for it to fully disappear from etcd.

1. The pending-deletion PVC is deleted.
2. A new broker pod is created referencing the replacement PVC. Because provisioning started in cycle N the PVC is likely already `Bound`, minimising startup latency.

### Cycle N+2 — pod is present again

The strip fires as soon as a non-Terminating pod exists for the broker and no pending-deletion PVC remains — the pod does not need to be fully Running.

1. No pending-deletion PVC remains and the replacement PVC exists → resize is complete.
2. The `cache-resize-state` and `replaces-pvc` annotations are stripped from the replacement PVC, which becomes an ordinary PVC from this point forward.

---

## Grow vs shrink

A cache PVC **grow** takes the normal Kubernetes in-place expansion path: the PVC spec is updated with the larger size and Kubernetes expands the volume without a pod restart (requires `allowVolumeExpansion: true` on the StorageClass). No annotations are written and no rolling restart is triggered.

A cache PVC **shrink** uses the delete-and-recreate flow described above. Shrinking is only supported for tiered storage cache volumes — regular Kafka log volumes reject any size decrease.

---

## Properties of this design

| Property | Value |
|----------|-------|
| State survives reconciler crash | Mostly — PVC annotations are durable in etcd; the one non-re-entrant window is between annotating the old PVC and creating the replacement, but `ConfigOutOfSync` set in that cycle persists in broker status so the rolling upgrade still proceeds |
| Atomicity gap | Eliminated — new PVC is created before old is deleted |
| Provisioning overlaps gate evaluation | Yes — new PVC created in cycle N, not N+1 |
| Observable via kubectl | Yes — `kubectl get pvc -o yaml` shows resize state directly |
| ConfigOutOfSync overloading | Reduced — `ConfigOutOfSync` still used, but the *reason* is legible in PVC annotations |
| CC disk rebalance for cache PVCs | Fixed — tiered cache PVCs are explicitly excluded from `GracefulDiskRebalanceRequired` logic |

---

## Sequence diagram

```
Cycle N  (pod UP, resize detected)
  ├─ annotate old PVC: pending-deletion
  ├─ create replacement PVC (provisioning starts)
  ├─ set ConfigOutOfSync
  └─ handleRollingUpgrade
     ├─ [gates fail] → requeue 15s, retry next cycle
     └─ [gates pass] → delete pod → requeue 15s

Cycle N+k  (pod UP, gates failing — any number of cycles)
  └─ ensure ConfigOutOfSync, requeue

Cycle N+k+1  (pod ABSENT — gone or Terminating)
  ├─ delete pending-deletion PVC
  └─ create new pod bound to replacement PVC

Cycle N+k+2  (pod PRESENT — non-Terminating, not necessarily Running)
  └─ strip annotations → replacement PVC becomes ordinary PVC
```
