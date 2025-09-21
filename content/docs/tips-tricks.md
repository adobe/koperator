---
title: Tips and tricks for the Koperator
shorttitle: Tips and tricks
weight: 970
---

## Rebalancing

The Koperator installs Cruise Control (CC) to oversee your Kafka cluster. When you change the cluster (for example, add new nodes), the Koperator engages CC to perform a rebalancing if needed. How and when CC performs rebalancing depends on its settings (see goal settings in the official CC documentation) and on how long CC was trained with Kafka’s behavior (this may take weeks).

You can also trigger rebalancing manually from the CC UI:

```bash
kubectl port-forward -n kafka svc/kafka-cruisecontrol-svc 8090:8090
```

Cruise Control UI will be available at [http://localhost:8090](http://localhost:8090).

## Headless service

When the **headlessServiceEnabled** option is enabled (true) in your KafkaCluster CR, it creates a headless service for accessing the kafka cluster from within the Kubernetes cluster.

When the **headlessServiceEnabled** option is disabled (false), it creates a ClusterIP service. When using a ClusterIP service, your client application doesn’t need to be aware of every Kafka broker endpoint, it simply connects to *kafka-all-broker:29092* which covers dynamically all the available brokers. That way if the Kafka cluster is scaled dynamically, there is no need to reconfigure the client applications.

## Retrieving broker configuration during downscale operation

When a broker is downscaling, the broker configuration is missing from the kafkaCluster/spec/brokers field. You can retrieve the last broker configuration with the following command.

```bash
echo <value of the kafkaCluster/status/brokerState/brokerID/configurationBackup> | base64 -d | gzip -d
```
