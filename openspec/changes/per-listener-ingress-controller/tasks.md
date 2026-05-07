# Tasks: Per-Listener Ingress Controller

## 1. API and effective controller helper

- [x] 1.1 Add optional `IngressController string` to `ExternalListenerConfig` in `api/v1beta1/kafkacluster_types.go` with json tag and kubebuilder validation enum `envoy;contour;istioingress`
- [x] 1.2 Add `GetIngressController(spec *KafkaClusterSpec) string` on `ExternalListenerConfig` (or equivalent helper in util) that returns `eListener.IngressController` if non-empty, else `spec.GetIngressController()`
- [x] 1.3 Run code generation (`make generate`) and ensure deepcopy includes the new field

## 2. GetIngressConfigs and util

- [x] 2.1 In `pkg/util/util.go`, update `GetIngressConfigs` to derive effective controller per listener via the new helper and use it to select the envoy/contour/istio branch and merge the correct cluster-level config
- [x] 2.2 Add unit tests for `GetIngressConfigs` with per-listener override (e.g. listener contour + spec envoy returns contour config)

## 3. Ingress reconcilers (Contour, Envoy, Istio)

- [x] 3.1 In `pkg/resources/contouringress/contour.go`, replace cluster-level `GetIngressController() == contour` check with per-listener effective controller; only reconcile listeners whose effective controller is contour; cleanup branch uses same effective check
- [x] 3.2 In `pkg/resources/envoy/envoy.go`, replace cluster-level check with per-listener effective controller; only reconcile listeners whose effective controller is envoy; cleanup branch uses same effective check
- [x] 3.3 In `pkg/resources/istioingress/istioingress.go`, replace cluster-level check with per-listener effective controller; only reconcile listeners whose effective controller is istioingress; cleanup branch uses same effective check

## 4. Kafka status and service lookup

- [x] 4.1 Update `getServiceFromExternalListener` in `pkg/resources/kafka/kafka.go` to accept effective controller for the listener (or listener + spec) and use it in the switch for service name template
- [x] 4.2 Update `createExternalListenerStatuses` and any callers that resolve ingress services to pass effective controller per listener when calling `getServiceFromExternalListener`
- [x] 4.3 Find and update any other listener-scoped use of `GetIngressController()` in `pkg/resources/kafka/kafka.go` (e.g. broker config) to use effective controller for that listener

## 5. Webhook validation

- [x] 5.1 In `pkg/webhooks/kafkacluster_validator.go`, add validation that each external listener’s `IngressController` (when non-empty) is one of envoy, contour, istioingress
- [x] 5.2 Ensure validation that when any listener’s effective controller is istioingress, `Spec.IstioControlPlane` is required (extend or reuse existing rule)
- [x] 5.3 Add or extend webhook tests for per-listener enum and Istio control plane

## 6. Tests

- [x] 6.1 Add controller test(s) for mixed listener controller types (e.g. one listener envoy, one contour) and verify only expected resources created per reconciler
- [x] 6.2 Add test for defaulting: no per-listener field, all listeners use cluster default (backward compatibility)
- [x] 6.3 Add test for cleanup when listener switches controller type with RemoveUnusedIngressResources true
