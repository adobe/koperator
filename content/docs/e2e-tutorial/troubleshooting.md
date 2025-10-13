---
title: Troubleshooting Guide
weight: 80
---

# Troubleshooting Guide

This section provides comprehensive troubleshooting guidance for common issues you might encounter during the Kafka deployment and operation. It includes diagnostic commands, common error patterns, and resolution strategies.

## Overview

Common categories of issues:

1. **Cluster Setup Issues** - Problems during initial deployment
2. **Connectivity Issues** - Network and service discovery problems
3. **Performance Issues** - Throughput and latency problems
4. **Storage Issues** - Persistent volume and disk problems
5. **Monitoring Issues** - Metrics collection and dashboard problems
6. **Operator Issues** - Koperator-specific problems

## 1. Diagnostic Commands

### Essential Debugging Commands

```bash
# Set namespace context for convenience
kubectl config set-context --current --namespace=kafka

# Quick cluster health check
echo "=== Cluster Health Overview ==="
kubectl get kafkacluster kafka -o wide
kubectl get pods -l kafka_cr=kafka
kubectl get svc | grep kafka
kubectl get pvc | grep kafka
```

### Detailed Diagnostics

```bash
# Comprehensive cluster diagnostics
function kafka_diagnostics() {
    echo "=== Kafka Cluster Diagnostics ==="
    echo "Timestamp: $(date)"
    echo ""
    
    echo "1. KafkaCluster Resource:"
    kubectl describe kafkacluster kafka
    echo ""
    
    echo "2. Broker Pods:"
    kubectl get pods -l kafka_cr=kafka -o wide
    echo ""
    
    echo "3. Pod Events:"
    kubectl get events --sort-by=.metadata.creationTimestamp | grep kafka | tail -10
    echo ""
    
    echo "4. Persistent Volumes:"
    kubectl get pv | grep kafka
    echo ""
    
    echo "5. Services:"
    kubectl get svc | grep kafka
    echo ""
    
    echo "6. Operator Status:"
    kubectl get pods -l app.kubernetes.io/instance=kafka-operator
    echo ""
}

# Run diagnostics
kafka_diagnostics
```

## 2. Cluster Setup Issues

### Issue: Koperator Pod Not Starting

**Symptoms:**
- Operator pod in `CrashLoopBackOff` or `ImagePullBackOff`
- KafkaCluster resource not being processed

**Diagnosis:**
```bash
# Check operator pod status
kubectl get pods -l app.kubernetes.io/instance=kafka-operator

# Check operator logs
kubectl logs -l app.kubernetes.io/instance=kafka-operator -c manager --tail=50

# Check operator events
kubectl describe pod -l app.kubernetes.io/instance=kafka-operator
```

**Common Solutions:**
```bash
# 1. Restart operator deployment
kubectl rollout restart deployment kafka-operator-operator

# 2. Check RBAC permissions
kubectl auth can-i create kafkaclusters --as=system:serviceaccount:kafka:kafka-operator-operator

# 3. Reinstall operator
helm uninstall kafka-operator -n kafka
helm install kafka-operator oci://ghcr.io/adobe/helm-charts/kafka-operator \
  --namespace=kafka \
  --set webhook.enabled=false \
  --version 0.28.0-adobe-20250923
```

### Issue: Kafka Brokers Not Starting

**Symptoms:**
- Broker pods stuck in `Pending` or `Init` state
- Brokers failing health checks

**Diagnosis:**
```bash
# Check broker pod status
kubectl get pods -l kafka_cr=kafka -o wide

# Check specific broker logs
BROKER_POD=$(kubectl get pods -l kafka_cr=kafka -o jsonpath='{.items[0].metadata.name}')
kubectl logs $BROKER_POD --tail=100

# Check pod events
kubectl describe pod $BROKER_POD
```

**Common Solutions:**
```bash
# 1. Check resource constraints
kubectl describe nodes | grep -A 5 "Allocated resources"

# 2. Check storage class
kubectl get storageclass

# 3. Check ZooKeeper connectivity
kubectl run zk-test --rm -i --tty=true \
  --image=busybox \
  --restart=Never \
  -- telnet zk-client.zookeeper 2181

# 4. Force broker recreation
kubectl delete pod $BROKER_POD
```

## 3. Connectivity Issues

### Issue: Cannot Connect to Kafka Cluster

**Symptoms:**
- Timeout errors when connecting to Kafka
- DNS resolution failures

**Diagnosis:**
```bash
# Test DNS resolution
kubectl run dns-test --rm -i --tty=true \
  --image=busybox \
  --restart=Never \
  -- nslookup kafka-headless.kafka.svc.cluster.local

# Test port connectivity
kubectl run port-test --rm -i --tty=true \
  --image=busybox \
  --restart=Never \
  -- telnet kafka-headless 29092

# Check service endpoints
kubectl get endpoints kafka-headless
```

**Solutions:**
```bash
# 1. Verify service configuration
kubectl get svc kafka-headless -o yaml

# 2. Check network policies
kubectl get networkpolicy -A

# 3. Restart CoreDNS (if DNS issues)
kubectl rollout restart deployment coredns -n kube-system
```

### Issue: External Access Not Working

**Diagnosis:**
```bash
# Check external services
kubectl get svc | grep LoadBalancer

# Check ingress configuration
kubectl get ingress -A

# Test external connectivity
kubectl run external-test --rm -i --tty=true \
  --image=ghcr.io/adobe/koperator/kafka:2.13-3.9.1 \
  --restart=Never \
  -- /opt/kafka/bin/kafka-broker-api-versions.sh \
  --bootstrap-server <EXTERNAL_IP>:<EXTERNAL_PORT>
```

## 4. Performance Issues

### Issue: High Latency or Low Throughput

**Diagnosis:**
```bash
# Check broker resource usage
kubectl top pods -n kafka

# Check JVM metrics
kubectl exec -n kafka $BROKER_POD -- jstat -gc 1

# Check disk I/O
kubectl exec -n kafka $BROKER_POD -- iostat -x 1 5

# Check network metrics
kubectl exec -n kafka $BROKER_POD -- ss -tuln
```

**Performance Tuning:**
```bash
# 1. Increase broker resources
kubectl patch kafkacluster kafka --type='merge' -p='
{
  "spec": {
    "brokerConfigGroups": {
      "default": {
        "resourceRequirements": {
          "requests": {
            "cpu": "2",
            "memory": "4Gi"
          },
          "limits": {
            "cpu": "4",
            "memory": "8Gi"
          }
        }
      }
    }
  }
}'

# 2. Optimize JVM settings
kubectl patch kafkacluster kafka --type='merge' -p='
{
  "spec": {
    "brokerConfigGroups": {
      "default": {
        "jvmPerformanceOpts": "-server -XX:+UseG1GC -XX:MaxGCPauseMillis=20 -XX:InitiatingHeapOccupancyPercent=35 -Xms4g -Xmx4g"
      }
    }
  }
}'
```

### Issue: Disk Space Problems

**Diagnosis:**
```bash
# Check disk usage in broker pods
for pod in $(kubectl get pods -l kafka_cr=kafka -o jsonpath='{.items[*].metadata.name}'); do
  echo "=== $pod ==="
  kubectl exec $pod -- df -h /kafka-logs
done

# Check PVC usage
kubectl get pvc | grep kafka
```

**Solutions:**
```bash
# 1. Increase PVC size (if storage class supports expansion)
kubectl patch pvc kafka-101-storage-0 -p='{"spec":{"resources":{"requests":{"storage":"20Gi"}}}}'

# 2. Configure log retention
kubectl patch kafkacluster kafka --type='merge' -p='
{
  "spec": {
    "brokerConfigGroups": {
      "default": {
        "config": {
          "log.retention.hours": "168",
          "log.segment.bytes": "1073741824",
          "log.retention.check.interval.ms": "300000"
        }
      }
    }
  }
}'
```

## 5. Monitoring Issues

### Issue: Metrics Not Appearing in Prometheus

**Diagnosis:**
```bash
# Check ServiceMonitor
kubectl get servicemonitor -n kafka

# Check Prometheus targets
kubectl port-forward -n default svc/monitoring-kube-prometheus-prometheus 9090 &
curl -s "http://localhost:9090/api/v1/targets" | jq '.data.activeTargets[] | select(.labels.job | contains("kafka"))'

# Check metrics endpoints
kubectl exec -n kafka $BROKER_POD -- curl -s localhost:9020/metrics | head -10
```

**Solutions:**
```bash
# 1. Recreate ServiceMonitor
kubectl delete servicemonitor kafka-servicemonitor -n kafka
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
EOF

# 2. Check Prometheus configuration
kubectl get prometheus -o yaml | grep -A 10 serviceMonitorSelector
```

### Issue: Grafana Dashboard Not Loading

**Diagnosis:**
```bash
# Check Grafana pod
kubectl get pods -l app.kubernetes.io/name=grafana

# Check dashboard ConfigMap
kubectl get configmap -l grafana_dashboard=1

# Check Grafana logs
kubectl logs -l app.kubernetes.io/name=grafana
```

**Solutions:**
```bash
# 1. Restart Grafana
kubectl rollout restart deployment monitoring-grafana

# 2. Recreate dashboard ConfigMap
kubectl delete configmap kafka-looking-glass-dashboard
# Then recreate using the configuration from the deployment section
```

## 6. Operator Issues

### Issue: KafkaCluster Resource Not Reconciling

**Diagnosis:**
```bash
# Check operator logs for errors
kubectl logs -l app.kubernetes.io/instance=kafka-operator -c manager --tail=100

# Check KafkaCluster status
kubectl describe kafkacluster kafka

# Check operator events
kubectl get events --field-selector involvedObject.kind=KafkaCluster
```

**Solutions:**
```bash
# 1. Restart operator
kubectl rollout restart deployment kafka-operator-operator

# 2. Check CRD versions
kubectl get crd kafkaclusters.kafka.banzaicloud.io -o yaml | grep version

# 3. Force reconciliation
kubectl annotate kafkacluster kafka kubectl.kubernetes.io/restartedAt="$(date +%Y-%m-%dT%H:%M:%S%z)"
```

## 7. Common Error Patterns

### Error: "No space left on device"

**Solution:**
```bash
# Check disk usage
kubectl exec -n kafka $BROKER_POD -- df -h

# Clean up old log segments
kubectl exec -n kafka $BROKER_POD -- find /kafka-logs -name "*.log" -mtime +7 -delete

# Increase PVC size or configure retention
```

### Error: "Connection refused"

**Solution:**
```bash
# Check if broker is listening
kubectl exec -n kafka $BROKER_POD -- netstat -tuln | grep 9092

# Check broker configuration
kubectl exec -n kafka $BROKER_POD -- cat /opt/kafka/config/server.properties | grep listeners

# Restart broker if needed
kubectl delete pod $BROKER_POD
```

### Error: "ZooKeeper connection timeout"

**Solution:**
```bash
# Check ZooKeeper status
kubectl get pods -n zookeeper

# Test ZooKeeper connectivity
kubectl run zk-test --rm -i --tty=true \
  --image=busybox \
  --restart=Never \
  -- telnet zk-client.zookeeper 2181

# Check ZooKeeper logs
kubectl logs -n zookeeper zk-0
```

## 8. Monitoring Access

### Quick Access to Monitoring Tools

```bash
# Function to start all monitoring port-forwards
function start_monitoring() {
    echo "Starting monitoring port-forwards..."
    
    # Prometheus
    kubectl port-forward -n default svc/monitoring-kube-prometheus-prometheus 9090 &
    echo "Prometheus: http://localhost:9090"
    
    # Grafana
    kubectl port-forward -n default svc/monitoring-grafana 3000:80 &
    echo "Grafana: http://localhost:3000"
    echo "Grafana password: $(kubectl get secret --namespace default monitoring-grafana -o jsonpath="{.data.admin-password}" | base64 --decode)"
    
    # AlertManager
    kubectl port-forward -n default svc/monitoring-kube-prometheus-alertmanager 9093 &
    echo "AlertManager: http://localhost:9093"
    
    # Cruise Control
    kubectl port-forward -n kafka svc/kafka-cruisecontrol-svc 8090:8090 &
    echo "Cruise Control: http://localhost:8090"
    
    echo "All monitoring tools are now accessible!"
}

# Run the function
start_monitoring
```

### Stop All Port-Forwards

```bash
# Function to stop all port-forwards
function stop_monitoring() {
    echo "Stopping all port-forwards..."
    pkill -f "kubectl port-forward"
    echo "All port-forwards stopped."
}
```

## 9. Emergency Procedures

### Complete Cluster Reset

```bash
# WARNING: This will delete all data!
function emergency_reset() {
    echo "WARNING: This will delete all Kafka data!"
    read -p "Are you sure? (yes/no): " confirm
    
    if [ "$confirm" = "yes" ]; then
        # Delete KafkaCluster
        kubectl delete kafkacluster kafka -n kafka
        
        # Delete all Kafka pods
        kubectl delete pods -l kafka_cr=kafka -n kafka --force --grace-period=0
        
        # Delete PVCs (this deletes data!)
        kubectl delete pvc -l app=kafka -n kafka
        
        # Recreate cluster
        echo "Recreate your KafkaCluster resource to start fresh"
    else
        echo "Reset cancelled"
    fi
}
```

### Backup Critical Data

```bash
# Backup ZooKeeper data
kubectl exec -n zookeeper zk-0 -- tar czf /tmp/zk-backup.tar.gz /data

# Copy backup locally
kubectl cp zookeeper/zk-0:/tmp/zk-backup.tar.gz ./zk-backup-$(date +%Y%m%d).tar.gz

# Backup Kafka topic metadata
kubectl run kafka-backup --rm -i --tty=true \
  --image=ghcr.io/adobe/koperator/kafka:2.13-3.9.1 \
  --restart=Never \
  -- /opt/kafka/bin/kafka-topics.sh \
  --bootstrap-server kafka-headless:29092 \
  --list > kafka-topics-backup-$(date +%Y%m%d).txt
```

## 10. Getting Help

### Collect Support Information

```bash
# Generate support bundle
function collect_support_info() {
    local output_dir="kafka-support-$(date +%Y%m%d-%H%M%S)"
    mkdir -p $output_dir
    
    # Cluster information
    kubectl cluster-info > $output_dir/cluster-info.txt
    kubectl get nodes -o wide > $output_dir/nodes.txt
    
    # Kafka resources
    kubectl get kafkacluster kafka -n kafka -o yaml > $output_dir/kafkacluster.yaml
    kubectl get pods -n kafka -o wide > $output_dir/kafka-pods.txt
    kubectl get svc -n kafka > $output_dir/kafka-services.txt
    kubectl get pvc -n kafka > $output_dir/kafka-pvcs.txt
    
    # Logs
    kubectl logs -l app.kubernetes.io/instance=kafka-operator -c manager --tail=1000 > $output_dir/operator-logs.txt
    
    # Events
    kubectl get events -n kafka --sort-by=.metadata.creationTimestamp > $output_dir/kafka-events.txt
    
    # Create archive
    tar czf $output_dir.tar.gz $output_dir
    rm -rf $output_dir
    
    echo "Support bundle created: $output_dir.tar.gz"
}

# Run support collection
collect_support_info
```

## Next Steps

You've now completed the comprehensive Kafka on Kubernetes tutorial! For production deployments, consider:

1. **Security hardening** - Enable SSL/TLS, SASL authentication
2. **Backup strategies** - Implement regular data backups
3. **Monitoring alerts** - Configure production alerting rules
4. **Capacity planning** - Size resources for production workloads
5. **Disaster recovery** - Plan for multi-region deployments

---

> **Remember**: Always test changes in a development environment before applying them to production clusters.
