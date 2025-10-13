---
title: Dependencies Installation
weight: 30
---

# Dependencies Installation

Before installing the Koperator, we need to set up several dependencies that are required for a complete Kafka deployment. This section covers the installation of cert-manager, ZooKeeper operator, and Prometheus operator.

## Overview

The dependencies we'll install are:

1. **cert-manager**: Manages TLS certificates for secure communication
2. **ZooKeeper Operator**: Manages ZooKeeper clusters (required for traditional Kafka deployments)
3. **Prometheus Operator**: Provides monitoring and alerting capabilities

## 1. Install cert-manager

cert-manager is essential for TLS certificate management in Kafka deployments.

### Install cert-manager CRDs

First, install the Custom Resource Definitions:

```bash
# Install cert-manager CRDs
kubectl create --validate=false -f https://github.com/cert-manager/cert-manager/releases/download/v1.18.2/cert-manager.crds.yaml
```

**Expected output:**
```
customresourcedefinition.apiextensions.k8s.io/certificaterequests.cert-manager.io created
customresourcedefinition.apiextensions.k8s.io/certificates.cert-manager.io created
customresourcedefinition.apiextensions.k8s.io/challenges.acme.cert-manager.io created
customresourcedefinition.apiextensions.k8s.io/clusterissuers.cert-manager.io created
customresourcedefinition.apiextensions.k8s.io/issuers.cert-manager.io created
customresourcedefinition.apiextensions.k8s.io/orders.acme.cert-manager.io created
```

### Create cert-manager Namespace

```bash
# Create namespace for cert-manager
kubectl create namespace cert-manager
```

### Install cert-manager using Helm

```bash
# Add cert-manager Helm repository
helm repo add cert-manager https://charts.jetstack.io
helm repo update

# Install cert-manager
helm install cert-manager cert-manager/cert-manager \
  --namespace cert-manager \
  --version v1.18.2

# Wait for cert-manager to be ready
kubectl wait --for=condition=Available deployment --all -n cert-manager --timeout=300s
```

### Verify cert-manager Installation

```bash
# Check cert-manager pods
kubectl get pods -n cert-manager

# Check cert-manager services
kubectl get svc -n cert-manager

# Verify cert-manager is working
kubectl get certificates -A
```

**Expected output:**
```
NAME                                       READY   STATUS    RESTARTS   AGE
cert-manager-cainjector-7d55bf8f78-xyz123   1/1     Running   0          2m
cert-manager-webhook-97f8b47bc-abc456       1/1     Running   0          2m
cert-manager-7dd5854bb4-def789              1/1     Running   0          2m
```

## 2. Install ZooKeeper Operator

The ZooKeeper operator manages ZooKeeper clusters required by Kafka.

### Create ZooKeeper Namespace

```bash
# Create namespace for ZooKeeper
kubectl create namespace zookeeper
```

### Install ZooKeeper Operator CRDs

```bash
# Install ZooKeeper CRDs
kubectl create -f https://raw.githubusercontent.com/adobe/zookeeper-operator/master/config/crd/bases/zookeeper.pravega.io_zookeeperclusters.yaml
```

### Clone ZooKeeper Operator Repository

```bash
# Clone the ZooKeeper operator repository
cd $TUTORIAL_DIR
rm -rf /tmp/zookeeper-operator
git clone --single-branch --branch master https://github.com/adobe/zookeeper-operator /tmp/zookeeper-operator
cd /tmp/zookeeper-operator
```

### Install ZooKeeper Operator using Helm

```bash
# Install ZooKeeper operator
helm template zookeeper-operator \
  --namespace=zookeeper \
  --set crd.create=false \
  --set image.repository='adobe/zookeeper-operator' \
  --set image.tag='0.2.15-adobe-20250914' \
  ./charts/zookeeper-operator | kubectl create -n zookeeper -f -

# Wait for operator to be ready
kubectl wait --for=condition=Available deployment --all -n zookeeper --timeout=300s
```

### Deploy ZooKeeper Cluster

Create a 3-node ZooKeeper cluster:

```bash
# Create ZooKeeper cluster
kubectl create --namespace zookeeper -f - <<EOF
apiVersion: zookeeper.pravega.io/v1beta1
kind: ZookeeperCluster
metadata:
  name: zk
  namespace: zookeeper
spec:
  replicas: 3
  image:
    repository: adobe/zookeeper
    tag: 3.8.4-0.2.15-adobe-20250914
    pullPolicy: IfNotPresent
  config:
    initLimit: 10
    tickTime: 2000
    syncLimit: 5
  probes:
    livenessProbe:
      initialDelaySeconds: 41
  persistence:
    reclaimPolicy: Delete
    spec:
      accessModes:
      - ReadWriteOnce
      resources:
        requests:
          storage: 20Gi
EOF
```

### Verify ZooKeeper Installation

```bash
# Check ZooKeeper cluster status
kubectl get zookeepercluster -n zookeeper -o wide

# Watch ZooKeeper cluster creation
kubectl get pods -n zookeeper -w
# Press Ctrl+C to stop watching when all pods are running

# Check ZooKeeper services
kubectl get svc -n zookeeper

# Verify ZooKeeper cluster is ready
kubectl wait --for=condition=Ready pod --all -n zookeeper --timeout=600s
```

**Expected output:**
```
NAME   REPLICAS   READY REPLICAS   VERSION                           DESIRED VERSION                   INTERNAL ENDPOINT    EXTERNAL ENDPOINT   AGE
zk     3          3               3.8.4-0.2.15-adobe-20250914      3.8.4-0.2.15-adobe-20250914      zk-client:2181                           5m
```

## 3. Install Prometheus Operator

The Prometheus operator provides comprehensive monitoring for Kafka and ZooKeeper.

### Install Prometheus Operator CRDs

```bash
# Install Prometheus Operator CRDs
kubectl create -f https://raw.githubusercontent.com/prometheus-operator/prometheus-operator/master/example/prometheus-operator-crd/monitoring.coreos.com_alertmanagers.yaml
kubectl create -f https://raw.githubusercontent.com/prometheus-operator/prometheus-operator/master/example/prometheus-operator-crd/monitoring.coreos.com_alertmanagerconfigs.yaml
kubectl create -f https://raw.githubusercontent.com/prometheus-operator/prometheus-operator/master/example/prometheus-operator-crd/monitoring.coreos.com_prometheuses.yaml
kubectl create -f https://raw.githubusercontent.com/prometheus-operator/prometheus-operator/master/example/prometheus-operator-crd/monitoring.coreos.com_prometheusrules.yaml
kubectl create -f https://raw.githubusercontent.com/prometheus-operator/prometheus-operator/master/example/prometheus-operator-crd/monitoring.coreos.com_servicemonitors.yaml
kubectl create -f https://raw.githubusercontent.com/prometheus-operator/prometheus-operator/master/example/prometheus-operator-crd/monitoring.coreos.com_podmonitors.yaml
kubectl create -f https://raw.githubusercontent.com/prometheus-operator/prometheus-operator/master/example/prometheus-operator-crd/monitoring.coreos.com_thanosrulers.yaml
kubectl create -f https://raw.githubusercontent.com/prometheus-operator/prometheus-operator/master/example/prometheus-operator-crd/monitoring.coreos.com_probes.yaml
```

### Install Prometheus Stack using Helm

```bash
# Add Prometheus community Helm repository
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update

# Install kube-prometheus-stack (includes Prometheus, Grafana, and AlertManager)
helm install monitoring \
  --namespace=default \
  prometheus-community/kube-prometheus-stack \
  --set prometheusOperator.createCustomResource=false

# Wait for monitoring stack to be ready
kubectl wait --for=condition=Available deployment --all -n default --timeout=600s
```

### Verify Prometheus Installation

```bash
# Check monitoring pods
kubectl get pods -l release=monitoring

# Check monitoring services
kubectl get svc -l release=monitoring

# Check Prometheus targets (optional)
kubectl get prometheus -o wide
```

**Expected output:**
```
NAME                                                   READY   STATUS    RESTARTS   AGE
monitoring-kube-prometheus-prometheus-node-exporter-*  1/1     Running   0          3m
monitoring-kube-state-metrics-*                       1/1     Running   0          3m
monitoring-prometheus-operator-*                      1/1     Running   0          3m
monitoring-grafana-*                                  3/3     Running   0          3m
```

## Access Monitoring Dashboards

### Get Grafana Admin Password

```bash
# Get Grafana admin password
kubectl get secret --namespace default monitoring-grafana \
  -o jsonpath="{.data.admin-password}" | base64 --decode
echo ""
```

### Set Up Port Forwarding (Optional)

You can access the monitoring dashboards using port forwarding:

```bash
# Prometheus (in a separate terminal)
kubectl --namespace default port-forward svc/monitoring-kube-prometheus-prometheus 9090 &

# Grafana (in a separate terminal)
kubectl --namespace default port-forward svc/monitoring-grafana 3000:80 &

# AlertManager (in a separate terminal)
kubectl --namespace default port-forward svc/monitoring-kube-prometheus-alertmanager 9093 &
```

Access the dashboards at:
- **Prometheus**: http://localhost:9090
- **Grafana**: http://localhost:3000 (admin/[password from above])
- **AlertManager**: http://localhost:9093

## Verification Summary

Verify all dependencies are properly installed:

```bash
echo "=== cert-manager ==="
kubectl get pods -n cert-manager

echo -e "\n=== ZooKeeper ==="
kubectl get pods -n zookeeper
kubectl get zookeepercluster -n zookeeper

echo -e "\n=== Monitoring ==="
kubectl get pods -l release=monitoring

echo -e "\n=== All Namespaces ==="
kubectl get namespaces
```

## Troubleshooting

### cert-manager Issues

```bash
# Check cert-manager logs
kubectl logs -n cert-manager deployment/cert-manager

# Check webhook connectivity
kubectl get validatingwebhookconfigurations
```

### ZooKeeper Issues

```bash
# Check ZooKeeper operator logs
kubectl logs -n zookeeper deployment/zookeeper-operator

# Check ZooKeeper cluster events
kubectl describe zookeepercluster zk -n zookeeper
```

### Prometheus Issues

```bash
# Check Prometheus operator logs
kubectl logs -l app.kubernetes.io/name=prometheus-operator

# Check Prometheus configuration
kubectl get prometheus -o yaml
```

## Next Steps

With all dependencies successfully installed, you can now proceed to install the Koperator itself. Continue to the [Koperator Installation]({{< relref "koperator-install.md" >}}) section.

---

> **Note**: The monitoring stack will start collecting metrics immediately. You can explore the Grafana dashboards to see cluster metrics even before deploying Kafka.
