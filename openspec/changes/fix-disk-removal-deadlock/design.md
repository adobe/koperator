# Design: Fix Disk Removal Deadlock

## Architecture Context

The koperator reconcile loop in `pkg/resources/kafka/kafka.go` (main `Reconcile` function) runs these steps sequentially:

```
reconcileKafkaPodDelete()     // Line 265 - delete pods removed from spec
    ↓
reconcileKafkaPvc()           // Line 326 - PVC lifecycle (create, resize, disk removal)
    ↓  ← BLOCKS HERE when disk removal pending
build runningBrokers map      // Line 332 - query pod list
    ↓
reconcileKafkaPod()           // Line 448 - per-broker pod create/update/rolling upgrade
```

When `reconcileKafkaPvc` returns an error, all subsequent steps are skipped.

Inside `reconcileKafkaPvc`:
- Iterates ALL brokers' PVCs
- Calls `handleDiskRemoval()` when existing PVCs > desired PVCs
- `handleDiskRemoval` sets `waitForDiskRemovalToFinish = true` for any non-succeeded removal state
- At the end, if `waitForDiskRemovalToFinish` → returns `CruiseControlTaskRunning` error

Inside `reconcileKafkaPod`:
- When `len(podList.Items) == 0` → creates pod (line 831-836)
- When `len(podList.Items) == 1` → handles rolling upgrade via `handleRollingUpgrade()`

## Design Decision

### Where to put the check

**Option A**: Inside `handleDiskRemoval` — skip `waitForDiskRemovalToFinish = true` per-broker if pod missing.
**Option B**: At the end of `reconcileKafkaPvc` — override the error if any broker with removal has missing pod.
**Option C**: In the main reconcile — catch the error and decide whether to proceed.

**Chosen: Option B.** Reasons:
- Minimal change surface (only the blocking decision at line 1278)
- `handleDiskRemoval` still correctly tracks state and logs — no behavior change inside it
- The main reconcile doesn't need to understand PVC internals
- Easy to test: one function, one new parameter

### What data is needed

`reconcileKafkaPvc` needs to know which broker pods exist. The `runningBrokers` map (currently built at line 332) provides this. Move it earlier and pass it in.

### Behavioral change

| Scenario | Current | After Fix |
|---|---|---|
| Disk removal pending, all pods running | Block (error) | Block (error) — unchanged |
| Disk removal pending, broker pod missing | Block (error) — DEADLOCK | Allow (nil) — pod gets created |
| No disk removal pending | Allow (nil) | Allow (nil) — unchanged |

## Key Code Paths

### `handleDiskRemoval` (line 1285-1339)

```
for each existing PVC not in desired:
    if volumeState not found → continue (removal done)
    if IsDiskRemovalSucceeded → delete PVC, delete status
    if IsDiskRemoval → waitForDiskRemovalToFinish = true    ← these are the blocking states
    if IsDiskRebalance → waitForDiskRemovalToFinish = true   ← (rebalance before removal)
    default → mark GracefulDiskRemovalRequired, wait = true  ← initial marking
return waitForDiskRemovalToFinish
```

### `reconcileKafkaPvc` blocking (line 1278-1280)

```go
if waitForDiskRemovalToFinish {
    return errorfactory.New(CruiseControlTaskRunning{}, "Disk removal pending", ...)
}
```

The fix adds a check before this return:
```go
if waitForDiskRemovalToFinish {
    // Check if any broker with pending removal has a missing pod
    for brokerId := range brokersDesiredPvcs {
        if _, podExists := runningBrokers[brokerId]; !podExists {
            if state has IsDiskRemoval volume → return nil
        }
    }
    return error  // all relevant pods exist, block normally
}
```

## Edge Cases

1. **Multiple brokers with missing pods**: Still returns nil. All missing pods will be created on this reconcile cycle.

2. **Broker pod missing but NO disk removal for that broker**: `runningBrokers` missing + no IsDiskRemoval volume state → doesn't trigger the override. The error is still returned. This is intentional — we only bypass when the deadlock condition is present.

3. **Pod deleted between runningBrokers check and reconcileKafkaPod**: Possible but harmless — `reconcileKafkaPod` re-queries the pod list per broker (line 826).

4. **Disk removal completes while pod is being created**: The next reconcile cycle will see `IsDiskRemovalSucceeded` and clean up the PVC. No conflict.
