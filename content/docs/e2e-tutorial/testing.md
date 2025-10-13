---
title: Testing and Validation
weight: 60
---

# Testing and Validation

In this section, you'll thoroughly test your Kafka cluster deployment by creating topics, running producers and consumers, and performing performance tests. This validates that your cluster is working correctly and can handle production workloads.

## Overview

We'll perform the following tests:

1. **Basic connectivity tests** - Verify cluster accessibility
2. **Topic management** - Create, list, and configure topics
3. **Producer/Consumer tests** - Send and receive messages
4. **Performance testing** - Load testing with high throughput
5. **Monitoring validation** - Verify metrics collection
6. **Multi-AZ validation** - Confirm rack awareness

## 1. Basic Connectivity Tests

### List Existing Topics

First, verify that you can connect to the Kafka cluster:

```bash
# List topics using kubectl run
kubectl run kafka-topics --rm -i --tty=true \
  --image=ghcr.io/adobe/koperator/kafka:2.13-3.9.1 \
  --restart=Never \
  -- /opt/kafka/bin/kafka-topics.sh \
  --bootstrap-server kafka-headless:29092 \
  --list
```

**Expected output:**
```
__CruiseControlMetrics
__CruiseControlModelTrainingSamples
__consumer_offsets
```

### Check Cluster Information

```bash
# Get cluster metadata
kubectl run kafka-metadata --rm -i --tty=true \
  --image=ghcr.io/adobe/koperator/kafka:2.13-3.9.1 \
  --restart=Never \
  -- /opt/kafka/bin/kafka-broker-api-versions.sh \
  --bootstrap-server kafka-headless:29092
```

## 2. Topic Management

### Create a Test Topic

Create a topic with multiple partitions and replicas:

```bash
# Create a test topic with 12 partitions distributed across brokers
kubectl run kafka-topics --rm -i --tty=true \
  --image=ghcr.io/adobe/koperator/kafka:2.13-3.9.1 \
  --restart=Never \
  -- /opt/kafka/bin/kafka-topics.sh \
  --bootstrap-server kafka-headless:29092 \
  --topic perf_topic \
  --replica-assignment 101:201:301,102:202:302,101:201:301,102:202:302,101:201:301,102:202:302,101:201:301,102:202:302,101:201:301,102:202:302,101:201:301,102:202:302 \
  --create
```

### Describe the Topic

```bash
# Describe the topic to verify configuration
kubectl run kafka-topics --rm -i --tty=true \
  --image=ghcr.io/adobe/koperator/kafka:2.13-3.9.1 \
  --restart=Never \
  -- /opt/kafka/bin/kafka-topics.sh \
  --bootstrap-server kafka-headless:29092 \
  --topic perf_topic \
  --describe
```

**Expected output:**
```
Topic: perf_topic	TopicId: xyz-123-abc	PartitionCount: 12	ReplicationFactor: 3
	Topic: perf_topic	Partition: 0	Leader: 101	Replicas: 101,201,301	Isr: 101,201,301
	Topic: perf_topic	Partition: 1	Leader: 102	Replicas: 102,202,302	Isr: 102,202,302
	...
```

### Configure Topic Retention

```bash
# Set custom retention period (12 minutes for testing)
kubectl run kafka-configs --rm -i --tty=true \
  --image=ghcr.io/adobe/koperator/kafka:2.13-3.9.1 \
  --restart=Never \
  -- /opt/kafka/bin/kafka-configs.sh \
  --zookeeper zk-client.zookeeper:2181/kafka \
  --alter --entity-name perf_topic \
  --entity-type topics \
  --add-config retention.ms=720000
```

## 3. Producer/Consumer Tests

### Simple Message Test

#### Start a Producer

```bash
# Start a simple producer (run in one terminal)
kubectl run kafka-producer \
  --image=ghcr.io/adobe/koperator/kafka:2.13-3.9.1 \
  --restart=Never \
  -- /opt/kafka/bin/kafka-console-producer.sh \
  --bootstrap-server kafka-headless:29092 \
  --topic perf_topic
```

Type some test messages:
```
Hello Kafka!
This is a test message
Testing multi-AZ deployment
```

#### Start a Consumer

```bash
# Start a consumer (run in another terminal)
kubectl run kafka-consumer \
  --image=ghcr.io/adobe/koperator/kafka:2.13-3.9.1 \
  --restart=Never \
  -- /opt/kafka/bin/kafka-console-consumer.sh \
  --bootstrap-server kafka-headless:29092 \
  --topic perf_topic \
  --from-beginning
```

You should see the messages you sent from the producer.

### Clean Up Test Pods

```bash
# Clean up the test pods
kubectl delete pod kafka-producer --ignore-not-found=true
kubectl delete pod kafka-consumer --ignore-not-found=true
```

## 4. Performance Testing

### Producer Performance Test

Run a high-throughput producer test:

```bash
# Start producer performance test
kubectl run kafka-producer-perf \
  --image=ghcr.io/adobe/koperator/kafka:2.13-3.9.1 \
  --restart=Never \
  -- /opt/kafka/bin/kafka-producer-perf-test.sh \
  --producer-props bootstrap.servers=kafka-headless:29092 acks=all \
  --topic perf_topic \
  --record-size 1000 \
  --throughput 29000 \
  --num-records 2110000
```

**Expected output:**
```
100000 records sent, 28500.0 records/sec (27.18 MB/sec), 2.1 ms avg latency, 45 ms max latency.
200000 records sent, 29000.0 records/sec (27.66 MB/sec), 1.8 ms avg latency, 38 ms max latency.
...
```

### Consumer Performance Test

In another terminal, run a consumer performance test:

```bash
# Start consumer performance test
kubectl run kafka-consumer-perf \
  --image=ghcr.io/adobe/koperator/kafka:2.13-3.9.1 \
  --restart=Never \
  -- /opt/kafka/bin/kafka-consumer-perf-test.sh \
  --broker-list kafka-headless:29092 \
  --group perf-consume \
  --messages 10000000 \
  --topic perf_topic \
  --show-detailed-stats \
  --from-latest \
  --timeout 100000
```

**Expected output:**
```
start.time, end.time, data.consumed.in.MB, MB.sec, data.consumed.in.nMsg, nMsg.sec, rebalance.time.ms, fetch.time.ms, fetch.MB.sec, fetch.nMsg.sec
2024-01-15 10:30:00:000, 2024-01-15 10:30:10:000, 95.37, 9.54, 100000, 10000.0, 1500, 8500, 11.22, 11764.7
```

## 5. Monitoring Validation

### Check Kafka Metrics in Prometheus

```bash
# Port forward to Prometheus (if not already done)
kubectl port-forward -n default svc/monitoring-kube-prometheus-prometheus 9090 &

# Check if Kafka metrics are being collected
curl -s "http://localhost:9090/api/v1/query?query=kafka_server_brokertopicmetrics_messagesin_total" | jq .
```

### Access Grafana Dashboard

```bash
# Port forward to Grafana (if not already done)
kubectl port-forward -n default svc/monitoring-grafana 3000:80 &

# Get Grafana admin password
kubectl get secret --namespace default monitoring-grafana \
  -o jsonpath="{.data.admin-password}" | base64 --decode
echo ""
```

Visit http://localhost:3000 and:
1. Login with admin/[password]
2. Navigate to Dashboards
3. Look for "Kafka Looking Glass" dashboard
4. Verify metrics are being displayed

### Check AlertManager

```bash
# Port forward to AlertManager
kubectl port-forward -n default svc/monitoring-kube-prometheus-alertmanager 9093 &
```

Visit http://localhost:9093 to see any active alerts.

## 6. Multi-AZ Validation

### Verify Broker Distribution

Check that brokers are distributed across availability zones:

```bash
# Check broker pod distribution
kubectl get pods -n kafka -l kafka_cr=kafka -o wide \
  --sort-by='.spec.nodeName'

# Check node labels
kubectl get nodes \
  --label-columns=topology.kubernetes.io/zone \
  -l topology.kubernetes.io/zone
```

### Verify Rack Awareness

```bash
# Check if rack awareness is working by examining topic partition distribution
kubectl run kafka-topics --rm -i --tty=true \
  --image=ghcr.io/adobe/koperator/kafka:2.13-3.9.1 \
  --restart=Never \
  -- /opt/kafka/bin/kafka-topics.sh \
  --bootstrap-server kafka-headless:29092 \
  --topic perf_topic \
  --describe
```

Verify that replicas are distributed across different broker IDs (which correspond to different AZs).

## 7. Advanced Testing

### Test Topic Creation via CRD

Create a topic using Kubernetes CRD:

```bash
# Create topic using KafkaTopic CRD
kubectl apply -n kafka -f - <<EOF
apiVersion: kafka.banzaicloud.io/v1alpha1
kind: KafkaTopic
metadata:
  name: test-topic-crd
  namespace: kafka
spec:
  clusterRef:
    name: kafka
  name: test-topic-crd
  partitions: 6
  replicationFactor: 2
  config:
    "retention.ms": "604800000"
    "cleanup.policy": "delete"
EOF
```

### Verify CRD Topic Creation

```bash
# Check KafkaTopic resource
kubectl get kafkatopic -n kafka

# Verify topic exists in Kafka
kubectl run kafka-topics --rm -i --tty=true \
  --image=ghcr.io/adobe/koperator/kafka:2.13-3.9.1 \
  --restart=Never \
  -- /opt/kafka/bin/kafka-topics.sh \
  --bootstrap-server kafka-headless:29092 \
  --list | grep test-topic-crd
```

### Test Consumer Groups

```bash
# Create multiple consumers in the same group
for i in {1..3}; do
  kubectl run kafka-consumer-group-$i \
    --image=ghcr.io/adobe/koperator/kafka:2.13-3.9.1 \
    --restart=Never \
    -- /opt/kafka/bin/kafka-console-consumer.sh \
    --bootstrap-server kafka-headless:29092 \
    --topic perf_topic \
    --group test-group &
done

# Check consumer group status
kubectl run kafka-consumer-groups --rm -i --tty=true \
  --image=ghcr.io/adobe/koperator/kafka:2.13-3.9.1 \
  --restart=Never \
  -- /opt/kafka/bin/kafka-consumer-groups.sh \
  --bootstrap-server kafka-headless:29092 \
  --group test-group \
  --describe
```

## 8. Performance Metrics Summary

After running the performance tests, you should see metrics similar to:

### Producer Performance
- **Throughput**: 25,000-30,000 records/sec
- **Latency**: 1-3 ms average
- **Record Size**: 1KB

### Consumer Performance
- **Throughput**: 10,000+ records/sec
- **Lag**: Minimal (< 100 records)

### Resource Utilization
- **CPU**: 20-40% per broker
- **Memory**: 2-3GB per broker
- **Disk I/O**: Moderate

## 9. Cleanup Test Resources

```bash
# Clean up performance test pods
kubectl delete pod kafka-producer-perf --ignore-not-found=true
kubectl delete pod kafka-consumer-perf --ignore-not-found=true

# Clean up consumer group pods
for i in {1..3}; do
  kubectl delete pod kafka-consumer-group-$i --ignore-not-found=true
done

# Optionally delete test topics
kubectl delete kafkatopic test-topic-crd -n kafka
```

## Troubleshooting

### Producer/Consumer Connection Issues

```bash
# Check broker connectivity
kubectl run kafka-test --rm -i --tty=true \
  --image=ghcr.io/adobe/koperator/kafka:2.13-3.9.1 \
  --restart=Never \
  -- /bin/bash

# Inside the pod, test connectivity
telnet kafka-headless 29092
```

### Performance Issues

```bash
# Check broker resource usage
kubectl top pods -n kafka

# Check broker logs
kubectl logs -n kafka kafka-101-xyz123

# Check JVM metrics
kubectl exec -n kafka kafka-101-xyz123 -- jps -v
```

### Monitoring Issues

```bash
# Check ServiceMonitor
kubectl get servicemonitor -n kafka

# Check Prometheus targets
curl -s "http://localhost:9090/api/v1/targets" | jq '.data.activeTargets[] | select(.labels.job | contains("kafka"))'
```

## Next Steps

With your Kafka cluster thoroughly tested and validated, you can now explore disaster recovery scenarios. Continue to the [Disaster Recovery Scenarios]({{< relref "disaster-recovery.md" >}}) section to test failure handling and recovery procedures.

---

> **Note**: Keep the performance test results for comparison after implementing any configuration changes or during disaster recovery testing.
