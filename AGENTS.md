# AGENTS.md - Koperator Project Guide for AI Agents

## Project Overview

**Koperator** is a Kubernetes operator for managing Apache Kafka clusters on Kubernetes. Originally developed by Cisco/Banzai Cloud, now maintained by Adobe.

- **Language**: Go 1.25
- **Framework**: Kubebuilder v2 with controller-runtime v0.22.4
- **Primary CRD**: `KafkaCluster` (v1beta1)
- **Key Features**: Fine-grained broker management, Cruise Control integration, multiple ingress options (Envoy, Istio, Contour)

## Architecture

### Operator Pattern
This is a standard Kubernetes operator using:
- **controller-runtime** for reconciliation loops
- **Custom Resource Definitions (CRDs)** for declarative configuration
- **Webhooks** for validation and defaults
- **k8s-objectmatcher** for detecting resource drift

### Controllers
1. **KafkaClusterReconciler** - Main controller managing Kafka cluster lifecycle
2. **KafkaTopicReconciler** - Manages Kafka topics
3. **KafkaUserReconciler** - Manages Kafka users and ACLs
4. **CruiseControlTaskReconciler** - Handles Cruise Control tasks (scaling, rebalancing)
5. **CruiseControlOperationReconciler** - Manages Cruise Control operations
6. **AlertManagerForKafka** - Self-healing based on Prometheus alerts

### Multi-Module Structure
Uses Go workspaces with 5+ modules:
- Main: `/go.mod`
- API: `/api/go.mod`
- Properties parser: `/properties/go.mod`
- E2E tests: `/tests/e2e/go.mod`
- Third-party vendored: `/third_party/github.com/banzaicloud/*/go.mod`

## Directory Structure

```
/
├── api/                          # CRD definitions
│   ├── v1alpha1/                 # KafkaTopic, KafkaUser, CruiseControlOperation
│   ├── v1beta1/                  # KafkaCluster (main resource)
│   └── util/                     # API utilities
├── controllers/                  # Reconciliation logic
│   └── tests/                    # Controller tests (Ginkgo/Gomega)
├── pkg/                          # Core packages
│   ├── resources/                # Resource generators (pods, services, etc.)
│   │   ├── kafka/                # Broker resources
│   │   ├── cruisecontrol/        # Cruise Control resources
│   │   ├── envoy/                # Envoy proxy
│   │   ├── istioingress/         # Istio ingress
│   │   ├── contouringress/       # Contour ingress
│   │   └── templates/            # Common metadata templates
│   ├── kafkaclient/              # Kafka client (uses Sarama)
│   ├── scale/                    # Scaling logic
│   ├── webhooks/                 # Admission webhooks
│   ├── k8sutil/                  # Kubernetes utilities
│   ├── pki/                      # Certificate management
│   └── util/                     # General utilities
├── config/                       # Kubernetes manifests
│   ├── base/                     # Base manifests (CRDs, RBAC)
│   ├── overlays/                 # Kustomize overlays
│   └── samples/                  # Example KafkaCluster configs
├── charts/                       # Helm chart
├── tests/e2e/                    # End-to-end tests
├── docs/                         # Documentation
└── main.go                       # Operator entry point
```

## Build System

### Key Makefile Targets

```bash
make test                # Run unit tests with envtest
make test-e2e            # Run end-to-end tests
make lint                # Run golangci-lint across all modules
make check               # Run tests and linters
make generate            # Generate deepcopy, CRDs, and RBAC
make manifests           # Generate Kubernetes manifests
make tidy                # Run go mod tidy on all modules
make docker-build        # Build operator image
make install             # Install CRDs to cluster
make deploy              # Deploy operator to cluster
make run                 # Run operator locally (outside cluster)
```

### Important Make Variables
- `IMG` - Operator image name (default: `ghcr.io/adobe/koperator:latest`)
- `ENVTEST_K8S_VERSION` - Kubernetes version for tests (default: 1.31.x)

## Development Workflows

### Adding a New Feature

1. **Modify API types**
   
   ```bash
   # Edit api/v1beta1/kafkacluster_types.go or relevant file
   vi api/v1beta1/kafkacluster_types.go
   ```

2. **Generate code**
   
   ```bash
   make generate  # Generates deepcopy methods
   make manifests # Generates CRDs and RBAC
   ```

3. **Update controller logic**
   
   ```bash
   # Edit controllers/*.go
   vi controllers/kafkacluster_controller.go
   ```

4. **Add resource generators**
   
   ```bash
   # Add new resource reconciler in pkg/resources/<type>/
   mkdir pkg/resources/<type>
   vi pkg/resources/<type>/<type>.go
   ```

5. **Add tests**
   
   ```bash
   # Unit tests: *_test.go alongside source
   # Controller tests: controllers/tests/
   vi controllers/tests/kafkacluster_controller_test.go
   ```

6. **Validate**
   
   ```bash
   make check
   ```

### Modifying CRDs

CRDs are defined in Go structs with Kubebuilder markers:

```go
// +kubebuilder:validation:Minimum=1
// +kubebuilder:validation:Maximum=100
// +kubebuilder:default=3
Replicas int32 `json:"replicas,omitempty"`

// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.state"
```

After modifying:
```bash
make generate manifests
# CRDs generated to: config/base/crds/kafka.banzaicloud.io_kafkaclusters.yaml
```

### Adding a Resource Type

Resource reconcilers follow this pattern:

```go
package myresource

import (
    "github.com/banzaicloud/koperator/pkg/resources"
)

type Reconciler struct {
    resources.Reconciler
}

func New(client client.Client, cluster *v1beta1.KafkaCluster) *Reconciler {
    return &Reconciler{
        Reconciler: resources.Reconciler{
            Client:       client,
            KafkaCluster: cluster,
        },
    }
}

func (r *Reconciler) Reconcile(log logr.Logger) error {
    log = log.WithValues("component", "myresource")

    // Generate desired resource
    desired := r.myResource()

    // Reconcile using k8sutil
    if err := k8sutil.Reconcile(log, r.Client, desired, r.KafkaCluster); err != nil {
        return err
    }

    return nil
}
```

### Working with Owner References

All resources owned by KafkaCluster should use:

```go
import "github.com/banzaicloud/koperator/pkg/resources/templates"

// For resources with owner references (auto-deleted on cluster deletion)
metav1.ObjectMeta = templates.ObjectMeta(name, labels, cluster)

// For resources without owner references (manual cleanup required)
metav1.ObjectMeta = templates.ObjectMetaWithoutOwnerRef(name, labels, cluster)
```

Owner references set:
- `Controller: true` - Resource controlled by this owner
- `BlockOwnerDeletion: true` - Owner can't be deleted until resource is deleted

**Important**: In envtest (unit tests), garbage collection doesn't work. Manually delete resources in test cleanup.

## Testing

### Unit Tests

Framework: Standard Go testing + testify assertions

```bash
# Run all unit tests
make test

# Run specific package tests
go test ./pkg/resources/kafka/...

# Run with verbose output
go test -v ./controllers/...
```

**envtest**: Provides a fake Kubernetes API for controller testing without a real cluster.

### Controller Tests

Location: `controllers/tests/`
Framework: Ginkgo v2 + Gomega

```bash
# Run all controller tests
go test -v ./controllers/tests/...

# Run specific test suite
go test -v ./controllers/tests/ -ginkgo.focus="KafkaCluster"
```

**Important patterns**:
- Use `Eventually()` for async operations
- Use `Consistently()` to verify stable state
- Clean up resources in `JustAfterEach` blocks

### E2E Tests

Location: `tests/e2e/`

```bash
make test-e2e
```

Runs actual Kafka operations against test clusters using Kind.

## Code Patterns

### Resource Reconciliation

The `k8sutil.Reconcile()` function handles resource lifecycle:

```go
import "github.com/banzaicloud/koperator/pkg/k8sutil"

// Creates resource if not exists
// Updates resource if differs from desired state
// Uses k8s-objectmatcher to detect meaningful changes
err := k8sutil.Reconcile(log, r.Client, desired, r.KafkaCluster)
```

### Logging

Uses go-logr interface with structured logging:

```go
log.Info("resource created", "kind", "Service", "name", svcName)
log.Error(err, "failed to reconcile", "component", "kafka")
log.V(1).Info("debug message")  // V(1) = debug level
```

### Error Handling

Use the errorfactory package for consistent errors:

```go
import "github.com/banzaicloud/koperator/pkg/errorfactory"

return errorfactory.New(
    errorfactory.ResourceNotReady{},
    err,
    "broker not ready",
    "brokerId", brokerID,
)
```

### Kafka Client Usage

```go
import "github.com/banzaicloud/koperator/pkg/kafkaclient"

client, close, err := kafkaclient.NewFromCluster(r.Client, cluster)
if err != nil {
    return err
}
defer close()

// Use client methods
topics, err := client.ListTopics()
```

### Owner Reference Cleanup in Tests

When testing resources with `BlockOwnerDeletion: true`:

```go
// Remove owner references before deletion to avoid timing issues in envtest
service.SetOwnerReferences(nil)
err = k8sClient.Update(ctx, service)
Expect(err).NotTo(HaveOccurred())

err = k8sClient.Delete(ctx, service)
Expect(client.IgnoreNotFound(err)).NotTo(HaveOccurred())
```

## Important Files

### Entry Points
- `main.go` - Operator initialization, registers controllers and webhooks

### Controllers
- `controllers/kafkacluster_controller.go` - Main cluster reconciliation (17KB)
- `controllers/cruisecontroloperation_controller.go` - Cruise Control ops (30KB)

### API Definitions
- `api/v1beta1/kafkacluster_types.go` - KafkaCluster CRD (160KB)
- `api/v1alpha1/kafkatopic_types.go` - KafkaTopic CRD
- `api/v1alpha1/kafkauser_types.go` - KafkaUser CRD

### Resource Generators
- `pkg/resources/kafka/pod.go` - Broker pod generation
- `pkg/resources/kafka/configmap.go` - Kafka configuration
- `pkg/resources/kafka/service.go` - Service definitions
- `pkg/resources/nodeportexternalaccess/service.go` - NodePort services

### Tests
- `controllers/tests/kafkacluster_controller_test.go` - Main controller tests
- `controllers/tests/kafkacluster_controller_externalnodeport_test.go` - NodePort tests
- `tests/e2e/koperator_suite_test.go` - E2E test suite

### Configuration
- `config/base/crds/` - Generated CRDs
- `config/base/rbac/` - RBAC definitions
- `config/samples/` - Example KafkaCluster manifests

## Common Tasks

### Running Operator Locally

```bash
# Install CRDs
make install

# Run operator outside cluster (connects to kubeconfig context)
make run
```

### Debugging

```bash
# Enable verbose logging
go run ./main.go --verbose

# Development mode (more logging)
go run ./main.go --development

# Watch operator logs in cluster
kubectl logs -n kafka -l app.kubernetes.io/name=kafka-operator -f
```

### Updating Dependencies

```bash
# Update all Go dependencies across all modules
make update-go-deps

# Tidy all modules
make tidy

# Verify everything still works
make check
```

### Fixing Test Failures

Common issues:

1. **envtest timeout issues**: Increase timeouts in `Eventually()` blocks
2. **Resource cleanup**: Ensure resources are deleted in `JustAfterEach`
3. **Owner reference issues**: Remove owner refs before deletion in tests
4. **Port conflicts**: Ensure NodePort services are fully deleted between tests

## CI/CD

GitHub Actions workflows (`.github/workflows/`):
- `ci.yml` - PR checks (tests, linting)
- `e2e-test.yaml` - End-to-end tests
- `operator-release.yml` - Release builds
- `codeql-analysis.yml` - Security scanning

## Key Dependencies

- **controller-runtime** v0.22.4 - Operator framework
- **k8s.io/*** v0.34.3 - Kubernetes client libraries
- **IBM/sarama** v1.46.3 - Kafka client
- **Ginkgo** v2 - BDD testing framework
- **cert-manager** v1.19.2 - Certificate management integration

## Troubleshooting

### Build Issues

```bash
# Clean and regenerate
make clean
make generate manifests

# Update dependencies
make tidy
```

### Test Issues

```bash
# Run specific test with verbose output
go test -v -run TestName ./path/to/package

# Run Ginkgo tests with focus
go test -v ./controllers/tests -ginkgo.focus="TestPattern"
```

### CRD Issues

```bash
# Reinstall CRDs
make uninstall install

# Check CRD is registered
kubectl get crd kafkaclusters.kafka.banzaicloud.io
```

## Best Practices

1. **Always run `make generate manifests` after modifying API types**
2. **Use `Eventually()` for async Kubernetes operations in tests**
3. **Clean up test resources in `JustAfterEach` blocks**
4. **Use structured logging with key-value pairs**
5. **Handle NotFound errors with `client.IgnoreNotFound()`**
6. **Set owner references for auto-cleanup (except in specific cases)**
7. **Run `make check` before committing**
8. **Keep commit messages descriptive and reference issues**

## Contributing

1. Fork and create a branch
2. Make changes
3. Run `make check` to validate
4. Write/update tests
5. Commit with descriptive message
6. Push and create PR

## Resources

- **Main Docs**: `/docs/` directory
- **API Reference**: `/api/v1beta1/` Go struct definitions
- **Examples**: `/config/samples/` for KafkaCluster manifests
- **Helm Chart**: `/charts/kafka-operator/`
- **GitHub Issues**: https://github.com/adobe/koperator/issues

---

Generated for AI agents working with the Koperator codebase.
Last updated: 2026-02-03
