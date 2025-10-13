---
title: Kafka Cluster Deployment
weight: 50
---

# Kafka Cluster Deployment

In this section, you'll deploy a production-ready Kafka cluster with monitoring, alerting, and dashboard integration. The deployment will demonstrate multi-AZ distribution, persistent storage, and comprehensive observability.

## Overview

We'll deploy:

1. **A 6-broker Kafka cluster** distributed across 3 availability zones
2. **Prometheus monitoring** with ServiceMonitor resources
3. **AlertManager rules** for auto-scaling and alerting
4. **Grafana dashboard** for Kafka metrics visualization
5. **Cruise Control** for cluster management and rebalancing

## 1. Deploy Kafka Cluster

### Create Kafka Cluster Configuration

First, let's create a comprehensive Kafka cluster configuration:

```bash
cd $TUTORIAL_DIR

# Create the KafkaCluster resource
kubectl create -n kafka -f - <<EOF
apiVersion: kafka.banzaicloud.io/v1beta1
kind: KafkaCluster
metadata:
  name: kafka
  namespace: kafka
spec:
  headlessServiceEnabled: true
  zkAddresses:
    - "zk-client.zookeeper:2181"
  rackAwareness:
    labels:
      - "topology.kubernetes.io/zone"
  brokerConfigGroups:
    default:
      brokerAnnotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "9020"
      storageConfigs:
        - mountPath: "/kafka-logs"
          pvcSpec:
            accessModes:
              - ReadWriteOnce
            resources:
              requests:
                storage: 10Gi
      serviceAccountName: "default"
      resourceRequirements:
        limits:
          cpu: "2"
          memory: "4Gi"
        requests:
          cpu: "1"
          memory: "2Gi"
      jvmPerformanceOpts: "-server -XX:+UseG1GC -XX:MaxGCPauseMillis=20 -XX:InitiatingHeapOccupancyPercent=35 -XX:+ExplicitGCInvokesConcurrent -Djava.awt.headless=true -Dsun.awt.fontpath=/usr/share/fonts/TTF"
      config:
        "auto.create.topics.enable": "true"
        "cruise.control.metrics.topic.auto.create": "true"
        "cruise.control.metrics.topic.num.partitions": "1"
        "cruise.control.metrics.topic.replication.factor": "2"
        "default.replication.factor": "2"
        "min.insync.replicas": "1"
        "num.partitions": "3"
        "offsets.topic.replication.factor": "2"
        "replica.lag.time.max.ms": "30000"
        "transaction.state.log.replication.factor": "2"
        "transaction.state.log.min.isr": "1"
  brokers:
    - id: 101
      brokerConfigGroup: "default"
      nodePortExternalIP:
        externalIP: "127.0.0.1"
    - id: 102
      brokerConfigGroup: "default"
      nodePortExternalIP:
        externalIP: "127.0.0.1"
    - id: 201
      brokerConfigGroup: "default"
      nodePortExternalIP:
        externalIP: "127.0.0.1"
    - id: 202
      brokerConfigGroup: "default"
      nodePortExternalIP:
        externalIP: "127.0.0.1"
    - id: 301
      brokerConfigGroup: "default"
      nodePortExternalIP:
        externalIP: "127.0.0.1"
    - id: 302
      brokerConfigGroup: "default"
      nodePortExternalIP:
        externalIP: "127.0.0.1"
  rollingUpgradeConfig:
    failureThreshold: 1
  cruiseControlConfig:
    cruiseControlEndpoint: "kafka-cruisecontrol-svc.kafka:8090"
    config: |
      # Copyright 2017 LinkedIn Corp. Licensed under the BSD 2-Clause License (the "License").
      # Sample Cruise Control configuration file.
      
      # Configuration for the metadata client.
      # =======================================
      
      # The maximum interval in milliseconds between two metadata refreshes.
      metadata.max.age.ms=300000
      
      # Client id for the Cruise Control. It is used for the metadata client.
      client.id=kafka-cruise-control
      
      # The size of TCP send buffer for Kafka network client.
      send.buffer.bytes=131072
      
      # The size of TCP receive buffer for Kafka network client.
      receive.buffer.bytes=131072
      
      # The time to wait for response from a server.
      request.timeout.ms=30000
      
      # Configurations for the load monitor
      # ===================================
      
      # The number of metric fetcher thread to fetch metrics for the Kafka cluster
      num.metric.fetchers=1
      
      # The metric sampler class
      metric.sampler.class=com.linkedin.kafka.cruisecontrol.monitor.sampling.CruiseControlMetricsReporterSampler
      
      # Configurations for CruiseControlMetricsReporter
      cruise.control.metrics.reporter.interval.ms=10000
      cruise.control.metrics.reporter.kubernetes.mode=true
      
      # The sample store class name
      sample.store.class=com.linkedin.kafka.cruisecontrol.monitor.sampling.KafkaSampleStore
      
      # The config for the Kafka sample store to save the partition metric samples
      partition.metric.sample.store.topic=__CruiseControlMetrics
      
      # The config for the Kafka sample store to save the model training samples
      broker.metric.sample.store.topic=__CruiseControlModelTrainingSamples
      
      # The replication factor of Kafka metric sample store topic
      sample.store.topic.replication.factor=2
      
      # The config for the number of Kafka sample store consumer threads
      num.sample.loading.threads=8
      
      # The partition assignor class for the metric samplers
      metric.sampler.partition.assignor.class=com.linkedin.kafka.cruisecontrol.monitor.sampling.DefaultMetricSamplerPartitionAssignor
      
      # The metric sampling interval in milliseconds
      metric.sampling.interval.ms=120000
      
      # The partition metrics window size in milliseconds
      partition.metrics.window.ms=300000
      
      # The number of partition metric windows to keep in memory
      num.partition.metrics.windows=1
      
      # The minimum partition metric samples required for a partition in each window
      min.samples.per.partition.metrics.window=1
      
      # The broker metrics window size in milliseconds
      broker.metrics.window.ms=300000
      
      # The number of broker metric windows to keep in memory
      num.broker.metrics.windows=20
      
      # The minimum broker metric samples required for a broker in each window
      min.samples.per.broker.metrics.window=1
      
      # The configuration for the BrokerCapacityConfigFileResolver (supports JBOD and non-JBOD broker capacities)
      capacity.config.file=config/capacity.json
      
      # Configurations for the analyzer
      # ===============================
      
      # The list of goals to optimize the Kafka cluster for with pre-computed proposals
      default.goals=com.linkedin.kafka.cruisecontrol.analyzer.goals.RackAwareGoal,com.linkedin.kafka.cruisecontrol.analyzer.goals.ReplicaCapacityGoal,com.linkedin.kafka.cruisecontrol.analyzer.goals.DiskCapacityGoal,com.linkedin.kafka.cruisecontrol.analyzer.goals.NetworkInboundCapacityGoal,com.linkedin.kafka.cruisecontrol.analyzer.goals.NetworkOutboundCapacityGoal,com.linkedin.kafka.cruisecontrol.analyzer.goals.CpuCapacityGoal,com.linkedin.kafka.cruisecontrol.analyzer.goals.ReplicaDistributionGoal,com.linkedin.kafka.cruisecontrol.analyzer.goals.PotentialNwOutGoal,com.linkedin.kafka.cruisecontrol.analyzer.goals.DiskUsageDistributionGoal,com.linkedin.kafka.cruisecontrol.analyzer.goals.NetworkInboundUsageDistributionGoal,com.linkedin.kafka.cruisecontrol.analyzer.goals.NetworkOutboundUsageDistributionGoal,com.linkedin.kafka.cruisecontrol.analyzer.goals.CpuUsageDistributionGoal,com.linkedin.kafka.cruisecontrol.analyzer.goals.TopicReplicaDistributionGoal,com.linkedin.kafka.cruisecontrol.analyzer.goals.LeaderReplicaDistributionGoal,com.linkedin.kafka.cruisecontrol.analyzer.goals.LeaderBytesInDistributionGoal
      
      # The list of supported goals
      goals=com.linkedin.kafka.cruisecontrol.analyzer.goals.RackAwareGoal,com.linkedin.kafka.cruisecontrol.analyzer.goals.ReplicaCapacityGoal,com.linkedin.kafka.cruisecontrol.analyzer.goals.DiskCapacityGoal,com.linkedin.kafka.cruisecontrol.analyzer.goals.NetworkInboundCapacityGoal,com.linkedin.kafka.cruisecontrol.analyzer.goals.NetworkOutboundCapacityGoal,com.linkedin.kafka.cruisecontrol.analyzer.goals.CpuCapacityGoal,com.linkedin.kafka.cruisecontrol.analyzer.goals.ReplicaDistributionGoal,com.linkedin.kafka.cruisecontrol.analyzer.goals.PotentialNwOutGoal,com.linkedin.kafka.cruisecontrol.analyzer.goals.DiskUsageDistributionGoal,com.linkedin.kafka.cruisecontrol.analyzer.goals.NetworkInboundUsageDistributionGoal,com.linkedin.kafka.cruisecontrol.analyzer.goals.NetworkOutboundUsageDistributionGoal,com.linkedin.kafka.cruisecontrol.analyzer.goals.CpuUsageDistributionGoal,com.linkedin.kafka.cruisecontrol.analyzer.goals.TopicReplicaDistributionGoal,com.linkedin.kafka.cruisecontrol.analyzer.goals.LeaderReplicaDistributionGoal,com.linkedin.kafka.cruisecontrol.analyzer.goals.LeaderBytesInDistributionGoal,com.linkedin.kafka.cruisecontrol.analyzer.goals.PreferredLeaderElectionGoal
      
      # The list of supported hard goals
      hard.goals=com.linkedin.kafka.cruisecontrol.analyzer.goals.RackAwareGoal,com.linkedin.kafka.cruisecontrol.analyzer.goals.ReplicaCapacityGoal,com.linkedin.kafka.cruisecontrol.analyzer.goals.DiskCapacityGoal,com.linkedin.kafka.cruisecontrol.analyzer.goals.NetworkInboundCapacityGoal,com.linkedin.kafka.cruisecontrol.analyzer.goals.NetworkOutboundCapacityGoal,com.linkedin.kafka.cruisecontrol.analyzer.goals.CpuCapacityGoal
      
      # The minimum percentage of well monitored partitions out of all the partitions
      min.monitored.partition.percentage=0.95
      
      # The balance threshold for CPU
      cpu.balance.threshold=1.1
      
      # The balance threshold for disk
      disk.balance.threshold=1.1
      
      # The balance threshold for network inbound utilization
      network.inbound.balance.threshold=1.1
      
      # The balance threshold for network outbound utilization
      network.outbound.balance.threshold=1.1
      
      # The balance threshold for the replica count
      replica.count.balance.threshold=1.1
      
      # The capacity threshold for CPU in percentage
      cpu.capacity.threshold=0.8
      
      # The capacity threshold for disk in percentage
      disk.capacity.threshold=0.8
      
      # The capacity threshold for network inbound utilization in percentage
      network.inbound.capacity.threshold=0.8
      
      # The capacity threshold for network outbound utilization in percentage
      network.outbound.capacity.threshold=0.8
      
      # The threshold for the number of replicas per broker
      replica.capacity.threshold=1000
      
      # The weight adjustment in the optimization algorithm
      cpu.low.utilization.threshold=0.0
      disk.low.utilization.threshold=0.0
      network.inbound.low.utilization.threshold=0.0
      network.outbound.low.utilization.threshold=0.0
      
      # The metric anomaly percentile upper threshold
      metric.anomaly.percentile.upper.threshold=90.0
      
      # The metric anomaly percentile lower threshold
      metric.anomaly.percentile.lower.threshold=10.0
      
      # How often should the cached proposal be expired and recalculated if necessary
      proposal.expiration.ms=60000
      
      # The maximum number of replicas that can reside on a broker at any given time.
      max.replicas.per.broker=10000
      
      # The number of threads to use for proposal candidate precomputing.
      num.proposal.precompute.threads=1
      
      # the topics that should be excluded from the partition movement.
      #topics.excluded.from.partition.movement
      
      # Configurations for the executor
      # ===============================
      
      # The max number of partitions to move in/out on a given broker at a given time.
      num.concurrent.partition.movements.per.broker=10
      
      # The interval between two execution progress checks.
      execution.progress.check.interval.ms=10000
      
      # Configurations for anomaly detector
      # ===================================
      
      # The goal violation notifier class
      anomaly.notifier.class=com.linkedin.kafka.cruisecontrol.detector.notifier.SelfHealingNotifier
      
      # The metric anomaly finder class
      metric.anomaly.finder.class=com.linkedin.kafka.cruisecontrol.detector.KafkaMetricAnomalyFinder
      
      # The anomaly detection interval
      anomaly.detection.interval.ms=10000
      
      # The goal violation to detect.
      anomaly.detection.goals=com.linkedin.kafka.cruisecontrol.analyzer.goals.RackAwareGoal,com.linkedin.kafka.cruisecontrol.analyzer.goals.ReplicaCapacityGoal,com.linkedin.kafka.cruisecontrol.analyzer.goals.DiskCapacityGoal,com.linkedin.kafka.cruisecontrol.analyzer.goals.NetworkInboundCapacityGoal,com.linkedin.kafka.cruisecontrol.analyzer.goals.NetworkOutboundCapacityGoal,com.linkedin.kafka.cruisecontrol.analyzer.goals.CpuCapacityGoal
      
      # The interested metrics for metric anomaly analyzer.
      metric.anomaly.analyzer.metrics=BROKER_PRODUCE_LOCAL_TIME_MS_MAX,BROKER_PRODUCE_LOCAL_TIME_MS_MEAN,BROKER_CONSUMER_FETCH_LOCAL_TIME_MS_MAX,BROKER_CONSUMER_FETCH_LOCAL_TIME_MS_MEAN,BROKER_FOLLOWER_FETCH_LOCAL_TIME_MS_MAX,BROKER_FOLLOWER_FETCH_LOCAL_TIME_MS_MEAN,BROKER_LOG_FLUSH_TIME_MS_MAX,BROKER_LOG_FLUSH_TIME_MS_MEAN
      
      # The zk path to store the anomaly detector state. This is to avoid duplicate anomaly detection due to controller failure.
      anomaly.detection.state.path=/CruiseControlAnomalyDetector/AnomalyDetectorState
      
      # Enable self healing for all anomaly detectors, unless the particular anomaly detector is explicitly disabled
      self.healing.enabled=true
      
      # Enable self healing for broker failure detector
      self.healing.broker.failure.enabled=true
      
      # Enable self healing for goal violation detector
      self.healing.goal.violation.enabled=true
      
      # Enable self healing for metric anomaly detector
      self.healing.metric.anomaly.enabled=true
      
      # configurations for the webserver
      # ================================
      
      # HTTP listen port for the Cruise Control
      webserver.http.port=8090
      
      # HTTP listen address for the Cruise Control
      webserver.http.address=0.0.0.0
      
      # Whether CORS support is enabled for API or not
      webserver.http.cors.enabled=false
      
      # Value for Access-Control-Allow-Origin
      webserver.http.cors.origin=http://localhost:8090/
      
      # Value for Access-Control-Request-Method
      webserver.http.cors.allowmethods=OPTIONS,GET,POST
      
      # Headers that should be exposed to the Browser (Webapp)
      # This is a special header that is used by the
      # User Tasks subsystem and should be explicitly
      # Enabled when CORS mode is used as part of the
      # Admin Interface
      webserver.http.cors.exposeheaders=User-Task-ID
      
      # REST API default prefix (dont forget the ending /*)
      webserver.api.urlprefix=/kafkacruisecontrol/*
      
      # Location where the Cruise Control frontend is deployed
      webserver.ui.diskpath=./cruise-control-ui/dist/
      
      # URL path prefix for UI
      webserver.ui.urlprefix=/*
      
      # Time After which request is converted to Async
      webserver.request.maxBlockTimeMs=10000
      
      # Default Session Expiry Period
      webserver.session.maxExpiryTimeMs=60000
      
      # Session cookie path
      webserver.session.path=/
      
      # Server Access Logs
      webserver.accesslog.enabled=true
      
      # Location of HTTP Request Logs
      webserver.accesslog.path=access.log
      
      # HTTP Request Log retention days
      webserver.accesslog.retention.days=14
EOF
```

### Monitor Cluster Deployment

Watch the cluster deployment progress:

```bash
# Watch KafkaCluster status
kubectl get kafkacluster kafka -n kafka -w -o wide
# Press Ctrl+C to stop watching when cluster is ready

# Check broker pods
kubectl get pods -n kafka -l kafka_cr=kafka

# Check all resources in kafka namespace
kubectl get all -n kafka
```

**Expected output (after 5-10 minutes):**
```
NAME    AGE   WARNINGS
kafka   10m   

NAME                                    READY   STATUS    RESTARTS   AGE
pod/kafka-101-xyz123                    1/1     Running   0          8m
pod/kafka-102-abc456                    1/1     Running   0          8m
pod/kafka-201-def789                    1/1     Running   0          8m
pod/kafka-202-ghi012                    1/1     Running   0          8m
pod/kafka-301-jkl345                    1/1     Running   0          8m
pod/kafka-302-mno678                    1/1     Running   0          8m
pod/kafka-cruisecontrol-xyz789          1/1     Running   0          6m
```

## 2. Configure Monitoring

### Create Prometheus ServiceMonitor

Create monitoring configuration for Prometheus to scrape Kafka metrics:

```bash
# Create ServiceMonitor for Kafka metrics
kubectl apply -n kafka -f - <<EOF
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: kafka-servicemonitor
  namespace: kafka
  labels:
    app: kafka
    release: monitoring
spec:
  selector:
    matchLabels:
      app: kafka
  endpoints:
  - port: metrics
    interval: 30s
    path: /metrics
---
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: kafka-jmx-servicemonitor
  namespace: kafka
  labels:
    app: kafka-jmx
    release: monitoring
spec:
  selector:
    matchLabels:
      app: kafka
  endpoints:
  - port: jmx-metrics
    interval: 30s
    path: /metrics
EOF
```

### Create AlertManager Rules

Set up alerting rules for Kafka monitoring and auto-scaling:

```bash
# Create PrometheusRule for Kafka alerting
kubectl apply -n kafka -f - <<EOF
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: kafka-alerts
  namespace: kafka
  labels:
    app: kafka
    release: monitoring
spec:
  groups:
  - name: kafka.rules
    rules:
    - alert: KafkaOfflinePartitions
      expr: kafka_controller_kafkacontroller_offlinepartitionscount > 0
      for: 5m
      labels:
        severity: critical
      annotations:
        summary: "Kafka has offline partitions"
        description: "Kafka cluster {{ \$labels.instance }} has {{ \$value }} offline partitions"
    
    - alert: KafkaUnderReplicatedPartitions
      expr: kafka_server_replicamanager_underreplicatedpartitions > 0
      for: 5m
      labels:
        severity: warning
      annotations:
        summary: "Kafka has under-replicated partitions"
        description: "Kafka cluster {{ \$labels.instance }} has {{ \$value }} under-replicated partitions"
    
    - alert: KafkaHighProducerRequestRate
      expr: rate(kafka_network_requestmetrics_requests_total{request="Produce"}[5m]) > 1000
      for: 10m
      labels:
        severity: warning
        command: "upScale"
      annotations:
        summary: "High Kafka producer request rate"
        description: "Kafka producer request rate is {{ \$value }} requests/sec"
    
    - alert: KafkaLowProducerRequestRate
      expr: rate(kafka_network_requestmetrics_requests_total{request="Produce"}[5m]) < 100
      for: 30m
      labels:
        severity: info
        command: "downScale"
      annotations:
        summary: "Low Kafka producer request rate"
        description: "Kafka producer request rate is {{ \$value }} requests/sec"
EOF
```

## 3. Load Grafana Dashboard

### Apply Complete Kafka Dashboard

The complete Kafka Looking Glass dashboard provides comprehensive monitoring with dozens of panels covering all aspects of Kafka performance:

```bash
# Apply the complete Kafka Looking Glass dashboard directly
kubectl apply -n default \
  -f https://raw.githubusercontent.com/amuraru/k8s-kafka-operator/master/grafana-dashboard.yaml
```

### Dashboard Features

The complete Kafka Looking Glass dashboard includes:

**Overview Section:**
- Brokers online count
- Cluster version information
- Active controllers
- Topic count
- Offline partitions
- Under-replicated partitions

**Performance Metrics:**
- Message throughput (in/out per second)
- Bytes throughput (in/out per second)
- Request latency breakdown
- Network request metrics
- Replication rates

**Broker Health:**
- JVM memory usage
- Garbage collection metrics
- Thread states
- Log flush times
- Disk usage

**Topic Analysis:**
- Per-topic throughput
- Partition distribution
- Leader distribution
- Consumer lag metrics

**ZooKeeper Integration:**
- ZooKeeper quorum size
- Leader count
- Request latency
- Digest mismatches

**Error Monitoring:**
- Offline broker disks
- Orphan replicas
- Under-replicated partitions
- Network issues

## 4. Verify Deployment

### Check Cluster Status

```bash
# Describe the KafkaCluster
kubectl describe kafkacluster kafka -n kafka

# Check broker distribution across zones
kubectl get pods -n kafka -l kafka_cr=kafka -o wide

# Check persistent volumes
kubectl get pv,pvc -n kafka
```

### Access Cruise Control

```bash
# Port forward to Cruise Control (in a separate terminal)
kubectl port-forward -n kafka svc/kafka-cruisecontrol-svc 8090:8090 &

# Check Cruise Control status (optional)
curl -s http://localhost:8090/kafkacruisecontrol/v1/state | jq .
```

### Verify Monitoring Integration

```bash
# Check if Prometheus is scraping Kafka metrics
kubectl port-forward -n default svc/monitoring-kube-prometheus-prometheus 9090 &

# Visit http://localhost:9090 and search for kafka_ metrics
```

### Access the Kafka Looking Glass Dashboard

```bash
# Get Grafana admin password
kubectl get secret --namespace default monitoring-grafana \
  -o jsonpath="{.data.admin-password}" | base64 --decode
echo ""

# Port forward to Grafana
kubectl port-forward -n default svc/monitoring-grafana 3000:80 &
```

Visit http://localhost:3000 and:
1. Login with admin/[password from above]
2. Navigate to Dashboards → Browse
3. Look for "Kafka Looking Glass" dashboard
4. The dashboard should show real-time metrics from your Kafka cluster

The dashboard will automatically detect your cluster using the template variables:
- **Namespace**: Should auto-select "kafka"
- **Cluster Name**: Should auto-select "kafka"
- **Broker**: Shows all brokers (101, 102, 201, 202, 301, 302)
- **Topic**: Shows all topics in your cluster

## Next Steps

With your Kafka cluster successfully deployed and monitoring configured, you can now proceed to test the deployment. Continue to the [Testing and Validation]({{< relref "testing.md" >}}) section to create topics and run producer/consumer tests.

---

> **Note**: The cluster deployment may take 10-15 minutes to complete. The brokers will be distributed across the three availability zones you configured earlier, providing high availability and fault tolerance.
