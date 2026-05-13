## Why

When downscaling a Kafka cluster, the operator creates one CruiseControl operation per removed broker and immediately drops removed brokers from external listener config (envoy/istio/contour), causing unnecessary partition movements and client connectivity loss during draining.

## What Changes

- **Batch broker removal**: Collect all broker IDs pending downscale and submit them as a single `remove_broker` CC operation, matching the existing `add_broker` batching behavior.
- **Retain draining brokers in external listeners**: Keep removed brokers in envoy/istio/contour config until CruiseControl finishes draining them (`GracefulDownscaleSucceeded`), so clients retain connectivity while data is being moved.

## Capabilities

### New Capabilities

- `batched-broker-removal`: Single CruiseControl `remove_broker` operation for all brokers removed in a manifest apply, eliminating redundant partition movements.
- `draining-broker-listener-retention`: Brokers removed from spec but still being drained by CruiseControl remain visible in all external listener resources until draining completes.

### Modified Capabilities

<!-- none — no existing specs to delta -->

## Impact

- `controllers/cruisecontroltask_controller.go`: Replace single-broker `removeBroker` logic with `removeBrokers` (plural), mirroring `addBrokers` pattern.
- `pkg/util/util.go`: `ShouldIncludeBroker` gains a fallback path for `brokerConfig == nil` that checks `CruiseControlState`.
- All external listener reconcilers (`pkg/resources/envoy/`, `pkg/resources/istioingress/`) benefit automatically — they all gate on `ShouldIncludeBroker`.
- No API or CRD changes. No breaking changes.
