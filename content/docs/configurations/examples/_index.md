---
title: KafkaCluster CR Examples
shorttitle: KafkaCluster CR Examples
weight: 7000
---

The following KafkaCluster custom resource examples show you some basic use cases.
You can use these examples as a base for your own Kafka cluster.

## KafkaCluster CR with detailed explanation

This is our most descriptive KafkaCluster CR. You can find a lot of valuable explanation about the settings.

- [Detailed CR with descriptions](https://github.com/adobe/koperator/blob/master/config/samples/banzaicloud_v1beta1_kafkacluster.yaml)

## Kafka cluster with monitoring

This is a very simple KafkaCluster CR with Prometheus monitoring enabled.

- [Simple KafkaCluster with monitoring](https://github.com/adobe/koperator/blob/master/config/samples/simplekafkacluster.yaml)

## Kafka cluster with ACL, SSL, and rack awareness

You can read more details about rack awareness [here]({{< relref "../../rackawareness/index.md" >}}).

- [Use SSL and rack awareness](https://github.com/adobe/koperator/blob/master/config/samples/kafkacluster_with_ssl_groups.yaml)

## Kafka cluster with broker configuration

- [Use broker configuration groups](https://github.com/adobe/koperator/blob/master/config/samples/kafkacluster_without_ssl_groups.yaml)
- [Use independent broker configurations](https://github.com/adobe/koperator/blob/master/config/samples/kafkacluster_without_ssl.yaml)

## Kafka cluster with custom SSL certificates for external listeners

You can specify custom SSL certificates for listeners.  
For details about SSL configuration, see {{% xref "../../ssl.md" %}}.

- [Use custom SSL certificate for an external listener](https://github.com/adobe/koperator/blob/master/config/samples/kafkacluster_with_external_ssl_customcert.yaml)
- [Use custom SSL certificate for controller and inter-broker communication](https://github.com/adobe/koperator/blob/master/config/samples/kafkacluster_with_ssl_groups_customcert.yaml). In this case you also need to provide the client SSL certificate for Koperator.  
- [Hybrid solution](https://github.com/adobe/koperator/blob/master/config/samples/kafkacluster_with_ssl_hybrid_customcert.yaml): some listeners have custom SSL certificates and some use certificates Koperator has generated automatically using cert-manager.

## Kafka cluster with SASL

You can use SASL authentication on the listeners.
For details, see {{% xref "../../external-listener/index.md" %}}.

- [Use SASL authentication on the listeners](https://github.com/adobe/koperator/blob/master/config/samples/simplekafkacluster_with_sasl.yaml)

## Kafka cluster with load balancers and brokers in the same availability zone

You can create a broker-ingress mapping to eliminate traffic across availability zones between load balancers and brokers by configuring load balancers for brokers in same availability zone.

- [Load balancers and brokers in same availability zone](https://github.com/adobe/koperator/blob/master/config/samples/simplekafkacluster-with-brokerbindings.yaml)

## Kafka cluster with Istio

You can use Istio as the ingress controller for your external listeners. Koperator now uses standard Istio resources (Gateway, VirtualService) instead of the deprecated banzaicloud istio-operator, providing better compatibility and working with any Istio installation.

- [Kafka cluster with Istio as ingress controller](https://github.com/adobe/koperator/blob/master/config/samples/kafkacluster-with-istio.yaml)

## Kafka cluster with custom advertised address for external listeners and brokers

You can set custom advertised IP address for brokers.  
This is useful when you're advertising the brokers on an IP address different from the Kubernetes node IP address.  
You can also set custom advertised address for external listeners.  
For details, see {{% xref "../../external-listener/index.md" %}}.

- [Custom advertised address for external listeners](https://github.com/adobe/koperator/blob/master/config/samples/simplekafkacluster-with-nodeport-external.yaml)

## Kafka cluster with Kubernetes scheduler affinity settings

You can set node [affinity](https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/) for your brokers.

- [Custom affinity settings](https://github.com/adobe/koperator/blob/master/config/samples/simplekafkacluster_affinity.yaml)

## Kafka cluster with custom storage class

You can configure your brokers to use custom [storage classes](https://kubernetes.io/docs/concepts/storage/storage-classes/).

- [Custom storage class](https://github.com/adobe/koperator/blob/master/config/samples/simplekafkacluster_ebs_csi.yaml)

## Kafka cluster with KRaft mode (ZooKeeper-free)

You can deploy Kafka clusters using KRaft mode, which eliminates the need for ZooKeeper by using Kafka's built-in consensus mechanism. This is the future of Kafka and is recommended for new deployments.

- [Simple KafkaCluster with KRaft mode](https://github.com/adobe/koperator/blob/master/config/samples/kraft/simplekafkacluster_kraft.yaml)

For detailed information about KRaft configuration and deployment, see {{% xref "../../kraft.md" %}}.
