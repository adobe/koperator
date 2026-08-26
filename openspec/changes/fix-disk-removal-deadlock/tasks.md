# Tasks: Fix Disk Removal Deadlock

## Task 1: Move `runningBrokers` before `reconcileKafkaPvc` [x]
- **File**: `pkg/resources/kafka/kafka.go`
- **What**: Move the broker pod list query (lines 332-343) to before `reconcileKafkaPvc` call (line 326). Remove the duplicate query at its original location.
- **Details**: The `var brokerPods` / `runningBrokers` block currently runs AFTER `reconcileKafkaPvc`. Move it before. Pass `runningBrokers` to `reconcileKafkaPvc`.

## Task 2: Update `reconcileKafkaPvc` to accept and use `runningBrokers` [x]
- **File**: `pkg/resources/kafka/kafka.go`
- **What**:
  1. Add `runningBrokers map[string]struct{}` parameter to `reconcileKafkaPvc`
  2. At line 1278, before returning "Disk removal pending" error: check if any broker in `brokersDesiredPvcs` has a missing pod AND has a `IsDiskRemoval()` volume state. If so, return nil.
- **Details**: This is the core fix. The check iterates `brokersDesiredPvcs` keys, looks up `runningBrokers`, and if a pod is missing checks the broker's volume states for active disk removal.

## Task 3: Update tests [x]
- **File**: `pkg/resources/kafka/kafka_test.go`
- **What**:
  1. Update all existing callers of `reconcileKafkaPvc` to pass the new `runningBrokers` parameter
  2. Add test case: disk removal pending + broker pod missing → returns nil
  3. Add test case: disk removal pending + all pods present → returns CruiseControlTaskRunning error
- **Details**: The new test cases should set up a KafkaCluster with a broker whose volume state is `GracefulDiskRemovalScheduled`, then call `reconcileKafkaPvc` with/without the broker in `runningBrokers`.

## Task 4: Verify [x]
- Run `go test ./pkg/resources/kafka/...`
- Run `make test` for full suite
- Run `go vet ./...` and `go build ./...`
