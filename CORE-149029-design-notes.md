# CORE-149029 — Broker restart loop with admission controllers

Design/investigation notes. Branch `CORE-149029-fix`. Fix committed as `c78708b3`.

## Problem

Kafka broker pods (e.g. `pipeline-kafka-202`) crash-loop in a delete/recreate
cycle every ~8s during rolling upgrade. The already-merged preferred-affinity
fix (`ignorePreferredAffinities`) was deployed, but the loop persisted.

Evidence: `tmp/logs.txt` (koperator log), `tmp/202.yml` (crash-looping pod,
ScaleOps VPA applied), `tmp/202-post.yml` (healthy pod, VPA not applied).

## Root cause

Two independent admission-controller mutations can trigger koperator's rolling
upgrade; the affinity fix only covered one.

1. **Affinities (already fixed):** koperator-emitted preferred affinity lists
   are atomic (no `patchMergeKey`); a webhook appending to them looked like a
   value change and got reverted. Only relevant when koperator itself emits
   preferred terms (e.g. `oneBrokerPerNode: false`). Not the trigger on
   pipeline-kafka.

2. **Resources (the actual trigger):** ScaleOps VPA rewrites the live pod's
   container `requests` after admission (kafka cpu `1`→`392m`; fluent-bit cpu
   `100m`→`11m`, mem `256Mi`→`71420084`). koperator's last-applied annotation
   and freshly-generated desired pod both keep the CR values.

### Why the diff reverts it

`patch.DefaultPatchMaker.Calculate` (k8s-objectmatcher v1.8.0) uses
`CreateThreeWayMergePatch(original, modified, current)`. Per apimachinery
`strategicpatch/patch.go:2070-2078`:

```
deltaMap     = diffMaps(current,  modified, {IgnoreDeletions: true})   // changes + additions
deletionsMap = diffMaps(original, modified, {IgnoreChangesAndAdditions: true})
patch        = merge(deletionsMap, deltaMap)
```

The change-detection half (`deltaMap`) diffs **current (live pod) vs modified
(desired)**. `IgnoreDeletions:true` preserves webhook *additions* (annotations,
labels, extra `podAffinity`), which is why those don't loop — but a **value
change** to a field koperator declares (`requests.cpu`) is reverted → non-empty
patch → rolling upgrade → pod deleted → ScaleOps re-mutates → loop.

### Why the last-applied annotation doesn't help

`original` (last-applied) is only consulted for the *deletions* half. The
changes half never looks at it. So the annotation protects against foreign
*additions* and governs *deletions*, but cannot stop koperator reverting a
value change to a field it still declares. Same rule as `kubectl apply` vs
HPA/VPA: an autoscaler can own a field only if you omit it from your applied
config.

## Fix — intent-aware two-way merge

Trigger a rolling upgrade **iff koperator's own desired spec changed since it
last applied it**: `diff(original, modified)`, ignoring the live pod entirely.

- External mutation, CR unchanged → `original == modified` → no restart; live
  value (VPA tuning, webhook affinity) preserved.
- CR edit (resources or soft affinity) → `original != modified` → one restart,
  new pod gets the CR value, then settles (ScaleOps re-tunes; next reconcile
  `original == modified` again).

The patch is only a boolean change-signal (+ logging); koperator recreates the
pod rather than patching it in place, so two-way `diff(original, modified)` is
exactly the right question.

This **subsumes** the affinity fix and is strictly better: the old strip
approach also *swallowed intentional soft-affinity edits made via the CR*; the
intent diff propagates them.

### Implementation (commit c78708b3)

- `pkg/resources/kafka/util.go`: new `podSpecIntentChanged(currentPod,
  desiredPod)` — `GetOriginalConfiguration` (last-applied) vs marshaled desired
  via `strategicpatch.CreateTwoWayMergePatch`, plus a `$setElementOrder`
  re-check. Removed `ignorePreferredAffinities()`/`deletePreferredAffinities()`.
- `pkg/resources/kafka/kafka.go`: `handleRollingUpgrade` switch uses
  `!intentChanged` in place of `patchResult.IsEmpty()`. The
  `isPodTainted(currentPod)` case is unchanged and still ordered *first*.
- `pkg/resources/kafka/util_test.go`: `TestPodSpecIntentChanged` (5 rows) and
  `TestParkedBrokerRestartsIndependentOfIntent`.

Verified: `go build ./...`, `go vet`, `gofmt -l`, full package tests — all
clean/green.

## Shredder park flow is unaffected

`shredder.ethos.adobe.net/upgrade-status=parked` restart works via
`isPodTainted(currentPod)` → `Spec.TaintedBrokersSelector` (a label lookup on
the *live* pod), which is independent of the diff and ordered before the intent
check. Covered by `TestParkedBrokerRestartsIndependentOfIntent`. (Config
requirement: `TaintedBrokersSelector` must actually select that label.)

## Why this matches built-in controllers

Every built-in workload controller decides from recorded intent, never from a
live-pod spec diff — which is why they don't fight admission webhooks:

| Controller     | Intent signal                        | Compared against          |
|----------------|--------------------------------------|---------------------------|
| ReplicaSet     | replica count                        | number of owned pods      |
| Deployment     | `pod-template-hash` (from template)  | each RS's stored template |
| StatefulSet    | `controller-revision-hash` (template)| pod's revision label      |
| koperator (new)| last-applied annotation              | freshly generated desired |

koperator is a hand-rolled StatefulSet for Kafka; the bug was that it diffed the
live pod instead of recorded intent. The fix is koperator's analog of the
StatefulSet revision-hash check.

## Security tradeoff (accepted)

The old live-pod diff *incidentally* reverted out-of-band tampering to
live-mutable fields (e.g. a `kubectl patch` image swap). The intent diff no
longer does — consistent with RS/Deployment/StatefulSet, none of which revert
live-pod drift. This was never a reliable control; the real defenses are RBAC
least-privilege on `pods` write/exec, admission policy on UPDATE (registry
allowlist + image signature verification), digest pinning, and audit/detection.
If operator-level tamper detection is wanted, it should be an explicit
detect/alert feature, not coupled to the reconcile diff (that coupling is what
caused the loop).

## Status / follow-ups

- Committed `c78708b3` on `CORE-149029-fix`. **Not pushed** — `origin`
  (`git@github-public:adobe/koperator.git`) needs the `github-public` SSH
  identity, which is not loaded in the non-interactive session; run the push
  interactively.
- `make lint` blocked by an **environmental** toolchain mismatch (pinned
  golangci-lint 2.12.2 built with go1.25.3 rejects the go1.26.0 target) — fails
  repo-wide, unrelated to this diff. Run in CI or with a go1.26-built linter.
- Toleration merge (`kafka.go:947-958`) likely redundant now; candidate for a
  follow-up removal + verification.
