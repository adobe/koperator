## ADDED Requirements

### Requirement: Broker removals from a single manifest apply are batched into one CC operation
When a KafkaCluster spec is applied that removes N brokers, the operator SHALL submit a single `remove_broker` CruiseControl operation containing all N broker IDs, rather than N separate operations.

#### Scenario: Multiple brokers removed simultaneously
- **WHEN** a KafkaCluster spec is applied removing brokers 3 and 4 from a 5-broker cluster
- **THEN** exactly one `CruiseControlOperation` of type `remove_broker` is created
- **THEN** the operation's broker ID parameter contains both broker IDs (e.g. `"3,4"`)
- **THEN** both brokers transition to `GracefulDownscaleScheduled` referencing the same operation

#### Scenario: Single broker removal (unchanged behavior)
- **WHEN** a KafkaCluster spec is applied removing one broker
- **THEN** exactly one `CruiseControlOperation` of type `remove_broker` is created
- **THEN** the broker transitions to `GracefulDownscaleScheduled`

#### Scenario: Brokers already scheduled are not re-batched
- **WHEN** broker 3 is already in `GracefulDownscaleScheduled` state and broker 4 enters `GracefulDownscaleRequired`
- **THEN** a new `CruiseControlOperation` is created for broker 4 only
- **THEN** broker 3's existing operation is not modified
