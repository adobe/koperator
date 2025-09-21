---
title: Monitoring Apache Kafka on Kubernetes
shorttitle: Monitoring
weight: 600
---



This documentation shows you how to enable custom monitoring on an Apache Kafka cluster installed using the [Koperator](/products/kafka-operator/).

## Using Helm for Prometheus

By default, the Koperator does not set annotations on the broker pods. To set annotations on the broker pods, specify them in the KafkaCluster CR. Also, you must open port 9020 on brokers and in CruiseControl to enable scraping. For example:

```yaml
brokerConfigGroups:
  default:
    brokerAnnotations:
      prometheus.io/scrape: "true"
      prometheus.io/port: "9020"

...

cruiseControlConfig:
  cruiseControlAnnotations:
    prometheus.io/port: "9020"
    prometheus.io/scrape: "true"
```

Prometheus must be configured to recognize these annotations. The following example contains the required config.

```yaml
# Example scrape config for pods
#
# The relabeling allows the actual pod scrape endpoint to be configured via the
# following annotations:
#
# * `prometheus.io/scrape`: Only scrape pods that have a value of `true`
# * `prometheus.io/path`: If the metrics path is not `/metrics` override this.
# * `prometheus.io/port`: Scrape the pod on the indicated port instead of the default of `9102`.
- job_name: 'kubernetes-pods'

  kubernetes_sd_configs:
    - role: pod

  relabel_configs:
    - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_scrape]
      action: keep
      regex: true
    - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_path]
      action: replace
      target_label: __metrics_path__
      regex: (.+)
    - source_labels: [__address__, __meta_kubernetes_pod_annotation_prometheus_io_port]
      action: replace
      regex: ([^:]+)(?::\d+)?;(\d+)
      replacement: $1:$2
      target_label: __address__
```

If you are using the provided [CR](https://github.com/adobe/koperator/blob/master/config/samples/banzaicloud_v1beta1_kafkacluster.yaml), the operator installs the official [jmx exporter](https://github.com/prometheus/jmx_exporter) for Prometheus.

To change this behavior, modify the following lines at the end of the CR.

```yaml
monitoringConfig:
   jmxImage describes the used prometheus jmx exporter agent container
    jmxImage: "ghcr.io/amuraru/jmx-javaagent:0.19.1"
   pathToJar describes the path to the jar file in the given image
    pathToJar: "/opt/jmx_exporter/jmx_prometheus_javaagent-0.19.1.jar"
   kafkaJMXExporterConfig describes jmx exporter config for Kafka
    kafkaJMXExporterConfig: |
     lowercaseOutputName: true
```

## Using the ServiceMonitors

To use ServiceMonitors, we recommend to use Kafka with unique service/broker instead of headless service.

Configure the CR the following way:

```yaml
  # Specify if the cluster should use headlessService for Kafka or individual services
  # using service/broker may come in handy in case of service mesh
  headlessServiceEnabled: false
```

Disabling Headless service means the operator will set up Kafka with unique services per broker.

Once you have a cluster up and running, create as many ServiceMonitors as brokers.

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: kafka-0
spec:
  selector:
    matchLabels:
      app: kafka
      brokerId: "0"
      kafka_cr: kafka
  endpoints:
  - port: metrics
    interval: 10s
```
