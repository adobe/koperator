# Per-Listener Ingress Controller

## Why

Today, a KafkaCluster has a single cluster-wide ingress controller type (`envoy`, `contour`, or `istioingress`). All external listeners use that same type. Operators cannot expose one listener via Envoy (e.g. public LoadBalancer) and another via Contour (e.g. internal TLS) on the same cluster without running multiple clusters or custom tooling. Allowing each external listener to specify its own ingress controller (with a cluster default) removes this limitation and supports mixed ingress environments.

## What Changes

- Add optional `ingressController` field on `ExternalListenerConfig`. When unset, the listener uses `KafkaCluster.Spec.IngressController` (current behavior).
- Reconcilers (envoy, contour, istioingress) will create/update/delete resources only for external listeners whose effective ingress controller matches that reconciler.
- `util.GetIngressConfigs` and all call sites that today use cluster-level `GetIngressController()` in a listener context will use the effective controller for that listener (helper: e.g. effective controller from listener + spec).
- Status and service lookup (e.g. `getServiceFromExternalListener`) will use the effective ingress controller per listener when resolving service names.
- `RemoveUnusedIngressResources` cleanup will remain per-listener and will remove resources when a listener no longer uses that controller type.
- Validation: per-listener value restricted to same enum as cluster (`envoy` | `contour` | `istioingress`). If any listener uses `istioingress`, `Spec.IstioControlPlane` must be set (unchanged rule).

No breaking changes: existing CRs omit the new field and keep current behavior (all listeners use cluster default).

## Capabilities

### New Capabilities

- `per-listener-ingress-controller`: External listeners can optionally specify which ingress controller type to use (envoy, contour, istioingress). When unspecified, the cluster-level `spec.ingressController` is used. The operator reconciles ingress resources per listener based on this effective controller and cleans up when a listener switches or is removed.

### Modified Capabilities

- (None — no existing specs in openspec/specs/; this is a new API/behavior capability.)

## Impact

- **API**: `api/v1beta1/kafkacluster_types.go` — `ExternalListenerConfig` gains optional `IngressController string`.
- **Util**: `pkg/util/util.go` — `GetIngressConfigs` and new helper for effective ingress controller per listener.
- **Reconcilers**: `pkg/resources/contouringress/contour.go`, `pkg/resources/envoy/envoy.go`, `pkg/resources/istioingress/istioingress.go` — gate on effective controller per listener.
- **Kafka/config/status**: `pkg/resources/kafka/kafka.go` — status creation and `getServiceFromExternalListener` use effective controller per listener.
- **Webhooks**: `pkg/webhooks/kafkacluster_validator.go` — validate per-listener enum and istio control plane when any listener uses istioingress.
- **Tests**: Controller and util tests for mixed listener controller types and defaulting.
