---
name: Fix log.dirs disk removal
overview: Identify why removed `additionalDisks` entries are still present in broker `log.dirs` and define a status-aware reconciliation strategy so `log.dirs` is updated once disk removal completes, with unit and e2e coverage.
todos:
  - id: implement-effective-logdirs
    content: Add status-aware helper for effective log.dirs and wire it into config generation in pkg/resources/kafka/configmap.go
    status: completed
  - id: stabilize-unit-tests
    content: Finalize and run unit tests for effective log.dirs behavior in pkg/resources/kafka/configmap_test.go
    status: completed
  - id: validate-e2e-flow
    content: Verify multidisk removal e2e assertions and sequencing in tests/e2e/test_multidisk_removal.go and tests/e2e/koperator_suite_test.go
    status: completed
isProject: false
---

# Fix `log.dirs` After Disk Removal

## Problem Analysis

- Current config generation in [koperator/pkg/resources/kafka/configmap.go](koperator/pkg/resources/kafka/configmap.go) always merges old + new mount paths:
  - `mountPathsMerged, isMountPathRemoved := mergeMountPaths(mountPathsOld, mountPathsNew)`
  - This preserves removed paths indefinitely, even after `GracefulDiskRemovalSucceeded` and PVC deletion.
- Disk-removal lifecycle state is already tracked in [koperator/pkg/resources/kafka/kafka.go](koperator/pkg/resources/kafka/kafka.go) (`GracefulActionState.VolumeStates`) and state semantics are defined in [koperator/api/v1beta1/common_types.go](koperator/api/v1beta1/common_types.go).
- Your new tests indicate the intended behavior: keep removed path while removal/rebalance is active, drop it when state is missing or succeeded.

## Proposed Solution

- Replace unconditional old+new merge for `log.dirs` with a status-aware effective set:
  - Keep all paths currently in spec (`mountPathsNew`).
  - For paths present only in old config (`mountPathsOld - mountPathsNew`), keep **only** if broker volume state for that mount path is active:
    - `CruiseControlVolumeState.IsDiskRemoval()` OR `CruiseControlVolumeState.IsDiskRebalance()`.
  - Drop removed paths when state is absent or `IsDiskRemovalSucceeded()`.
- Implement helper in [koperator/pkg/resources/kafka/configmap.go](koperator/pkg/resources/kafka/configmap.go):
  - `getEffectiveLogDirsMountPaths(mountPathsOld, mountPathsNew, brokerID, kafkaCluster)`
- Use this helper in `getConfigProperties()` when setting `log.dirs`.

## Test Plan

- Unit tests in [koperator/pkg/resources/kafka/configmap_test.go](koperator/pkg/resources/kafka/configmap_test.go):
  - Keep your added `TestGetEffectiveLogDirsMountPaths` and ensure coverage includes:
    - no state -> drop removed path
    - removal/rebalance active -> keep removed path
    - removal succeeded -> drop removed path
- E2E in [koperator/tests/e2e/test_multidisk_removal.go](koperator/tests/e2e/test_multidisk_removal.go):
  - Install multidisk sample, then apply single-disk sample, assert broker configmaps no longer contain removed path.
- Suite wiring in [koperator/tests/e2e/koperator_suite_test.go](koperator/tests/e2e/koperator_suite_test.go) is already aligned.

## Expected Outcome

- During removal in progress, `log.dirs` remains stable and safe for CC workflows.
- After successful completion and cleanup, removed disk paths disappear from `log.dirs`, preventing writes to unintended/root filesystem paths.
