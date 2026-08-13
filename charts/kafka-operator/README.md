# Koperator chart

The [Koperator](https://github.com/adobe/koperator) is a Kubernetes operator to deploy and manage [Apache Kafka](https://kafka.apache.org) resources for a Kubernetes cluster.

## Prerequisites

- Kubernetes 1.15.0+
- Helm 3.8+ (for OCI registry support)

## Installing the chart

Before installing the chart, you must first install the Koperator CustomResourceDefinition resources.
This is performed in a separate step to allow you to easily uninstall and reinstall Koperator without deleting your installed custom resources.

```bash
kubectl apply -f https://raw.githubusercontent.com/adobe/koperator/refs/heads/master/config/base/crds/kafka.banzaicloud.io_cruisecontroloperations.yaml
kubectl apply -f https://raw.githubusercontent.com/adobe/koperator/refs/heads/master/config/base/crds/kafka.banzaicloud.io_kafkaclusters.yaml
kubectl apply -f https://raw.githubusercontent.com/adobe/koperator/refs/heads/master/config/base/crds/kafka.banzaicloud.io_kafkatopics.yaml
kubectl apply -f https://raw.githubusercontent.com/adobe/koperator/refs/heads/master/config/base/crds/kafka.banzaicloud.io_kafkausers.yaml
```

To install the chart from the OCI registry. Use `--skip-crds` since the CRDs were already installed in the
previous step - without it, Helm's own CRD install can conflict with the `kubectl apply` above
([#265](https://github.com/adobe/koperator/issues/265)):

> 📦 **View available versions**: [ghcr.io/adobe/helm-charts/kafka-operator](https://github.com/adobe/koperator/pkgs/container/helm-charts%2Fkafka-operator/versions)

OCI registries have no floating "latest" tag, so `--version` is required (see available versions above):

```bash
helm install kafka-operator oci://ghcr.io/adobe/helm-charts/kafka-operator \
  --version 0.30.0 --namespace=kafka --create-namespace --skip-crds
```

To install the operator using an already installed cert-manager:
```bash
helm install kafka-operator oci://ghcr.io/adobe/helm-charts/kafka-operator \
  --version 0.30.0 \
  --set certManager.namespace=<your cert manager namespace> --namespace=kafka --create-namespace --skip-crds
```

## Upgrading the chart

To upgrade the chart since the helm 3 limitation you have to set a value as well to keep your CRDs.
If this value is not set your CRDs might be deleted. `--version` is required, same as for install.

```bash
helm upgrade kafka-operator oci://ghcr.io/adobe/helm-charts/kafka-operator \
  --version 0.30.0 --namespace=kafka
```

## Uninstalling the Chart

To uninstall/delete the `kafka-operator` release:

```
$ helm delete --purge kafka-operator
```

The command removes all the Kubernetes components associated with the chart and deletes the release.

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| additionalEnv | list | `[]` | Additional Environment Variables |
| additionalSidecars | list | `[]` | Additional Sidecars Configuration |
| additionalVolumes | list | `[]` | Additional volumes required for sidecars |
| affinity | object | `{}` | Operator pod affinity can be set |
| alertManager.enable | bool | `true` | AlertManager can be enabled |
| alertManager.permissivePeerAuthentication.create | bool | `false` | Permissive PeerAuthentication (Istio resource) for AlertManager can be created |
| alertManager.port | int | `9001` | AlertManager port |
| certManager.enabled | bool | `false` | Operator will integrate with the cert manager |
| certManager.namespace | string | `"cert-manager"` | Operator will look for the cert manager in this namespace namespace field specifies the Cert-manager's Cluster Resource Namespace. https://cert-manager.io/docs/configuration/ |
| certSigning.enabled | bool | `true` | Enable native certificate signing integration |
| containerSecurityContext | object | `{}` | Container Security Context |
| contour.enabled | bool | `false` | Enable Project Contour ingress integration. Only enable this when the Kafka cluster uses `ingressController: contour`. When enabled, Project Contour's HTTPProxy CRD (projectcontour.io/v1) must be installed in the cluster, otherwise the operator fails to start. https://projectcontour.io |
| fullnameOverride | string | `""` | Release full name can be overwritten |
| healthProbes | object | `{}` | Health probes configuration |
| nameOverride | string | `""` | Release name can be overwritten |
| nodeSelector | object | `{}` | Operator pod node selector can be set |
| operator.annotations | object | `{}` | Operator pod annotations can be set |
| operator.developmentLogging | bool | `false` | Enable development logging |
| operator.image.pullPolicy | string | `"IfNotPresent"` | Operator container image pull policy |
| operator.image.repository | string | `"ghcr.io/adobe/koperator"` | Operator container image repository |
| operator.image.tag | string | `"0.30.0"` | Operator container image tag |
| operator.namespaces | string | `"kafka, cert-manager"` | List of namespaces where Operator watches for custom resources.<br><br>**Note** that the operator still requires to read the cluster-scoped `Node` labels to configure `rack awareness`. Make sure the operator ServiceAccount is granted `get` permissions on this `Node` resource when using limited RBACs. |
| operator.resources.limits | object | `{"cpu":"200m","memory":"256Mi"}` | CPU/Memory limits |
| operator.resources.requests | object | `{"cpu":"200m","memory":"256Mi"}` | CPU/Memory requests |
| operator.serviceAccount.create | bool | `true` | If true, create the `operator.serviceAccount.name` service account |
| operator.serviceAccount.name | string | `"kafka-operator"` | ServiceAccount used by the operator pod |
| operator.verboseLogging | bool | `false` | Enable verbose logging |
| podSecurityContext | object | `{}` | Pod Security Context See https://kubernetes.io/docs/tasks/configure-pod-container/security-context/ |
| prometheusMetrics.authProxy.enabled | bool | `true` | If true, use auth proxy for Prometheus metrics |
| prometheusMetrics.authProxy.image.pullPolicy | string | `"IfNotPresent"` | Auth proxy container image pull policy |
| prometheusMetrics.authProxy.image.repository | string | `"quay.io/brancz/kube-rbac-proxy"` | Auth proxy container image repository |
| prometheusMetrics.authProxy.image.tag | string | `"v0.22.1"` | Auth proxy container image tag |
| prometheusMetrics.authProxy.serviceAccount.create | bool | `true` | If true, create the service account (see `prometheusMetrics.authProxy.serviceAccount.name`) used by prometheus auth proxy |
| prometheusMetrics.authProxy.serviceAccount.name | string | `"kafka-operator-authproxy"` | ServiceAccount used by prometheus auth proxy |
| prometheusMetrics.enabled | bool | `true` | If true, use direct access for Prometheus metrics |
| prometheusMetrics.podMonitor.enabled | bool | `false` | If true, create a PodMonitor for Prometheus metrics |
| prometheusMetrics.podMonitor.interval | string | `"30s"` |  |
| rbac.enabled | bool | `true` | Create rbac service account and roles |
| replicaCount | int | `1` | Operator replica count can be set |
| tolerations | list | `[]` | Operator pod tolerations can be set |
| webhook.certs.generate | bool | `true` | Helm chart will generate cert for the webhook |
| webhook.certs.secret | string | `"kafka-operator-serving-cert"` | Helm chart will use the secret name applied here for the cert |
| webhook.enabled | bool | `true` | Operator will activate the admission webhooks for custom resources |
