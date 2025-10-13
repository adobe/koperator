---
title: Kubernetes Cluster Setup
weight: 20
---

# Kubernetes Cluster Setup

In this section, you'll create a multi-node Kubernetes cluster using kind (Kubernetes in Docker) that simulates a production-like environment with multiple availability zones.

## Cluster Architecture

We'll create a 7-node cluster with the following configuration:

- **1 Control Plane node**: Manages the Kubernetes API and cluster state
- **6 Worker nodes**: Distributed across 3 simulated availability zones (2 nodes per AZ)

This setup allows us to demonstrate:
- Multi-AZ deployment patterns
- Rack awareness for Kafka brokers
- High availability configurations
- Realistic failure scenarios

## Create Cluster Configuration

First, create the kind cluster configuration file:

```bash
cd $TUTORIAL_DIR

# Create kind configuration directory
mkdir -p ~/.kind

# Create the cluster configuration
cat > ~/.kind/kind-config.yaml <<EOF
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
  - role: control-plane
    kubeadmConfigPatches:
    - |
      kind: InitConfiguration
      nodeRegistration:
        kubeletExtraArgs:
          node-labels: "ingress-ready=true"
  - role: worker
  - role: worker
  - role: worker
  - role: worker
  - role: worker
  - role: worker
containerdConfigPatches:
- |-
  [plugins."io.containerd.grpc.v1.cri".containerd]
    snapshotter = "overlayfs"
  [plugins."io.containerd.grpc.v1.cri".registry.mirrors."localhost:5000"]
    endpoint = ["http://localhost:5000"]
EOF
```

## Create the Kubernetes Cluster

Now create the kind cluster:

```bash
# Create the cluster (this may take 5-10 minutes)
kind create cluster \
  --name kafka \
  --config ~/.kind/kind-config.yaml \
  --image kindest/node:v1.33.4

# Wait for cluster to be ready
echo "Waiting for cluster to be ready..."
kubectl wait --for=condition=Ready nodes --all --timeout=300s
```

**Expected output:**
```
Creating cluster "kafka" ...
 ✓ Ensuring node image (kindest/node:v1.33.4) 🖼
 ✓ Preparing nodes 📦 📦 📦 📦 📦 📦 📦
 ✓ Writing configuration 📜
 ✓ Starting control-plane 🕹️
 ✓ Installing CNI 🔌
 ✓ Installing StorageClass 💾
 ✓ Joining worker nodes 🚜
Set kubectl context to "kind-kafka"
You can now use your cluster with:

kubectl cluster-info --context kind-kafka
```

## Verify Cluster Creation

Verify that all nodes are running and ready:

```bash
# Check cluster info
kubectl cluster-info --context kind-kafka

# List all nodes
kubectl get nodes -o wide

# Check node status
kubectl get nodes --show-labels
```

**Expected output:**
```
NAME                  STATUS   ROLES           AGE   VERSION
kafka-control-plane   Ready    control-plane   2m    v1.33.4
kafka-worker          Ready    <none>          2m    v1.33.4
kafka-worker2         Ready    <none>          2m    v1.33.4
kafka-worker3         Ready    <none>          2m    v1.33.4
kafka-worker4         Ready    <none>          2m    v1.33.4
kafka-worker5         Ready    <none>          2m    v1.33.4
kafka-worker6         Ready    <none>          2m    v1.33.4
```

## Configure Multi-AZ Simulation

To simulate a multi-availability zone environment, we'll label the nodes with region and zone information:

### 1. Label Nodes with Region

First, label all worker nodes with the same region:

```bash
# Label all worker nodes with region
kubectl label nodes \
  kafka-worker \
  kafka-worker2 \
  kafka-worker3 \
  kafka-worker4 \
  kafka-worker5 \
  kafka-worker6 \
  topology.kubernetes.io/region=region1
```

### 2. Label Nodes with Availability Zones

Now distribute the worker nodes across three availability zones:

```bash
# AZ1: kafka-worker and kafka-worker2
kubectl label nodes kafka-worker kafka-worker2 \
  topology.kubernetes.io/zone=az1

# AZ2: kafka-worker3 and kafka-worker4
kubectl label nodes kafka-worker3 kafka-worker4 \
  topology.kubernetes.io/zone=az2

# AZ3: kafka-worker5 and kafka-worker6
kubectl label nodes kafka-worker5 kafka-worker6 \
  topology.kubernetes.io/zone=az3
```

### 3. Verify Zone Configuration

Check that the zone labels are correctly applied:

```bash
# Display nodes with region and zone labels
kubectl get nodes \
  --label-columns=topology.kubernetes.io/region,topology.kubernetes.io/zone

# Show detailed node information
kubectl describe nodes | grep -E "Name:|topology.kubernetes.io"
```

**Expected output:**
```
NAME                  STATUS   ROLES           AGE   VERSION   REGION    ZONE
kafka-control-plane   Ready    control-plane   5m    v1.33.4   <none>    <none>
kafka-worker          Ready    <none>          5m    v1.33.4   region1   az1
kafka-worker2         Ready    <none>          5m    v1.33.4   region1   az1
kafka-worker3         Ready    <none>          5m    v1.33.4   region1   az2
kafka-worker4         Ready    <none>          5m    v1.33.4   region1   az2
kafka-worker5         Ready    <none>          5m    v1.33.4   region1   az2
kafka-worker6         Ready    <none>          5m    v1.33.4   region1   az3
```

## Configure kubectl Context

Ensure you're using the correct kubectl context:

```bash
# Set the current context to the kind cluster
kubectl config use-context kind-kafka

# Verify current context
kubectl config current-context

# Test cluster access
kubectl get namespaces
```

## Cluster Resource Verification

Check the cluster's available resources:

```bash
# Check node resources
kubectl top nodes 2>/dev/null || echo "Metrics server not yet available"

# Check cluster capacity
kubectl describe nodes | grep -A 5 "Capacity:"

# Check storage classes
kubectl get storageclass

# Check default namespace
kubectl get all
```

## Understanding the Cluster Layout

Your cluster now has the following topology:

```
┌─────────────────────────────────────────────────────────────────┐
│                         kind-kafka cluster                      │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ┌─────────────────┐                                            │
│  │ Control Plane   │                                            │
│  │ kafka-control-  │                                            │
│  │ plane           │                                            │
│  └─────────────────┘                                            │
│                                                                 │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐  │
│  │      AZ1        │  │      AZ2        │  │      AZ3        │  │
│  │                 │  │                 │  │                 │  │
│  │ kafka-worker    │  │ kafka-worker3   │  │ kafka-worker5   │  │
│  │ kafka-worker2   │  │ kafka-worker4   │  │ kafka-worker6   │  │
│  │                 │  │                 │  │                 │  │
│  └─────────────────┘  └─────────────────┘  └─────────────────┘  │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

## Troubleshooting

### Cluster Creation Issues

If cluster creation fails:

```bash
# Delete the failed cluster
kind delete cluster --name kafka

# Check Docker resources
docker system df
docker system prune -f

# Retry cluster creation
kind create cluster --name kafka --config ~/.kind/kind-config.yaml --image kindest/node:v1.33.4
```

### Node Not Ready

If nodes are not ready:

```bash
# Check node status
kubectl describe nodes

# Check system pods
kubectl get pods -n kube-system

# Check kubelet logs (from Docker)
docker logs kafka-worker
```

### Context Issues

If kubectl context is not set correctly:

```bash
# List available contexts
kubectl config get-contexts

# Set the correct context
kubectl config use-context kind-kafka

# Verify
kubectl config current-context
```

## Cluster Cleanup (Optional)

If you need to start over:

```bash
# Delete the cluster
kind delete cluster --name kafka

# Verify deletion
kind get clusters

# Remove configuration
rm ~/.kind/kind-config.yaml
```

## Next Steps

With your Kubernetes cluster ready and properly configured with multi-AZ simulation, you can now proceed to install the required dependencies. Continue to the [Dependencies Installation]({{< relref "dependencies.md" >}}) section.

---

> **Note**: The cluster will persist until you explicitly delete it with `kind delete cluster --name kafka`. You can stop and start Docker without losing your cluster state.
