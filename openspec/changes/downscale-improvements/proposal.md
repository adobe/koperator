## Why

When downscaling a Kafka cluster, the operator creates one CruiseControl operation per removed broker and immediately drops removed brokers from envoy external listener config, causing unnecessary partition movements and client connectivity loss during draining.

## What Changes

- **Batch broker removal**: Collect all broker IDs pending downscale and submit them as a single `remove_broker` CC operation, matching the existing `add_broker` batching behavior.
- **Retain draining brokers in envoy listeners**: Keep removed brokers in envoy config until CruiseControl finishes draining them (`GracefulDownscaleSucceeded`), so clients retain connectivity while data is being moved.

## Capabilities

### New Capabilities

- `batched-broker-removal`: Single CruiseControl `remove_broker` operation for all brokers removed in a manifest apply, eliminating redundant partition movements.
- `draining-broker-listener-retention`: Brokers removed from spec but still being drained by CruiseControl remain visible in envoy external listener resources until draining completes. Contour and NodePort listeners are not covered (follow-up).

### Modified Capabilities

<!-- none — no existing specs to delta -->

## Impact

- `controllers/cruisecontroltask_controller.go`: Replace single-broker `removeBroker` logic with `removeBrokers` (plural), mirroring `addBrokers` pattern.
- `pkg/util/util.go`: `ShouldIncludeBroker` gains a fallback path for `brokerConfig == nil` that checks `CruiseControlState`.
- Envoy external listener reconcilers (`pkg/resources/envoy/`) benefit automatically — they enumerate brokers via `GetBrokerIdsFromStatusAndSpec` and gate on `ShouldIncludeBroker`. Contour (`pkg/resources/contouringress`) and NodePort (`pkg/resources/nodeportexternalaccess`) iterate `Spec.Brokers` directly and are unaffected.
- No API or CRD changes. No breaking changes.
