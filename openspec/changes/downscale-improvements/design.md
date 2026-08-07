## Context

The koperator manages Kafka cluster lifecycle via a set of reconcilers. Downscale (broker removal) is handled in two phases:

1. **KafkaCluster reconciler** (`pkg/resources/kafka/kafka.go`): Detects brokers removed from spec. Collects all such brokers in one pass and sets them all to `GracefulDownscaleRequired` via a single atomic `UpdateBrokerStatus` call.

2. **CruiseControlTask reconciler** (`controllers/cruisecontroltask_controller.go`): Picks up brokers in `GracefulDownscaleRequired` state and submits CC operations. The `add_broker` path already batches all pending brokers into one CC operation. The `remove_broker` path does not — it picks only the first task and breaks.

3. **External listener reconcilers** (`pkg/resources/envoy/`): Gate broker inclusion in envoy config on `ShouldIncludeBroker()`. This function returns `false` when `brokerConfig == nil` (broker not in spec), causing draining brokers to vanish from envoy listener config immediately upon spec removal. Note: `pkg/resources/contouringress` and `pkg/resources/nodeportexternalaccess` iterate `Spec.Brokers` directly and are not affected by this fix (see Non-Goals).

**Key invariant:** `GetActiveTasksByOp(OperationRemoveBroker)` only returns brokers in `GracefulDownscaleRequired` state (via `IsRequired()` → `IsRequiredState()`). Brokers already `Scheduled` or `Running` are not returned. Because the KafkaCluster reconciler transitions all removed brokers atomically, all brokers from a single manifest apply are guaranteed to be in `Required` state simultaneously when the CC task reconciler fires.

## Goals / Non-Goals

**Goals:**
- All broker IDs removed in a single manifest apply are submitted as one CC `remove_broker` operation.
- Brokers removed from spec remain in external listener config until `GracefulDownscaleSucceeded`.
- Brokers stuck in `GracefulDownscaleCompletedWithError` or `GracefulDownscalePaused` remain in external listener config (broker still holds data; manual investigation needed).

**Non-Goals:**
- Guaranteed single-operation batching across multiple sequential manifest applies (best-effort; second apply may produce a second CC operation if first batch is already `Scheduled`).
- Changes to CRD schema or the CC REST API.
- KRaft controller-only node handling (controller-only nodes skip CC graceful downscale entirely; unchanged).

## Decisions

### D1: Mirror `addBrokers` pattern for `removeBrokers`

The `addBrokers` helper (lines 366-368) already accepts `[]string` and submits one CC operation. The `removeBroker` helper (lines 370-372) takes a single `string`.

**Decision:** Rename `removeBroker` → `removeBrokers`, change signature to `[]string`, and replace the early-break loop with the collect-all pattern.

**Why not a separate aggregation layer?** The batching boundary is already correct — `GetActiveTasksByOp` returns exactly the set to batch. No new abstraction needed.

### D2: Fix `ShouldIncludeBroker` for envoy callers that enumerate status∪spec

`ShouldIncludeBroker` is called by the envoy external listener reconcilers (configmap, service, deployment). When `brokerConfig == nil`, the function currently falls through to `return false`.

**Real precondition for the fix:** The fallback only fires for reconcilers that enumerate brokers via `GetBrokerIdsFromStatusAndSpec` (status∪spec union), which means removed brokers are still visited after spec removal. Reconcilers that iterate `Spec.Brokers` directly (Contour, NodePort) never pass a removed broker to `ShouldIncludeBroker` in the first place — the fix is invisible to them. Any future listener reconciler must use `GetBrokerIdsFromStatusAndSpec` to benefit automatically; iterating `Spec.Brokers` silently bypasses this protection.

**Decision:** Add a fallback block for `brokerConfig == nil`: check the broker's `CruiseControlState` in status. If `IsDownscale() && !IsSucceeded()` and the broker has the requested `ingressConfigName` in its `ExternalListenerConfigNames`, return `true`.

**Why `ExternalListenerConfigNames` check?** A broker may have been associated with a specific ingress config. Re-using the persisted `ExternalListenerConfigNames` (set when the pod was created, never cleared until `GracefulDownscaleSucceeded`) ensures we only retain the broker for the listener configs it actually served.

**Why `IsDownscale() && !IsSucceeded()` instead of an explicit state list?**
`IsDownscale()` covers all 6 downscale states. Excluding `IsSucceeded()` retains the broker in all non-terminal states, including `CompletedWithError` and `Paused`, which is the desired behavior for manual investigation. If new downscale states are added to the enum in future, they're covered automatically.

**Alternatives rejected:**
- Fix each envoy caller individually: more code, same logic duplicated.
- Add a new function: unnecessary indirection; `ShouldIncludeBroker` is the right seam for envoy callers.

### D3: Task order to satisfy CI (green at every commit)

The original plan placed failing tests before implementation. With CI gating on green builds, the order must be:

```
Commit 1: Unit test for createCCOperation (passes immediately — tests downstream, not the wrapper)
Commit 2: Implement removeBrokers + integration test (both green together)
Commit 3: Implement ShouldIncludeBroker fix + unit test (both green together)
Commit 4: E2E test + sample manifest
```

## Risks / Trade-offs

**[Race: second manifest apply before first batch starts]** → If a user applies a second spec change (removing more brokers) before the first CC operation is created, those new brokers will be included in the same batch (still in `Required` state). If the first CC operation is already `Scheduled`, new brokers get a separate operation. Acceptable; same behavior as `add_broker`.

**[CompletedWithError brokers stay in envoy indefinitely]** → A broker that fails CC draining stays in external listener config. This is intentional (data is still present, clients need connectivity), but operators must monitor and manually recover. No change from current behavior for the envoy side — previously, the broker would have been dropped from envoy even with data present, which was worse.

**[ExternalListenerConfigNames populated assumption]** → The fix assumes `ExternalListenerConfigNames` is non-empty for any broker that was ever reconciled. This field is set on pod creation and never cleared. Clusters created before this field existed would not benefit from the fix for those brokers, but all newly created or recently reconciled brokers are covered.

## Migration Plan

No migration required. Both fixes are backwards-compatible:
- `removeBrokers` produces the same CC API calls as `removeBroker` for single-broker downscales.
- `ShouldIncludeBroker` only changes behavior for brokers with `brokerConfig == nil` (already removed from spec); existing behavior for in-spec brokers is unchanged.

Rollback: revert the two commits. No persistent state is affected.

## Open Questions

None — all decisions resolved during exploration.
