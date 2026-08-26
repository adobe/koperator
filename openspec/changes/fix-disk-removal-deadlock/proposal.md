# Fix: Disk Removal Deadlock During Rolling Upgrade

**Status**: proposed
**Created**: 2026-05-18

## Problem

When a broker's pod is deleted during a rolling upgrade AND a disk removal is pending (`GracefulDiskRemovalScheduled`), the operator enters a deadlock:

1. `reconcileKafkaPvc` blocks the entire reconcile with "Disk removal pending" error
2. `reconcileKafkaPod` is never reached, so the pod is never recreated
3. Cruise Control cannot complete the disk removal because the broker isn't running
4. The cluster is stuck in `ClusterRollingUpgrading` indefinitely

This was observed in production: broker 103 of a 9-broker cluster (`pipeline-kafka`) had its pod deleted at 14:27:33 and was never recreated. The operator looped every ~20s for 10+ minutes with "Disk removal pending".

## Root Cause

In `pkg/resources/kafka/kafka.go`, the main reconcile function processes steps sequentially:

```
Line 326: reconcileKafkaPvc()   ← blocks here with "Disk removal pending"
Line 332: build runningBrokers  ← never reached
Line 448: reconcileKafkaPod()   ← never reached (this creates missing pods)
```

`reconcileKafkaPvc` checks disk removal for ALL brokers. If ANY broker has pending removal, it returns a `CruiseControlTaskRunning` error that aborts the ENTIRE reconcile — including pod creation for brokers whose pods are missing.

The deadlock emerges across reconcile cycles:
- **Cycle N**: PVC check passes (states not set yet) → rolling upgrade deletes pod → returns
- **Cycle N+1**: PVC check sets `GracefulDiskRemovalRequired` → blocks → pod never recreated
- **Cycle N+2...∞**: Same. Deadlock.

## Proposed Fix

**Don't block on disk removal when a broker's pod doesn't exist.**

CC disk removal REQUIRES the broker to be running (it moves partition replicas off the disk). Blocking pod creation while waiting for CC is counterproductive. The disk removal check is re-evaluated every reconcile cycle, so once the pod is back up, the check will correctly block again if still needed.

### Changes

**`pkg/resources/kafka/kafka.go`**:

1. Move the `runningBrokers` map building (lines 332-343) to BEFORE `reconcileKafkaPvc` (line 326)
2. Pass `runningBrokers` to `reconcileKafkaPvc`
3. In `reconcileKafkaPvc` (line 1278), before returning "Disk removal pending" error: check if any broker with pending disk removal has a missing pod. If yes, return `nil` instead.

**`pkg/resources/kafka/kafka_test.go`**:

- Update existing `reconcileKafkaPvc` tests for new signature
- Add test: disk removal pending + broker pod missing → returns `nil`
- Add test: disk removal pending + all pods running → returns error (unchanged behavior)

## Scope

- This fix is purely in the reconcile ordering/blocking logic
- No changes to Cruise Control integration, disk removal flow, or rolling upgrade semantics
- Existing behavior is preserved when all broker pods are running
- Only changes behavior when a broker pod is missing AND disk removal is pending

## Risk

**Low.** The fix only relaxes a blocking condition in a specific scenario (missing pod + pending disk removal) where the current behavior is provably wrong (deadlock). The disk removal check continues to work normally once the pod is recreated.
