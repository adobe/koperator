# Spec: Per-Listener Ingress Controller

## ADDED Requirements

### Requirement: External listener may specify ingress controller type

The system SHALL allow an external listener to optionally specify which ingress controller type (`envoy`, `contour`, or `istioingress`) to use. When not specified, the listener SHALL use the cluster-level `KafkaCluster.Spec.IngressController` value.

#### Scenario: Listener with explicit ingress controller

- **WHEN** an external listener has `ingressController` set to `contour`
- **THEN** only the Contour reconciler SHALL create or update ingress resources for that listener, and Envoy and Istio reconcilers SHALL not create resources for it

#### Scenario: Listener without explicit ingress controller

- **WHEN** an external listener does not set `ingressController` (empty or omitted)
- **THEN** the effective controller for that listener SHALL be `KafkaCluster.Spec.IngressController`, and reconcilers SHALL behave as before (all listeners use cluster default)

#### Scenario: Mixed controllers on same cluster

- **WHEN** one external listener has `ingressController: envoy` and another has `ingressController: contour`
- **THEN** the Envoy reconciler SHALL create resources only for the first listener and the Contour reconciler SHALL create resources only for the second listener

### Requirement: Effective controller drives config and status

The system SHALL use the effective ingress controller for a listener (per-listener value if set, else cluster default) when resolving ingress config, service names, and listener status for that listener.

#### Scenario: GetIngressConfigs uses effective controller

- **WHEN** `GetIngressConfigs(spec, eListener)` is called and the listener has `ingressController: contour`
- **THEN** the returned config SHALL be the Contour branch (ContourIngressConfig merge) regardless of `spec.IngressController`

#### Scenario: Service lookup uses effective controller

- **WHEN** status or service lookup resolves the ingress service for a listener that uses Contour
- **THEN** the system SHALL use the Contour service name template for that listener, not the Envoy or Istio template

### Requirement: Cleanup when listener no longer uses a controller

When `RemoveUnusedIngressResources` is true and an external listener’s effective controller is not a given type, the system SHALL remove any existing ingress resources (services, HTTPProxies, etc.) that were created for that (listener, controller) pair.

#### Scenario: Listener switches from Envoy to Contour

- **WHEN** a listener is updated from `ingressController: envoy` (or cluster default envoy) to `ingressController: contour` and `RemoveUnusedIngressResources` is true
- **THEN** Envoy resources for that listener SHALL be deleted and Contour resources SHALL be created for that listener

### Requirement: Validation of per-listener ingress controller

The system SHALL validate that when `ExternalListenerConfig.IngressController` is non-empty, it SHALL be one of `envoy`, `contour`, or `istioingress`. When any listener’s effective controller is `istioingress`, the system SHALL require `KafkaCluster.Spec.IstioControlPlane` to be set.

#### Scenario: Invalid per-listener value rejected

- **WHEN** an external listener has `ingressController: nginx`
- **THEN** the admission webhook SHALL reject the KafkaCluster with a validation error

#### Scenario: Istio without control plane rejected

- **WHEN** any external listener has effective controller `istioingress` and `Spec.IstioControlPlane` is nil
- **THEN** the admission webhook SHALL reject the KafkaCluster (same rule as today, applied to effective controllers in use)

### Requirement: Backward compatibility

Existing KafkaCluster resources that do not set `ingressController` on any external listener SHALL behave identically to before: all listeners SHALL use `Spec.IngressController` and all reconcilers SHALL behave as they do today.

#### Scenario: Existing CR unchanged

- **WHEN** a KafkaCluster has no `ingressController` field on any external listener
- **THEN** every listener’s effective controller SHALL be `Spec.IngressController` and resource creation/cleanup SHALL match current (pre-feature) behavior
