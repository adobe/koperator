# Design: Per-Listener Ingress Controller

## Context

KafkaCluster currently has a single cluster-wide `spec.ingressController` (`envoy` | `contour` | `istioingress`). All external listeners use that type. Each ingress reconciler (envoy, contour, istioingress) checks `Spec.GetIngressController() == <this controller>` and, if true, reconciles all external listeners that match the required access method (LoadBalancer for envoy/istio, ClusterIP for contour). Config selection (`util.GetIngressConfigs`) and service lookup (`getServiceFromExternalListener`) also use the cluster-level value. The goal is to allow each external listener to optionally specify its own ingress controller type, with the cluster default as fallback, so one cluster can mix controller types (e.g. one listener via Envoy, another via Contour).

## Goals / Non-Goals

**Goals:**

- Each external listener MAY specify an optional `ingressController`; when unset, the listener uses `KafkaCluster.Spec.IngressController`.
- Reconcilers create/update/delete ingress resources only for listeners whose effective controller matches that reconciler.
- Status, config, and service lookup use the effective controller per listener. Cleanup (RemoveUnusedIngressResources) removes resources when a listener no longer uses that controller type.
- Backward compatible: existing CRs without the new field behave exactly as today.

**Non-Goals:**

- Changing the coupling between controller type and access method (Envoy/Istio → LoadBalancer, Contour → ClusterIP). Per-listener controller does not introduce new access-method combinations.
- Supporting multiple controller types for a single listener (e.g. same listener on both Envoy and Contour). Each listener has exactly one effective controller.

## Decisions

### 1. API: optional field on ExternalListenerConfig

- **Choice:** Add `IngressController string` (optional, empty means use cluster default) on `ExternalListenerConfig`.
- **Rationale:** Keeps API simple and backward compatible. No pointer needed if empty string is treated as "use default"; existing CRs have no field (zero value) and continue to use cluster default. Alternatively a `*string` could make "unset" explicit; we prefer `string` with empty = default to avoid pointer proliferation and match existing style (e.g. `AccessMethod`).
- **Alternative:** Required per-listener field. Rejected because it would force every existing listener to be updated.

### 2. Effective controller helper

- **Choice:** Introduce a helper that returns the effective ingress controller for a listener, e.g. `GetIngressController(spec *KafkaClusterSpec, eListener ExternalListenerConfig) string`. If `eListener.IngressController != ""`, return it; else return `spec.GetIngressController()`.
- **Rationale:** Single place for defaulting; all call sites (reconcilers, GetIngressConfigs, status, getServiceFromExternalListener) use this instead of reading cluster spec only.
- **Alternative:** Inline logic at each call site. Rejected to avoid drift and bugs.

### 3. GetIngressConfigs signature

- **Choice:** Keep signature `GetIngressConfigs(spec, eListener)` and derive effective controller inside the function via the new helper. Use that effective value to choose the envoy/contour/istio branch and to merge the correct cluster-level config (EnvoyConfig, ContourIngressConfig, or IstioIngressConfig).
- **Rationale:** No API change for callers; behavior change is internal. Callers already pass both spec and listener.
- **Alternative:** Add an explicit `effectiveController string` parameter. Rejected as redundant when it can be derived from spec + listener.

### 4. Reconciler logic

- **Choice:** For each external listener, compute effective controller. If it matches this reconciler’s controller and access method, reconcile that listener (create/update resources). Else if RemoveUnusedIngressResources is set, treat as "this listener does not use this controller" and delete any existing resources for this (listener, controller) pair.
- **Rationale:** Aligns with current per-listener loop; only the condition changes from cluster-level to per-listener effective controller.
- **Alternative:** Separate "reconcile" and "cleanup" passes. Current code already does both in one loop; no need to split.

### 5. getServiceFromExternalListener and status

- **Choice:** These functions currently take cluster + listener name + ingress config name and use `cluster.Spec.GetIngressController()` to pick the service name template. Change them to accept the effective controller for that listener (or the listener config itself) and use it for the switch. Callers (e.g. status creation) already iterate listeners and call GetIngressConfigs; they can compute effective controller once per listener and pass it (or pass listener so the helper can be used).
- **Rationale:** Status and service lookup must use the same controller type that was used to create the service, i.e. the effective controller for that listener.

### 6. Validation

- **Choice:** In the KafkaCluster webhook, validate that `ExternalListenerConfig.IngressController` (when non-empty) is one of `envoy`, `contour`, `istioingress`. If any listener’s effective controller is `istioingress`, require `Spec.IstioControlPlane != nil` (same rule as today, evaluated for the set of effective controllers in use).
- **Rationale:** Prevents invalid enum values and ensures Istio config is present when any listener uses Istio.

## Risks / Trade-offs

- **[Risk]** Call sites that use `GetIngressController()` in a listener context might be missed, leading to wrong controller type (e.g. status pointing at wrong service). **Mitigation:** Grep for all uses of `GetIngressController()` and ensure listener-scoped paths use the effective controller helper; add tests for mixed listener types and status/service lookup.
- **[Risk]** Defaulting empty string to cluster default could be confused with "no ingress" if we ever add that. **Mitigation:** Document clearly; today there is no "no ingress" option, so empty = default is unambiguous.
- **[Trade-off]** Keeping controller–access-method coupling (Contour = ClusterIP only, Envoy/Istio = LoadBalancer only) means we do not support e.g. Contour with LoadBalancer. **Mitigation:** Out of scope for this change; document in design and proposal.

## Migration Plan

- **Deploy:** No migration required. New field is optional; existing CRs unchanged.
- **Rollback:** Revert the operator; CRs with per-listener `ingressController` set will be ignored by older operator (field unknown), and cluster-level default will be used for all listeners. If users had mixed controllers, reverting would effectively force all listeners back to the cluster default until CRs are edited. Document this in release notes.

## Open Questions

- None at design time. Validation and helper placement (API type vs util package) can be decided during implementation; recommendation is helper in `api/v1beta1` or `pkg/util` and used from both API and reconcilers.
