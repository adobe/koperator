---
title: KRaft Mode (ZooKeeper-free Kafka)
shorttitle: KRaft Mode
weight: 400
---

Apache Kafka KRaft (Kafka Raft) mode is a new consensus mechanism that eliminates the dependency on Apache ZooKeeper. KRaft mode uses Kafka's built-in Raft consensus algorithm to manage cluster metadata, making Kafka deployments simpler and more scalable.

## Overview

KRaft mode offers several advantages over traditional ZooKeeper-based deployments:

- **Simplified Architecture**: No need to deploy and manage a separate ZooKeeper cluster
- **Better Scalability**: Improved metadata handling for large clusters
- **Faster Recovery**: Quicker cluster startup and recovery times
- **Reduced Operational Complexity**: Fewer moving parts to monitor and maintain
- **Future-Ready**: KRaft is the future of Kafka and will eventually replace ZooKeeper
- **Production Stability**: Kafka 3.9.1 includes significant KRaft improvements and bug fixes

## Prerequisites

- Koperator version 0.26.0 or later
- Kafka version 3.9.1 or later (minimum: 3.3.0, but 3.9.1+ recommended for stability and features)
- Kubernetes cluster with sufficient resources

> **Note**: While KRaft mode is available starting from Kafka 3.3.0, version 3.9.1 includes significant stability improvements, bug fixes, and performance enhancements for KRaft deployments. For production environments, always use Kafka 3.9.1 or later.

## KRaft Architecture in Koperator

In KRaft mode, Koperator deploys Kafka brokers with different process roles:

- **Controller nodes**: Handle cluster metadata and leader election
- **Broker nodes**: Handle client requests and data storage
- **Combined nodes**: Can act as both controller and broker (not recommended for production)

## Basic KRaft Configuration

To enable KRaft mode in your KafkaCluster custom resource, set the `kRaft` field to `true`:

```yaml
apiVersion: kafka.banzaicloud.io/v1beta1
kind: KafkaCluster
metadata:
  name: kafka-kraft
spec:
  kRaft: true
  # ... other configuration
```

## Process Roles Configuration

Configure different process roles for your brokers using the `processRoles` field:

### Controller-only nodes

```yaml
brokers:
  - id: 3
    brokerConfig:
      processRoles:
        - controller
```

### Broker-only nodes

```yaml
brokers:
  - id: 0
    brokerConfig:
      processRoles:
        - broker
```

### Combined nodes (not recommended for production)

```yaml
brokers:
  - id: 0
    brokerConfig:
      processRoles:
        - controller
        - broker
```

## Listener Configuration for KRaft

KRaft mode requires specific listener configuration for controller communication:

```yaml
listenersConfig:
  internalListeners:
    - type: "plaintext"
      name: "internal"
      containerPort: 29092
      usedForInnerBrokerCommunication: true
    - type: "plaintext"
      name: "controller"
      containerPort: 29093
      usedForInnerBrokerCommunication: false
      usedForControllerCommunication: true
```

## Quick Start with KRaft

To quickly deploy a KRaft-enabled Kafka cluster, you can use this sample configuration:

{{< include-code "create-kraft-cluster.sample" "bash" >}}

## Complete KRaft Example

Here's a complete example of a KRaft-enabled Kafka cluster:

```yaml
apiVersion: kafka.banzaicloud.io/v1beta1
kind: KafkaCluster
metadata:
  name: kafka-kraft
spec:
  kRaft: true
  headlessServiceEnabled: true
  clusterImage: "ghcr.io/adobe/koperator/kafka:2.13-3.9.1"
  
  brokerConfigGroups:
    default:
      storageConfigs:
        - mountPath: "/kafka-logs"
          pvcSpec:
            accessModes:
              - ReadWriteOnce
            resources:
              requests:
                storage: 10Gi
    broker:
      processRoles:
        - broker
      storageConfigs:
        - mountPath: "/kafka-logs-broker"
          pvcSpec:
            accessModes:
              - ReadWriteOnce
            resources:
              requests:
                storage: 10Gi

  brokers:
    # Broker-only nodes
    - id: 0
      brokerConfigGroup: "broker"
    - id: 1
      brokerConfigGroup: "broker"
    - id: 2
      brokerConfigGroup: "broker"
    
    # Controller-only nodes
    - id: 3
      brokerConfigGroup: "default"
      brokerConfig:
        processRoles:
          - controller
    - id: 4
      brokerConfigGroup: "default"
      brokerConfig:
        processRoles:
          - controller
    - id: 5
      brokerConfigGroup: "default"
      brokerConfig:
        processRoles:
          - controller

  listenersConfig:
    internalListeners:
      - type: "plaintext"
        name: "internal"
        containerPort: 29092
        usedForInnerBrokerCommunication: true
      - type: "plaintext"
        name: "controller"
        containerPort: 29093
        usedForInnerBrokerCommunication: false
        usedForControllerCommunication: true

  cruiseControlConfig:
    cruiseControlTaskSpec:
      RetryDurationMinutes: 5
    topicConfig:
      partitions: 12
      replicationFactor: 3
```

## Kafka Version Recommendations

### Why Kafka 3.9.1 is Recommended

Kafka 3.9.1 includes several important improvements for KRaft mode:

- **Enhanced Stability**: Critical bug fixes for controller failover and metadata consistency
- **Improved Performance**: Better handling of large metadata operations and faster startup times
- **Security Enhancements**: Updated security features and vulnerability fixes
- **Monitoring Improvements**: Better metrics and observability for KRaft clusters
- **Production Readiness**: Extensive testing and validation for production workloads

## Best Practices

### Controller Node Configuration

- **Use odd numbers**: Deploy an odd number of controller nodes (3, 5, or 7) for proper quorum
- **Minimum 3 controllers**: For production environments, use at least 3 controller nodes
- **Separate controllers**: Use dedicated controller-only nodes for production workloads
- **Resource allocation**: Controllers need less CPU and memory than brokers but require fast storage

### Storage Considerations

- **Fast storage**: Use SSD storage for controller nodes to ensure fast metadata operations
- **Separate storage**: Consider using separate storage for controllers and brokers
- **Backup strategy**: Implement proper backup strategies for controller metadata

### Network Configuration

- **Controller listener**: Always configure a dedicated listener for controller communication
- **Security**: Apply the same security configurations (SSL, SASL) to controller listeners

## Migration from ZooKeeper

{{< warning >}}
Migration from ZooKeeper-based clusters to KRaft mode is a complex process that requires careful planning. Always test the migration process in a non-production environment first.
{{< /warning >}}

Currently, Koperator does not support automatic migration from ZooKeeper to KRaft mode. For migration scenarios:

1. **New deployments**: Use KRaft mode for all new Kafka clusters
2. **Existing clusters**: Plan for a blue-green deployment strategy
3. **Data migration**: Use tools like MirrorMaker 2.0 for data migration between clusters

## Monitoring KRaft Clusters

KRaft clusters expose additional metrics for monitoring controller health:

- `kafka.server:type=raft-metrics`: Raft consensus metrics
- `kafka.server:type=broker-metadata-metrics`: Metadata handling metrics

Configure your monitoring to track these KRaft-specific metrics alongside standard Kafka metrics.

## Troubleshooting

### Common Issues

1. **Controller quorum loss**: Ensure at least (n/2 + 1) controllers are healthy
2. **Metadata inconsistency**: Check controller logs for Raft consensus issues
3. **Slow startup**: Controllers may take longer to start during initial cluster formation

### Useful Commands

Check controller status:
```bash
kubectl exec -it kafka-controller-3-xxx -- kafka-metadata-shell.sh --snapshot /kafka-logs/__cluster_metadata-0/00000000000000000000.log
```

View controller logs:
```bash
kubectl logs kafka-controller-3-xxx -f
```

## Limitations

- **No ZooKeeper migration**: Automatic migration from ZooKeeper is not supported
- **Kafka version**: Requires Kafka 3.3.0 minimum, but 3.9.1 or later is strongly recommended
- **Feature parity**: Some advanced ZooKeeper features may not be available in early KRaft versions

## Resources

- [Apache Kafka KRaft Documentation](https://kafka.apache.org/documentation/#kraft)
- [KIP-500: Replace ZooKeeper with a Self-Managed Metadata Quorum](https://cwiki.apache.org/confluence/display/KAFKA/KIP-500%3A+Replace+ZooKeeper+with+a+Self-Managed+Metadata+Quorum)
- [Koperator KRaft Sample Configuration](https://github.com/adobe/koperator/blob/master/config/samples/kraft/simplekafkacluster_kraft.yaml)
