---
title: Grafana Dashboards
shorttitle: Grafana Dashboards
weight: 10
---

These ready-to-import [Grafana](https://grafana.com/) dashboards visualize a Koperator-managed Kafka cluster. They read the metrics produced by the [JMX exporter and ServiceMonitors]({{< relref "/docs/monitoring" >}}) described above, plus a couple of optional, commonly-paired components noted below.

| Dashboard | File | Requires |
|---|---|---|
| [Kafka Looking Glass](#kafka-looking-glass) | [`kafka-looking-glass.json`](kafka-looking-glass.json) | JMX exporter metrics, [Cruise Control]({{< relref "/docs/cruisecontroloperation.md" >}}) |
| [Kafka Topic](#kafka-topic) | [`kafka-topic.json`](kafka-topic.json) | JMX exporter metrics |
| [Kafka Consumers](#kafka-consumers) | [`kafka-consumers.json`](kafka-consumers.json) | [KMinion](https://github.com/redpanda-data/kminion) |

All three dashboards share the same Prometheus-datasource variable (`data_source`) and, where applicable, a `kubernetes_namespace` / `cluster_name` variable pair — `cluster_name` maps to the name of your `KafkaCluster` custom resource.

## Kafka Looking Glass

A cluster-wide health and performance overview: broker/ZooKeeper quorum status, under-replicated and offline partitions, throughput, replication lag, broker CPU/memory/disk/JVM/GC, and Cruise Control rebalance status.

A few rows depend on components that aren't part of a default Koperator install and can be safely removed if unused:

- **Authorization** — expects a custom Kafka authorizer plugin exporting `kafka_authorizer_*` JMX metrics (Kafka's built-in ACL authorizer does not emit these). Adjust the metric prefix/labels to match your own authorizer, or delete the row.
- **Envoy panels** (`Envoy Unhealthy Kafka`, `Envoy Instances`, `Envoy connections`) — only apply if brokers are fronted by an [Envoy](https://www.envoyproxy.io/) proxy for [external access]({{< relref "/docs/external-listener" >}}).
- **Canary Producer panels** (`Canary Producer Throughput`, `Canary Producer Errors`, in the Health row) — expect a synthetic canary producer exporting `kafka_canary_producer_*` metrics to continuously verify the external listener is reachable. Point these at your own canary tooling, or remove them.

## Kafka Topic

Per-topic drill-down: partition count, replication factor, ISR status, retention/cleanup-policy configuration, and throughput for a single topic selected via the `kafka_topic` variable. Includes an optional `Operations By User` panel with the same custom-authorizer dependency as the Looking Glass Authorization row.

## Kafka Consumers

Consumer-group lag, membership, and consume-vs-produce rate, selected via the `kafka_topic` and `consumer_group` variables. Powered entirely by [KMinion](https://github.com/redpanda-data/kminion), an open-source Kafka lag exporter — deploy it alongside Prometheus and point it at your cluster to populate this dashboard.

## Importing a dashboard

**Grafana UI**: *Dashboards → New → Import*, upload the JSON file, and map the `data_source` prompt to your Prometheus datasource.

**GitOps / sidecar**: ship the JSON as a `ConfigMap` labeled for your Grafana dashboard sidecar (for example, the `grafana` or `kube-prometheus-stack` Helm charts' `sidecar.dashboards` feature, which watches for the `grafana_dashboard` label):

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: kafka-looking-glass-dashboard
  labels:
    grafana_dashboard: "1"
binaryData:
  kafka-looking-glass.json: <base64-encoded contents of kafka-looking-glass.json>
```
