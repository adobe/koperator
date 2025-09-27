---
title: Istio Integration
weight: 200
---

# Istio Integration with Koperator

Koperator now supports Istio integration using standard Istio resources, providing advanced service mesh capabilities for Kafka clusters. This integration replaces the deprecated banzaicloud istio-operator with a more robust approach using standard Kubernetes and Istio resources.

## Overview

The Istio integration in Koperator provides:

- **Service Mesh Capabilities**: Full Istio service mesh integration for Kafka clusters
- **Traffic Management**: Advanced traffic routing and load balancing
- **Security**: mTLS encryption and authentication
- **Observability**: Enhanced monitoring and tracing capabilities
- **Gateway Management**: Automatic Istio Gateway and VirtualService creation
- **No Control Plane Dependency**: Works with any Istio installation

## Prerequisites

Before using Istio integration with Koperator, ensure you have:

1. **Istio Installation**: Any Istio installation (operator-based or manual)
2. **Kubernetes Cluster**: Version 1.19+ with sufficient resources
3. **Istio CRDs**: Istio Custom Resource Definitions installed

## Installation

### 1. Install Istio (Optional)

If you don't have Istio installed, you can install it using any method:

```bash
# Option 1: Using Istio operator
kubectl apply -f https://github.com/istio/istio/releases/download/1.19.0/istio-1.19.0-linux-amd64.tar.gz
istioctl install --set values.defaultRevision=default

# Option 2: Using Helm
helm repo add istio https://istio-release.storage.googleapis.com/charts
helm repo update
helm install istio-base istio/base -n istio-system --create-namespace
helm install istiod istio/istiod -n istio-system --wait
```

### 2. Verify Istio Installation

```bash
# Check Istio control plane status (if installed)
kubectl get pods -n istio-system

# Verify Istio CRDs are available
kubectl get crd | grep istio
```

## Configuration

### Basic Istio Configuration

Configure your KafkaCluster to use Istio ingress:

```yaml
apiVersion: kafka.banzaicloud.io/v1beta1
kind: KafkaCluster
metadata:
  name: kafka
  namespace: kafka
spec:
  ingressController: "istioingress"
  istioIngressConfig:
    gatewayConfig:
      mode: ISTIO_MUTUAL
  # ... rest of your Kafka configuration
```

**Note**: The `istioControlPlane` configuration is no longer required as Koperator now creates standard Kubernetes resources that work with any Istio installation.

### Advanced Configuration Options

#### Istio Ingress Configuration

```yaml
spec:
  istioIngressConfig:
    # Gateway configuration
    gatewayConfig:
      mode: ISTIO_MUTUAL  # or SIMPLE for non-mTLS
    
    # Resource limits and requests
    resources:
      requests:
        cpu: "100m"
        memory: "128Mi"
      limits:
        cpu: "2000m"
        memory: "1024Mi"
    
    # Replica configuration
    replicas: 2
    
    # Node selector for gateway placement
    nodeSelector:
      kubernetes.io/os: linux
    
    # Tolerations for gateway scheduling
    tolerations:
    - key: "istio"
      operator: "Equal"
      value: "true"
      effect: "NoSchedule"
    
    # Environment variables
    envs:
    - name: CUSTOM_VAR
      value: "custom-value"
    
    # Annotations for the gateway
    annotations:
      custom.annotation: "value"
    
    # Service annotations
    serviceAnnotations:
      service.beta.kubernetes.io/aws-load-balancer-type: "nlb"
```

## Architecture Changes

### Previous Architecture (banzaicloud istio-operator)

The previous implementation used:
- `IstioMeshGateway` custom resources
- banzaicloud istio-operator dependencies
- Custom Istio operator APIs

### New Architecture (Standard Istio Resources)

The new implementation uses:
- Standard Kubernetes `Deployment` and `Service` resources
- Native Istio `Gateway` and `VirtualService` resources
- No dependency on specific Istio control plane or operator

### Resource Creation

When you create a KafkaCluster with Istio integration, Koperator automatically creates:

1. **Kubernetes Deployment**: Istio proxy deployment with `docker.io/istio/proxyv2:latest` image
2. **Kubernetes Service**: Load balancer service for external access
3. **Istio Gateway**: Routes external traffic to Kafka brokers
4. **VirtualService**: Defines routing rules for the gateway

The implementation creates standard Kubernetes resources that work with any Istio installation, making it more flexible and compatible.

## Security Features

### mTLS Configuration

The Istio integration supports mutual TLS (mTLS) for secure communication:

```yaml
spec:
  istioIngressConfig:
    gatewayConfig:
      mode: ISTIO_MUTUAL  # Enables mTLS
```

### Authentication and Authorization

Istio provides additional security features:
- **PeerAuthentication**: Configure mTLS policies
- **AuthorizationPolicy**: Define access control rules
- **RequestAuthentication**: JWT token validation

## Monitoring and Observability

### Metrics

Istio integration provides enhanced metrics:
- **Traffic Metrics**: Request rates, latency, error rates
- **Gateway Metrics**: Istio gateway performance
- **Service Mesh Metrics**: End-to-end observability

### Tracing

Distributed tracing is automatically enabled:
- **Jaeger Integration**: Automatic trace collection
- **Zipkin Support**: Alternative tracing backend
- **Custom Trace Sampling**: Configurable sampling rates

## Troubleshooting

### Common Issues

1. **Istio Control Plane Not Found**
   ```bash
   # Verify Istio control plane is running
   kubectl get pods -n istio-system
   ```

2. **Gateway Not Receiving Traffic**
   ```bash
   # Check gateway status
   kubectl get gateway -n kafka
   kubectl describe gateway kafka-gateway -n kafka
   ```

3. **mTLS Configuration Issues**
   ```bash
   # Verify peer authentication
   kubectl get peerauthentication -n kafka
   ```

### Debugging Commands

```bash
# Check Istio proxy status
istioctl proxy-status

# Verify configuration
istioctl analyze

# Check gateway configuration
kubectl get gateway,virtualservice -n kafka

# View Istio logs
kubectl logs -n kafka -l app=istio-ingressgateway
```

## Migration from banzaicloud istio-operator

If you're migrating from the deprecated banzaicloud istio-operator:

1. **Remove old dependencies**: Uninstall banzaicloud istio-operator
2. **Install upstream Istio**: Follow the installation steps above
3. **Update configurations**: Update your KafkaCluster specs
4. **Test thoroughly**: Verify all functionality works as expected

## Best Practices

1. **Resource Planning**: Allocate sufficient resources for Istio components
2. **Security**: Always use mTLS in production environments
3. **Monitoring**: Set up comprehensive monitoring and alerting
4. **Testing**: Thoroughly test Istio configurations in non-production environments
5. **Updates**: Keep Istio and Koperator versions compatible

## Examples

### Complete KafkaCluster with Istio

```yaml
apiVersion: kafka.banzaicloud.io/v1beta1
kind: KafkaCluster
metadata:
  name: kafka
  namespace: kafka
spec:
  headlessServiceEnabled: false
  ingressController: "istioingress"
  istioIngressConfig:
    gatewayConfig:
      mode: ISTIO_MUTUAL
    resources:
      requests:
        cpu: "100m"
        memory: "128Mi"
      limits:
        cpu: "2000m"
        memory: "1024Mi"
    replicas: 2
    annotations:
      sidecar.istio.io/inject: "true"
  zkAddresses:
    - "zookeeper-server-client.zookeeper:2181"
  clusterImage: "ghcr.io/adobe/koperator/kafka:2.13-3.9.1"
  brokers:
    - id: 0
      brokerConfigGroup: "default"
    - id: 1
      brokerConfigGroup: "default"
    - id: 2
      brokerConfigGroup: "default"
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
  listenersConfig:
    internalListeners:
      - type: "plaintext"
        name: "internal"
        containerPort: 29092
        usedForInnerBrokerCommunication: true
    externalListeners:
      - type: "plaintext"
        name: "external"
        externalStartingPort: 19090
        containerPort: 9094
```

## Support

For issues related to Istio integration:

1. **Koperator Issues**: Report to the Koperator GitHub repository
2. **Istio Issues**: Report to the Istio GitHub repository
3. **Documentation**: Check the official Istio documentation
4. **Community**: Join the Istio and Koperator community discussions
