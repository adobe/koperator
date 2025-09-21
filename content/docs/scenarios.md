---
title: Koperator capablities
weight: 400
---

As highlighted in the [features section]({{< relref "_index.md#features" >}}), Koperator removed the reliance on StatefulSet,and supports several different usecases.

> Note: This is not a complete list, if you have a specific requirement or question, see our [support]({{< relref "/docs/support.md">}}) options.

## Vertical capacity scaling

You may have encountered situations where the horizontal scaling of a cluster is impossible. When **only one Broker is throttling** and needs more CPU or requires additional disks (because it handles the most partitions), a StatefulSet-based solution is useless, since it does not distinguish between replicas' specifications. The handling of such a case requires *unique* Broker configurations. If there is a need to add a new disk to a unique Broker, there can be a waste of disk space (and money) with a StatefulSet-based solution, since it can't add a disk to a specific Broker, the StatefulSet adds one to each replica.

With the [Koperator](https://github.com/adobe/koperator), adding a new disk to any Broker is as easy as changing a CR configuration. Similarly, any Broker-specific configuration can be done on a Broker by Broker basis.

## An unhandled error with Broker #1 in a three Broker cluster

In the event of an error with Broker #1, it is ideal to handle it without disrupting the other Brokers. To handle the error you would like to temporarily remove this Broker from the cluster, and fix its state, reconciling the node that serves the node, or maybe reconfigure the Broker using a new configuration. Again, when using StatefulSet, you lose the ability to remove specific Brokers from the cluster. StatefulSet only supports a field name replica that determines how many replicas an application should use. If there's a downscale/removal, this number can be lowered, however, this means that Kubernetes will remove the most recently added Pod (Broker #3) from the cluster - which, in this case, happens to suit the above purposes quite well.

To remove the #1 Broker from the cluster, you need to lower the number of brokers in the cluster from three to one. This will cause a state in which only one Broker is live, while you kill the brokers that handle traffic. Koperator supports removing specific brokers without disrupting traffic in the cluster.

## Fine grained Broker config support

Apache Kafka is a stateful application, where Brokers create/form a cluster with other Brokers. Every Broker is uniquely configurable (Koperator supports heterogenous environments, in which no nodes are the same, act the same or have the same specifications - from the infrastructure up through the Brokers' Envoy configuration). Kafka has lots of Broker configs, which can be used to fine tune specific brokers, and Koperator did not want to limit these to ALL Brokers in a StatefulSet. Koperator supports unique Broker configs.

*In each of the three scenarios listed above, Koperator does not use StatefulSet, relying, instead, on Pods, PVCs and ConfigMaps. While using StatefulSet is a very convenient starting point, as it handles roughly 80% of scenarios, it also introduces huge limitations when running Kafka on Kubernetes in production.*

## Monitoring based control

Use of monitoring is essential for any application, and all relevant information about Kafka should be published to a monitoring solution. When using Kubernetes, the de facto solution is Prometheus, which supports configuring alerts based on previously consumed metrics. Koperator was built as a standards-based solution (Prometheus and Alert Manager) that could handle and react to alerts automatically, so human operators wouldn't have to. Koperator supports alert-based Kafka cluster management.

## LinkedIn's Cruise Control

LinkedIn knows how to operate Kafka in a better way. They built a tool, called Cruise Control, to operate their Kafka infrastructure. And Koperator is built to **handle the infrastructure, but not to reinvent the wheel in so far as operating Kafka**. Koperator was built to leverage the Kubernetes operator pattern and our Kubernetes expertise by handling all Kafka infrastructure related issues in the best possible way. Managing Kafka can be a separate issue, for which there already exist some unique tools and solutions that are standard across the industry, so LinkedIn's Cruise Control is integrated with the Koperator.
