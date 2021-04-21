# Kafka-operator chart

[Kafka-operator](https://github.com/banzaicloud/kafka-operator) is a Kubernetes operator to deploy and manage [Kafka](https://kafka.apache.org) resources for a Kubernetes cluster.

## Prerequisites

- Kubernetes 1.15.0+

## Installing the chart

Before installing the chart, you must first install the kafka-operator CustomResourceDefinition resources.
This is performed in a separate step to allow you to easily uninstall and reinstall kafka-operator without deleting your installed custom resources.

```
kubectl create --validate=false -f https://github.com/banzaicloud/kafka-operator/releases/download/v0.14.0/kafka-operator.crds.yaml
```

To install the chart:

```
$ helm repo add banzaicloud-stable https://kubernetes-charts.banzaicloud.com
$ helm install kafka-operator --create-namespace --namespace=kafka banzaicloud-stable/kafka-operator
```

To install the operator using an already installed cert-manager
```bash
$ helm install kafka-operator --set certManager.namespace=<your cert manager namespace> --namespace=kafka  --create-namespace banzaicloud-stable/kafka-operator
```

## Upgrading the chart

To upgrade the chart since the helm 3 limitation you have to set a value as well to keep your CRDs.
If this value is not set your CRDs might be deleted.

```bash
helm upgrade kafka-operator --set crd.enabled=true --namespace=kafka banzaicloud-stable/kafka-operator
```

## Uninstalling the Chart

To uninstall/delete the `kafka-operator` release:

```
$ helm delete --purge kafka-operator
```

The command removes all the Kubernetes components associated with the chart and deletes the release.

## Configuration

The following table lists the configurable parameters of the Banzaicloud Kafka Operator chart and their default values.

Parameter | Description | Default
--------- | ----------- | -------
`operator.image.repository` | Operator container image repository | `ghcr.io/banzaicloud/kafka-operator`
`operator.image.tag` | Operator container image tag | `v0.12.4`
`operator.image.pullPolicy` | Operator container image pull policy | `IfNotPresent`
`operator.serviceAccount.name` | ServiceAccount used by the operator pod | `kafka-operator`
`operator.serviceAccount.create` | If true, create the `operator.serviceAccount.name` service account | `true`
`operator.resources` | CPU/Memory resource requests/limits (YAML) | Memory: `128Mi/256Mi`, CPU: `100m/200m`
`operator.namespaces` | List of namespaces where Operator watches for custom resources.<br><br>**Note** that the operator still requires to read the cluster-scoped `Node` labels to configure `rack awareness`. Make sure the operator ServiceAccount is granted `get` permissions on this `Node` resource when using limited RBACs.| `""` i.e. all namespaces
`operator.annotations` | Operator pod annotations can be set | `{}`
`prometheusMetrics.enabled` | If true, use direct access for Prometheus metrics | `false`
`prometheusMetrics.authProxy.enabled` | If true, use auth proxy for Prometheus metrics | `true`
`prometheusMetrics.authProxy.serviceAccount.create` | If true, create the service account (see `prometheusMetrics.authProxy.serviceAccount.name`) used by prometheus auth proxy | `true`
`prometheusMetrics.authProxy.serviceAccount.name` | ServiceAccount used by prometheus auth proxy | `kafka-operator-authproxy`
`prometheusMetrics.authProxy.image.repository` | Auth proxy container image repository | `gcr.io/kubebuilder/kube-rbac-proxy`
`prometheusMetrics.authProxy.image.tag` | Auth proxy container image tag | `v0.8.0`
`prometheusMetrics.authProxy.image.pullPolicy` | Auth proxy container image pull policy | `IfNotPresent`
`rbac.enabled` | Create rbac service account and roles | `true`
`imagePullSecrets` | Image pull secrets can be set | `[]`
`replicaCount` | Operator replica count can be set | `1`
`alertManager.enable` | AlertManager can be enabled | `true`
`nodeSelector` | Operator pod node selector can be set | `{}`
`tolerations` | Operator pod tolerations can be set | `[]`
`affinity` | Operator pod affinity can be set | `{}`
`nameOverride` | Release name can be overwritten | `""`
`crd.enabled` | Whether to enable CRD installation(used for upgrade only) | `false`
`fullnameOverride` | Release full name can be overwritten | `""`
`certManager.namespace` | Operator will look for the cert manager in this namespace | `cert-manager`
`certManager.enabled` | Operator will integrate with the cert manager | `false`
`webhook.enabled` | Operator will activate the admission webhooks for custom resources | `true`
`webhook.certs.generate` | Helm chart will generate cert for the webhook | `true`
`webhook.certs.secret` | Helm chart will use the secret name applied here for the cert | `kafka-operator-serving-cert`
`logForward:create` | If true, operator logs forward to Splunk | `false`
`splunk.host` | Splunk server can be set | `""`
`splunk.source` | Source of logs can be set | `kafka-operator`
`splunk.index` | Splunk index can be set | `""`
`splunk.logParser` | Splunk logParser can be set | `docker`
`splunk.sourceType` | Log SourceType can be set | `pod`
`splunk.port` | Splunk port can be set | `443`
`splunk.token` | Splunk secret token can be set | `""`
`splunk.fluentBit.name` | fluent-bit container name can be set | `fluent-bit`
`splunk.fluentBit.resources`| CPU/Memory resource requests/limits (YAML) | Memory: `128Mi/512Mi`, CPU: `50m/250m`
`splunk.fluentBit.port` | fluent-bit container port can be set | `2020`
`splunk.fluentBit.image.repository` | fluent-bit container image repository | `""`
`splunk.fluentBit.image.tag` | fluent-bit container image tag | `""`
`splunk.fluentBit.image.pullPolicy` | fluent-bit container image pull policy | `IfNotPresent`
`splunk.fluentBit.image.imagePullSecrets` | Image pull secrets can be set | `[]`
