---
title: Koperator Installation
weight: 40
---

# Koperator Installation

In this section, you'll install the Koperator (formerly BanzaiCloud Kafka Operator), which will manage your Kafka clusters on Kubernetes. The installation process involves installing Custom Resource Definitions (CRDs) and the operator itself.

## Overview

The Koperator installation consists of:

1. **Creating the Kafka namespace**
2. **Installing Koperator CRDs** (Custom Resource Definitions)
3. **Installing the Koperator using Helm**
4. **Verifying the installation**

## 1. Create Kafka Namespace

First, create a dedicated namespace for Kafka resources:

```bash
# Create namespace for Kafka
kubectl create namespace kafka

# Verify namespace creation
kubectl get namespaces | grep kafka
```

**Expected output:**
```
kafka         Active   10s
```

## 2. Install Koperator CRDs

The Koperator requires several Custom Resource Definitions to manage Kafka clusters, topics, and users.

### Install Required CRDs

```bash
# Install KafkaCluster CRD
kubectl apply --server-side -f https://raw.githubusercontent.com/adobe/koperator/refs/heads/master/config/base/crds/kafka.banzaicloud.io_kafkaclusters.yaml

# Install KafkaTopic CRD
kubectl apply --server-side -f https://raw.githubusercontent.com/adobe/koperator/refs/heads/master/config/base/crds/kafka.banzaicloud.io_kafkatopics.yaml

# Install KafkaUser CRD
kubectl apply --server-side -f https://raw.githubusercontent.com/adobe/koperator/refs/heads/master/config/base/crds/kafka.banzaicloud.io_kafkausers.yaml

# Install CruiseControlOperation CRD
kubectl apply --server-side -f https://raw.githubusercontent.com/adobe/koperator/refs/heads/master/config/base/crds/kafka.banzaicloud.io_cruisecontroloperations.yaml
```

### Verify CRD Installation

```bash
# Check that all CRDs are installed
kubectl get crd | grep kafka.banzaicloud.io

# Get detailed information about the CRDs
kubectl get crd kafkaclusters.kafka.banzaicloud.io -o yaml | head -20
```

**Expected output:**
```
cruisecontroloperations.kafka.banzaicloud.io   2024-01-15T10:30:00Z
kafkaclusters.kafka.banzaicloud.io             2024-01-15T10:30:00Z
kafkatopics.kafka.banzaicloud.io               2024-01-15T10:30:00Z
kafkausers.kafka.banzaicloud.io                2024-01-15T10:30:00Z
```

## 3. Install Koperator using Helm

Now install the Koperator using the OCI Helm chart:

```bash
# Install Koperator using OCI Helm chart
helm install kafka-operator oci://ghcr.io/adobe/helm-charts/kafka-operator \
  --namespace=kafka \
  --set webhook.enabled=false \
  --version 0.28.0-adobe-20250923

# Wait for the operator to be ready
kubectl wait --for=condition=Available deployment --all -n kafka --timeout=300s
```

**Expected output:**
```
Pulled: ghcr.io/adobe/helm-charts/kafka-operator:0.28.0-adobe-20250923
Digest: sha256:...
NAME: kafka-operator
LAST DEPLOYED: Mon Jan 15 10:35:00 2024
NAMESPACE: kafka
STATUS: deployed
REVISION: 1
```

## 4. Verify Koperator Installation

### Check Operator Pods

```bash
# Check Koperator pods
kubectl get pods -n kafka

# Check pod details
kubectl describe pods -n kafka

# Check operator logs
kubectl logs -l app.kubernetes.io/instance=kafka-operator -c manager -n kafka
```

**Expected output:**
```
NAME                                    READY   STATUS    RESTARTS   AGE
kafka-operator-operator-xyz123-abc456   2/2     Running   0          2m
```

### Check Operator Services

```bash
# Check services in kafka namespace
kubectl get svc -n kafka

# Check operator deployment
kubectl get deployment -n kafka
```

### Verify Operator Functionality

```bash
# Check if the operator is watching for KafkaCluster resources
kubectl get kafkaclusters -n kafka

# Check operator configuration
kubectl get deployment kafka-operator-operator -n kafka -o yaml | grep -A 10 -B 10 image:
```

## 5. Understanding Koperator Components

The Koperator installation includes several components:

### Manager Container
- **Purpose**: Main operator logic
- **Responsibilities**: Watches Kafka CRDs and manages Kafka clusters
- **Resource Management**: Creates and manages Kafka broker pods, services, and configurations

### Webhook (Disabled)
- **Purpose**: Admission control and validation
- **Status**: Disabled in this tutorial for simplicity
- **Production Note**: Should be enabled in production environments

### RBAC Resources
The operator creates several RBAC resources:
- ServiceAccount
- ClusterRole and ClusterRoleBinding
- Role and RoleBinding

```bash
# Check RBAC resources
kubectl get serviceaccount -n kafka
kubectl get clusterrole | grep kafka-operator
kubectl get rolebinding -n kafka
```

## 6. Operator Configuration

### View Operator Configuration

```bash
# Check operator deployment configuration
kubectl get deployment kafka-operator-operator -n kafka -o yaml

# Check operator environment variables
kubectl get deployment kafka-operator-operator -n kafka -o jsonpath='{.spec.template.spec.containers[0].env}' | jq .
```

### Key Configuration Options

The operator is configured with the following key settings:

- **Webhook disabled**: Simplifies the tutorial setup
- **Namespace**: Operates in the `kafka` namespace
- **Image**: Uses the official Adobe Koperator image
- **Version**: 0.28.0-adobe-20250923

## 7. Operator Capabilities

The Koperator provides the following capabilities:

### Kafka Cluster Management
- Automated broker deployment and scaling
- Rolling updates and configuration changes
- Persistent volume management
- Network policy configuration

### Security Features
- TLS/SSL certificate management
- SASL authentication support
- Network encryption
- User and ACL management

### Monitoring Integration
- JMX metrics exposure
- Prometheus integration
- Grafana dashboard support
- Custom alerting rules

### Advanced Features
- Cruise Control integration for rebalancing
- External access configuration
- Multi-AZ deployment support
- Rack awareness

## 8. Troubleshooting

### Operator Not Starting

If the operator pod is not starting:

```bash
# Check pod events
kubectl describe pod -l app.kubernetes.io/instance=kafka-operator -n kafka

# Check operator logs
kubectl logs -l app.kubernetes.io/instance=kafka-operator -c manager -n kafka --previous

# Check resource constraints
kubectl top pod -n kafka
```

### CRD Issues

If CRDs are not properly installed:

```bash
# Reinstall CRDs
kubectl delete crd kafkaclusters.kafka.banzaicloud.io
kubectl apply --server-side -f https://raw.githubusercontent.com/adobe/koperator/refs/heads/master/config/base/crds/kafka.banzaicloud.io_kafkaclusters.yaml

# Check CRD status
kubectl get crd kafkaclusters.kafka.banzaicloud.io -o yaml
```

### Helm Installation Issues

If Helm installation fails:

```bash
# Check Helm release status
helm list -n kafka

# Uninstall and reinstall
helm uninstall kafka-operator -n kafka
helm install kafka-operator oci://ghcr.io/adobe/helm-charts/kafka-operator \
  --namespace=kafka \
  --set webhook.enabled=false \
  --version 0.28.0-adobe-20250923
```

## 9. Verification Checklist

Before proceeding to the next section, ensure:

```bash
echo "=== Namespace ==="
kubectl get namespace kafka

echo -e "\n=== CRDs ==="
kubectl get crd | grep kafka.banzaicloud.io

echo -e "\n=== Operator Pod ==="
kubectl get pods -n kafka

echo -e "\n=== Operator Logs (last 10 lines) ==="
kubectl logs -l app.kubernetes.io/instance=kafka-operator -c manager -n kafka --tail=10

echo -e "\n=== Ready for Kafka Cluster Deployment ==="
kubectl get kafkaclusters -n kafka
```

**Expected final output:**
```
=== Namespace ===
NAME    STATUS   AGE
kafka   Active   10m

=== CRDs ===
cruisecontroloperations.kafka.banzaicloud.io   2024-01-15T10:30:00Z
kafkaclusters.kafka.banzaicloud.io             2024-01-15T10:30:00Z
kafkatopics.kafka.banzaicloud.io               2024-01-15T10:30:00Z
kafkausers.kafka.banzaicloud.io                2024-01-15T10:30:00Z

=== Operator Pod ===
NAME                                    READY   STATUS    RESTARTS   AGE
kafka-operator-operator-xyz123-abc456   2/2     Running   0          5m

=== Ready for Kafka Cluster Deployment ===
No resources found in kafka namespace.
```

## Next Steps

With the Koperator successfully installed and running, you're now ready to deploy a Kafka cluster. Continue to the [Kafka Cluster Deployment]({{< relref "kafka-deployment.md" >}}) section to create your first Kafka cluster.

---

> **Note**: The operator will continuously monitor the `kafka` namespace for KafkaCluster resources. Once you create a KafkaCluster resource, the operator will automatically provision the necessary Kafka infrastructure.
