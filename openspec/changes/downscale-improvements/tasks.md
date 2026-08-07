## 1. Batched broker removal — unit test (passes immediately)

- [x] 1.1 Add multi-broker test case to `TestCreateCCOperation` in `controllers/cruisecontroltask_controller_test.go` (after line 345): `operationType: OperationRemoveBroker`, `brokerIDs: []string{"1","2","3"}`, assert params contain `"1,2,3"` for `ParamBrokerID`
- [x] 1.2 Commit: `test: add unit test for batched remove_broker CC operation params`

## 2. Batched broker removal — implementation + integration test

- [x] 2.1 Rename `removeBroker` → `removeBrokers` in `controllers/cruisecontroltask_controller.go` (line 370), change `brokerID string` parameter to `brokerIDs []string`
- [x] 2.2 Replace the early-break loop (lines 173-186) with collect-all pattern: gather all broker IDs from `GetActiveTasksByOp(OperationRemoveBroker)`, call `removeBrokers`, set `CruiseControlOperationRef` and `StateScheduled` on all tasks
- [x] 2.3 Add integration test `When("multiple brokers are removed", ...)` to `controllers/tests/cruisecontroltask_controller_test.go` (after line 458): set brokers "1" and "2" to `GracefulDownscaleRequired`, assert exactly 1 `CruiseControlOperation` created, both brokers reference same operation, both transition to `GracefulDownscaleScheduled`
- [x] 2.4 Run `go test ./controllers/ -run TestCreateCCOperation` and `go test ./controllers/tests/ -run CruiseControlTaskReconciler` — all green
- [ ] 2.5 Commit: `feat: batch remove_broker operations into single CruiseControl task`

## 3. Draining broker listener retention — implementation + unit test

- [x] 3.1 Add fallback block to `ShouldIncludeBroker` in `pkg/util/util.go` (after line 284): when `brokerConfig == nil`, look up `brokerState` in `status.BrokersState`; if `ccState.IsDownscale() && !ccState.IsSucceeded()` and `StringSliceContains(brokerState.ExternalListenerConfigNames, ingressConfigName)`, return `true`
- [x] 3.2 Add unit test cases to `pkg/util/util_test.go` for `ShouldIncludeBroker` with `brokerConfig=nil`: all 5 active downscale states return `true`, `GracefulDownscaleSucceeded` returns `false`, missing broker state returns `false`
- [x] 3.3 Run `go test ./pkg/util/ -run TestShouldIncludeBroker` — all green
- [x] 3.4 Commit: `fix: retain draining brokers in external listener config until CruiseControl completes`

## 4. E2E test + sample manifest

- [x] 4.1 Create `config/samples/simplekafkacluster_5broker.yaml`: copy `simplekafkacluster.yaml`, add brokers 3 and 4, adjust `cruise.control.metrics.topic.replication.factor=2` and `min.insync.replicas=2`
- [x] 4.2 Add `testBatchedBrokerRemoval()` to `tests/e2e/test_broker_removal.go` following `testMultiDiskRemoval` pattern: apply 5-broker manifest, then apply 3-broker manifest, assert exactly 1 `CruiseControlOperation` of type `remove_broker`, assert only 3 pods remain Ready
- [x] 4.3 Wire into suite in `tests/e2e/koperator_suite_test.go` (after multi-disk removal block, line 74): `testInstallKafkaCluster("../../config/samples/simplekafkacluster_5broker.yaml")`, `testBatchedBrokerRemoval()`, `testUninstallKafkaCluster()`
- [ ] 4.4 Commit: `test: add e2e test for batched broker removal`
