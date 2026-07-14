## ADDED Requirements

### Requirement: Draining brokers remain in envoy listener config until CC completes
A broker removed from the KafkaCluster spec SHALL remain present in envoy external listener resources (configmap, service, deployment) while CruiseControl is actively draining it, and SHALL be removed only after `GracefulDownscaleSucceeded`.

> **Scope**: This requirement covers the envoy listener only. The Contour (`pkg/resources/contouringress`) and NodePort (`pkg/resources/nodeportexternalaccess`) reconcilers iterate `Spec.Brokers` directly and do not call `ShouldIncludeBroker`; they still drop removed brokers immediately. Fixing those paths is a follow-up. Istio ingress was removed from this fork and is not applicable.

#### Scenario: Broker retained during active draining
- **WHEN** a broker is removed from spec and its `CruiseControlState` is `GracefulDownscaleRunning`
- **THEN** `ShouldIncludeBroker` returns `true` for that broker
- **THEN** the broker remains present in the envoy configmap, service, and deployment resources

#### Scenario: Broker retained when CC operation is scheduled but not yet running
- **WHEN** a broker is removed from spec and its `CruiseControlState` is `GracefulDownscaleScheduled`
- **THEN** `ShouldIncludeBroker` returns `true` for that broker

#### Scenario: Broker retained when CC has not yet started (Required state)
- **WHEN** a broker is removed from spec and its `CruiseControlState` is `GracefulDownscaleRequired`
- **THEN** `ShouldIncludeBroker` returns `true` for that broker

#### Scenario: Broker retained on CC error (manual investigation needed)
- **WHEN** a broker is removed from spec and its `CruiseControlState` is `GracefulDownscaleCompletedWithError`
- **THEN** `ShouldIncludeBroker` returns `true` for that broker

#### Scenario: Broker retained when CC operation is paused
- **WHEN** a broker is removed from spec and its `CruiseControlState` is `GracefulDownscalePaused`
- **THEN** `ShouldIncludeBroker` returns `true` for that broker

#### Scenario: Broker removed from listener config after successful drain
- **WHEN** a broker's `CruiseControlState` transitions to `GracefulDownscaleSucceeded`
- **THEN** `ShouldIncludeBroker` returns `false` for that broker
- **THEN** the broker is removed from envoy external listener resources on the next reconcile

#### Scenario: Unknown broker excluded
- **WHEN** a broker ID has no entry in `BrokersState` (unknown broker)
- **THEN** `ShouldIncludeBroker` returns `false` for that broker
