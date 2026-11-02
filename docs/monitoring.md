# Monitoring Kafka clusters

Use Prometheus to collect broker and Cruise Control metrics, and Alertmanager
to notify operators or deliver approved actions to Koperator.

The [monitoring sample](../config/samples/kafkacluster-prometheus.yaml) provides
ServiceMonitors, alert rules, a Prometheus instance, and an Alertmanager instance.
Broker alerts are monitoring-only. The sample also enables automatic PVC
expansion for its disk-space alert; review that action before applying it.

For broker scaling configuration and operation, see the
[autoscaling user guide](autoscale.md).

## Prerequisites

- A running KafkaCluster with broker and Cruise Control JMX metrics available.
- Prometheus Operator with the `ServiceMonitor`, `PrometheusRule`, `Prometheus`,
  and `Alertmanager` CRDs installed.
- Network access from Prometheus to metrics endpoints, and from Alertmanager to
  notification integrations and Koperator's webhook.
- Kubelet volume metrics if you intend to use the disk-space rule. The sample's
  ServiceMonitors collect Kafka and Cruise Control metrics, not kubelet metrics.

The sample assumes a KafkaCluster named `kafka` in namespace `kafka`, and a
Koperator Helm release named `kafka-operator`. Adjust resource selectors,
namespace selectors, RBAC subjects, alert labels, and webhook addresses for
your installation.

## Configure notifications and actions

Edit `config/samples/kafkacluster-prometheus.yaml` before deployment.

The `alertmanager-kafka-alertmanager` Secret contains Alertmanager's
configuration in `stringData.alertmanager.yaml`. Its receivers are:

| Receiver | Alerts | Default behavior |
|---|---|---|
| `monitoring` | All alerts not matched by a child route | No notification integration configured |
| `koperator-storage` | `RemainingDiskSpaceLow` with `alertGroup="kafka"` | Sends the alert to Koperator to request PVC expansion |

Configure your human notification integration on `monitoring`. Without it,
alerts are visible in Prometheus and Alertmanager but do not notify a person.
Keep availability notifications separate from action routes.

The storage webhook URL is:

```text
http://kafka-operator-alertmanager.kafka.svc:9001/
```

This is Koperator's action receiver, not an Alertmanager server. Prometheus sends
alerts to the sample's actual Alertmanager Service, `kafka-alertmanager:9093`;
Alertmanager then applies its routes. Keep `send_resolved: true` for Koperator
receivers so processed alerts can be cleared when they resolve.

### Review automatic storage expansion

`RemainingDiskSpaceLow` fires when available space is below 15% and a linear
projection from the last six hours predicts exhaustion within four days. The
condition must remain true for two minutes. Its annotations request
`command: resizePvc` with an increment of `10G`.

Confirm that the relevant StorageClass supports expansion and that the added
capacity and cost are acceptable. Review the PVC selector, kubelet metric
labels, and forecast window against your storage setup.

If expansion requires manual approval, remove the storage action route so the
alert reaches `monitoring`, and remove its `command` and `incrementBy`
annotations. Preserve human-facing disk-space notifications.

## Deploy the monitoring sample

Select the intended Kubernetes context explicitly. From the repository root,
after reviewing the configuration:

```sh
CONTEXT=your-kubernetes-context
kubectl --context "$CONTEXT" -n kafka apply \
  -f config/samples/kafkacluster-prometheus.yaml
kubectl --context "$CONTEXT" -n kafka get \
  servicemonitors,prometheusrules,prometheuses,alertmanagers
kubectl --context "$CONTEXT" -n kafka get pods
```

For an existing Prometheus/Alertmanager installation, integrate the sample's
ServiceMonitors, rule groups, and routes into that installation instead of
deploying duplicate instances. Ensure its selectors include these resources.

## Verify metrics and alert delivery

In Prometheus, check the Targets page for healthy Kafka and Cruise Control
targets. Verify that broker metrics include `namespace`, `kafka_cr`, and
`brokerId`; the broker ServiceMonitor supplies these labels.

Useful metrics include:

| Signal | Metric |
|---|---|
| Request volume | `kafka_network_requestmetrics_requests_total` |
| Partition placement | `kafka_server_replicamanager_partitioncount` |
| Replication health | `kafka_server_replicamanager_underreplicatedpartitions` |
| Minimum ISR health | `kafka_server_replicamanager_underminisrpartitioncount` |
| Request-handler idle | `kafka_server_kafkarequesthandlerpool_requesthandleravgidle_oneminuterate_percent` |
| Network-processor idle | `kafka_network_socketserver_networkprocessoravgidle_percent` |

Metric availability depends on the Kafka version and JMX exporter configuration.
Inspect the exported series before building dashboards or action policies.

Check Prometheus's Alerts page, then Alertmanager's active alerts and receivers.
Use a non-action test alert to verify your human notification integration.
Do not use a broker or storage action alert merely to test notification delivery.

## Interpret the sample broker alerts

| Alert | Example condition | Operator response |
|---|---|---|
| `BrokerRequestRateHigh` | Per-broker request rate over 15 minutes exceeds 30 requests/s for five minutes | Check resource use, request latency, and workload skew |
| `BrokerUnderReplicated` | Under-replicated partitions remain above zero for five minutes | Investigate broker availability, network, disk, and replication |
| `PartitionCountHigh` | A broker has more than 100 partition replicas for three minutes | Review placement and resource use |
| `PartitionCountLow` | The lowest-count broker in a cluster has fewer than 40 partition replicas for three minutes | Check startup, redistribution, and capacity before considering removal |

These are diagnostic examples, not production capacity limits. Request rate
includes internal Kafka traffic, and low partition count does not prove spare
capacity. None of these sample broker alerts carries a scaling command.

## Troubleshooting

| Symptom | Check |
|---|---|
| Missing metrics | ServiceMonitor selectors, namespace selection, metrics port names, network access, and JMX exporter configuration |
| Rules not visible | Prometheus's rule selector and the PrometheusRule labels |
| Alert visible but no human notification | The `monitoring` receiver's integration, routes, silences, inhibition rules, and notification errors |
| Storage action not applied | Storage route and webhook address, PVC labels, StorageClass expansion support, operator logs, and the KafkaCluster/PVC state |
| Unexpected broker scaling | Deployed custom rules and integrations; the sample's monitoring-only defaults are not a global scaling disable switch |

For action limits, webhook acknowledgements, and scaling progress checks,
continue with the [autoscaling user guide](autoscale.md).
