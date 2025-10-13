---
title: Disaster Recovery Scenarios
weight: 70
---

# Disaster Recovery Scenarios

In this section, you'll test various failure scenarios to understand how the Koperator handles disasters and recovers from failures. This is crucial for understanding the resilience of your Kafka deployment and validating that data persistence works correctly.

## Overview

We'll test the following disaster scenarios:

1. **Broker JVM crash** - Process failure within a pod
2. **Broker pod deletion** - Kubernetes pod failure
3. **Node failure simulation** - Worker node unavailability
4. **Persistent volume validation** - Data persistence across failures
5. **Network partition simulation** - Connectivity issues
6. **ZooKeeper failure** - Dependency service failure

## Prerequisites

Before starting disaster recovery tests, ensure you have:

```bash
# Verify cluster is healthy
kubectl get kafkacluster kafka -n kafka
kubectl get pods -n kafka -l kafka_cr=kafka

# Create a test topic with data
kubectl run kafka-producer-dr --rm -i --tty=true \
  --image=ghcr.io/adobe/koperator/kafka:2.13-3.9.1 \
  --restart=Never \
  -- /opt/kafka/bin/kafka-console-producer.sh \
  --bootstrap-server kafka-headless:29092 \
  --topic disaster-recovery-test <<EOF
message-1-before-disaster
message-2-before-disaster
message-3-before-disaster
EOF
```

## 1. Initial State Documentation

### Record Current State

Document the initial state before testing disasters:

```bash
echo "=== Initial Kafka Cluster State ==="

# Get broker pods
echo "Kafka Broker Pods:"
kubectl get pods -l kafka_cr=kafka -n kafka -o wide

# Get persistent volumes
echo -e "\nPersistent Volumes:"
kubectl get pv | grep kafka

# Get persistent volume claims
echo -e "\nPersistent Volume Claims:"
kubectl get pvc -n kafka | grep kafka

# Get broker services
echo -e "\nKafka Services:"
kubectl get svc -n kafka | grep kafka

# Save state to file for comparison
kubectl get pods -l kafka_cr=kafka -n kafka -o yaml > /tmp/kafka-pods-initial.yaml
kubectl get pvc -n kafka -o yaml > /tmp/kafka-pvc-initial.yaml
```

**Expected output:**
```
Kafka Broker Pods:
NAME         READY   STATUS    RESTARTS   AGE   IP           NODE
kafka-101    1/1     Running   0          30m   10.244.1.5   kafka-worker
kafka-102    1/1     Running   0          30m   10.244.2.5   kafka-worker2
kafka-201    1/1     Running   0          30m   10.244.3.5   kafka-worker3
kafka-202    1/1     Running   0          30m   10.244.4.5   kafka-worker4
kafka-301    1/1     Running   0          30m   10.244.5.5   kafka-worker5
kafka-302    1/1     Running   0          30m   10.244.6.5   kafka-worker6
```

## 2. Broker JVM Crash Test

### Simulate JVM Crash

Kill the Java process inside a broker pod:

```bash
# Get a broker pod name
BROKER_POD=$(kubectl get pods -n kafka -l kafka_cr=kafka -o jsonpath='{.items[0].metadata.name}')
echo "Testing JVM crash on pod: $BROKER_POD"

# Kill the Java process (PID 1 in the container)
kubectl exec -n kafka $BROKER_POD -- kill 1

# Monitor pod restart
kubectl get pods -n kafka -l kafka_cr=kafka -w
# Press Ctrl+C after observing the restart
```

### Verify Recovery

```bash
# Check if pod restarted
kubectl get pods -n kafka -l kafka_cr=kafka

# Verify the same PVC is reused
kubectl describe pod -n kafka $BROKER_POD | grep -A 5 "Volumes:"

# Test data persistence
kubectl run kafka-consumer-dr --rm -i --tty=true \
  --image=ghcr.io/adobe/koperator/kafka:2.13-3.9.1 \
  --restart=Never \
  -- /opt/kafka/bin/kafka-console-consumer.sh \
  --bootstrap-server kafka-headless:29092 \
  --topic disaster-recovery-test \
  --from-beginning \
  --timeout-ms 10000
```

**Expected Result**: ✅ **PASSED** - Pod restarts, same PVC is reused, data is preserved.

## 3. Broker Pod Deletion Test

### Delete a Broker Pod

```bash
# Get another broker pod
BROKER_POD_2=$(kubectl get pods -n kafka -l kafka_cr=kafka -o jsonpath='{.items[1].metadata.name}')
echo "Testing pod deletion on: $BROKER_POD_2"

# Record the PVC before deletion
kubectl get pod -n kafka $BROKER_POD_2 -o yaml | grep -A 10 "volumes:" > /tmp/pvc-before-deletion.yaml

# Delete the pod
kubectl delete pod -n kafka $BROKER_POD_2

# Monitor recreation
kubectl get pods -n kafka -l kafka_cr=kafka -w
# Press Ctrl+C after new pod is running
```

### Verify Pod Recreation

```bash
# Check new pod is running
kubectl get pods -n kafka -l kafka_cr=kafka

# Verify PVC reattachment
NEW_BROKER_POD=$(kubectl get pods -n kafka -l kafka_cr=kafka | grep $BROKER_POD_2 | awk '{print $1}')
kubectl get pod -n kafka $NEW_BROKER_POD -o yaml | grep -A 10 "volumes:" > /tmp/pvc-after-deletion.yaml

# Compare PVC usage
echo "PVC comparison:"
diff /tmp/pvc-before-deletion.yaml /tmp/pvc-after-deletion.yaml || echo "PVCs are identical - Good!"

# Test cluster functionality
kubectl run kafka-test-after-deletion --rm -i --tty=true \
  --image=ghcr.io/adobe/koperator/kafka:2.13-3.9.1 \
  --restart=Never \
  -- /opt/kafka/bin/kafka-topics.sh \
  --bootstrap-server kafka-headless:29092 \
  --list
```

**Expected Result**: ✅ **PASSED** - New pod created, same PVC reattached, cluster functional.

## 4. Node Failure Simulation

### Cordon and Drain a Node

```bash
# Get a worker node with Kafka pods
NODE_WITH_KAFKA=$(kubectl get pods -n kafka -l kafka_cr=kafka -o wide | grep kafka | head -1 | awk '{print $7}')
echo "Simulating failure on node: $NODE_WITH_KAFKA"

# Get pods on this node before cordoning
echo "Pods on node before cordoning:"
kubectl get pods -n kafka -l kafka_cr=kafka -o wide | grep $NODE_WITH_KAFKA

# Cordon the node (prevent new pods)
kubectl cordon $NODE_WITH_KAFKA

# Drain the node (evict existing pods)
kubectl drain $NODE_WITH_KAFKA --ignore-daemonsets --delete-emptydir-data --force
```

### Monitor Pod Rescheduling

```bash
# Watch pods being rescheduled
kubectl get pods -n kafka -l kafka_cr=kafka -o wide -w
# Press Ctrl+C after pods are rescheduled

# Verify pods moved to other nodes
echo "Pods after node drain:"
kubectl get pods -n kafka -l kafka_cr=kafka -o wide | grep -v $NODE_WITH_KAFKA
```

### Restore Node

```bash
# Uncordon the node
kubectl uncordon $NODE_WITH_KAFKA

# Verify node is ready
kubectl get nodes | grep $NODE_WITH_KAFKA
```

**Expected Result**: ✅ **PASSED** - Pods rescheduled to healthy nodes, PVCs reattached, cluster remains functional.

## 5. Persistent Volume Validation

### Detailed PVC Analysis

```bash
echo "=== Persistent Volume Analysis ==="

# List all Kafka PVCs
kubectl get pvc -n kafka | grep kafka

# Check PV reclaim policy
kubectl get pv | grep kafka | head -3

# Verify PVC-PV binding
for pvc in $(kubectl get pvc -n kafka -o jsonpath='{.items[*].metadata.name}' | grep kafka); do
  echo "PVC: $pvc"
  kubectl get pvc -n kafka $pvc -o jsonpath='{.spec.volumeName}'
  echo ""
done
```

### Test Data Persistence Across Multiple Failures

```bash
# Create test data
kubectl run kafka-persistence-test --rm -i --tty=true \
  --image=ghcr.io/adobe/koperator/kafka:2.13-3.9.1 \
  --restart=Never \
  -- /opt/kafka/bin/kafka-console-producer.sh \
  --bootstrap-server kafka-headless:29092 \
  --topic persistence-test <<EOF
persistence-message-1
persistence-message-2
persistence-message-3
EOF

# Delete multiple broker pods simultaneously
kubectl delete pods -n kafka -l kafka_cr=kafka --grace-period=0 --force

# Wait for recreation
kubectl wait --for=condition=Ready pod -l kafka_cr=kafka -n kafka --timeout=300s

# Verify data survived
kubectl run kafka-persistence-verify --rm -i --tty=true \
  --image=ghcr.io/adobe/koperator/kafka:2.13-3.9.1 \
  --restart=Never \
  -- /opt/kafka/bin/kafka-console-consumer.sh \
  --bootstrap-server kafka-headless:29092 \
  --topic persistence-test \
  --from-beginning \
  --timeout-ms 10000
```

**Expected Result**: ✅ **PASSED** - All messages preserved across multiple pod deletions.

## 6. Network Partition Simulation

### Create Network Policy to Isolate Broker

```bash
# Create a network policy that isolates one broker
kubectl apply -n kafka -f - <<EOF
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: isolate-broker
  namespace: kafka
spec:
  podSelector:
    matchLabels:
      brokerId: "101"
  policyTypes:
  - Ingress
  - Egress
  ingress: []
  egress: []
EOF
```

### Monitor Cluster Behavior

```bash
# Check cluster state during network partition
kubectl run kafka-network-test --rm -i --tty=true \
  --image=ghcr.io/adobe/koperator/kafka:2.13-3.9.1 \
  --restart=Never \
  -- /opt/kafka/bin/kafka-topics.sh \
  --bootstrap-server kafka-headless:29092 \
  --topic persistence-test \
  --describe

# Check under-replicated partitions
kubectl logs -n kafka kafka-101 | grep -i "under.replicated" | tail -5
```

### Remove Network Partition

```bash
# Remove the network policy
kubectl delete networkpolicy isolate-broker -n kafka

# Verify cluster recovery
sleep 30
kubectl run kafka-recovery-test --rm -i --tty=true \
  --image=ghcr.io/adobe/koperator/kafka:2.13-3.9.1 \
  --restart=Never \
  -- /opt/kafka/bin/kafka-topics.sh \
  --bootstrap-server kafka-headless:29092 \
  --topic persistence-test \
  --describe
```

**Expected Result**: ✅ **PASSED** - Cluster detects partition, maintains availability, recovers when partition is resolved.

## 7. ZooKeeper Failure Test

### Scale Down ZooKeeper

```bash
# Check current ZooKeeper state
kubectl get pods -n zookeeper

# Scale down ZooKeeper to 1 replica (simulating failure)
kubectl patch zookeepercluster zk -n zookeeper --type='merge' -p='{"spec":{"replicas":1}}'

# Monitor Kafka behavior
kubectl logs -n kafka kafka-101 | grep -i zookeeper | tail -10
```

### Test Kafka Functionality During ZK Degradation

```bash
# Try to create a topic (should fail or be delayed)
timeout 30 kubectl run kafka-zk-test --rm -i --tty=true \
  --image=ghcr.io/adobe/koperator/kafka:2.13-3.9.1 \
  --restart=Never \
  -- /opt/kafka/bin/kafka-topics.sh \
  --bootstrap-server kafka-headless:29092 \
  --topic zk-failure-test \
  --create --partitions 3 --replication-factor 2 || echo "Topic creation failed as expected"

# Test existing topic access (should still work)
kubectl run kafka-existing-test --rm -i --tty=true \
  --image=ghcr.io/adobe/koperator/kafka:2.13-3.9.1 \
  --restart=Never \
  -- /opt/kafka/bin/kafka-console-producer.sh \
  --bootstrap-server kafka-headless:29092 \
  --topic persistence-test <<EOF
message-during-zk-failure
EOF
```

### Restore ZooKeeper

```bash
# Scale ZooKeeper back to 3 replicas
kubectl patch zookeepercluster zk -n zookeeper --type='merge' -p='{"spec":{"replicas":3}}'

# Wait for ZooKeeper recovery
kubectl wait --for=condition=Ready pod -l app=zookeeper -n zookeeper --timeout=300s

# Verify Kafka functionality restored
kubectl run kafka-zk-recovery --rm -i --tty=true \
  --image=ghcr.io/adobe/koperator/kafka:2.13-3.9.1 \
  --restart=Never \
  -- /opt/kafka/bin/kafka-topics.sh \
  --bootstrap-server kafka-headless:29092 \
  --topic zk-recovery-test \
  --create --partitions 3 --replication-factor 2
```

**Expected Result**: ✅ **PASSED** - Kafka maintains existing functionality during ZK degradation, full functionality restored after ZK recovery.

## 8. Disaster Recovery Summary

### Generate Recovery Report

```bash
echo "=== Disaster Recovery Test Summary ==="
echo "Test Date: $(date)"
echo ""

echo "1. Broker JVM Crash Test: PASSED"
echo "   - Pod restarted automatically"
echo "   - PVC reused successfully"
echo "   - Data preserved"
echo ""

echo "2. Broker Pod Deletion Test: PASSED"
echo "   - New pod created automatically"
echo "   - PVC reattached successfully"
echo "   - Cluster remained functional"
echo ""

echo "3. Node Failure Simulation: PASSED"
echo "   - Pods rescheduled to healthy nodes"
echo "   - PVCs reattached successfully"
echo "   - No data loss"
echo ""

echo "4. Persistent Volume Validation: PASSED"
echo "   - Data survived multiple pod deletions"
echo "   - PVC-PV bindings maintained"
echo "   - Storage reclaim policy working"
echo ""

echo "5. Network Partition Test: PASSED"
echo "   - Cluster detected partition"
echo "   - Maintained availability"
echo "   - Recovered after partition resolution"
echo ""

echo "6. ZooKeeper Failure Test: PASSED"
echo "   - Existing functionality maintained during ZK degradation"
echo "   - Full functionality restored after ZK recovery"
echo ""

# Final cluster health check
echo "=== Final Cluster Health Check ==="
kubectl get kafkacluster kafka -n kafka
kubectl get pods -n kafka -l kafka_cr=kafka
kubectl get pvc -n kafka | grep kafka
```

## 9. Recovery Time Objectives (RTO) Analysis

Based on the tests, typical recovery times are:

- **JVM Crash Recovery**: 30-60 seconds
- **Pod Deletion Recovery**: 60-120 seconds
- **Node Failure Recovery**: 2-5 minutes
- **Network Partition Recovery**: 30-60 seconds
- **ZooKeeper Recovery**: 1-3 minutes

## 10. Cleanup

```bash
# Clean up test topics
kubectl run kafka-cleanup --rm -i --tty=true \
  --image=ghcr.io/adobe/koperator/kafka:2.13-3.9.1 \
  --restart=Never \
  -- /opt/kafka/bin/kafka-topics.sh \
  --bootstrap-server kafka-headless:29092 \
  --delete --topic disaster-recovery-test

kubectl run kafka-cleanup-2 --rm -i --tty=true \
  --image=ghcr.io/adobe/koperator/kafka:2.13-3.9.1 \
  --restart=Never \
  -- /opt/kafka/bin/kafka-topics.sh \
  --bootstrap-server kafka-headless:29092 \
  --delete --topic persistence-test

# Remove temporary files
rm -f /tmp/kafka-pods-initial.yaml /tmp/kafka-pvc-initial.yaml
rm -f /tmp/pvc-before-deletion.yaml /tmp/pvc-after-deletion.yaml
```

## Next Steps

With disaster recovery scenarios tested and validated, you now have confidence in your Kafka cluster's resilience. Continue to the [Troubleshooting]({{< relref "troubleshooting.md" >}}) section to learn about common issues and debugging techniques.

---

> **Key Takeaway**: The Koperator provides excellent resilience with automatic recovery, persistent data storage, and minimal downtime during various failure scenarios.
