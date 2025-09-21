---
title: Provisioning Kafka Topics
shorttitle: Kafka topics
weight: 280
---

## Create topic

You can create Kafka topics either:

- directly against the cluster with command line utilities, or
- via the `KafkaTopic` CRD.

Below is an example `KafkaTopic` CR you can apply with kubectl.

{{< include-code "create-topic.sample" "yaml" >}}

For a full list of configuration options, see the [official Kafka documentation](https://kafka.apache.org/documentation/#topicconfigs).

## Update topic

If you want to update the configuration of the topic after it's been created, you can either:

- edit the manifest and run `kubectl apply` again, or 
- run `kubectl edit -n kafka kafkatopic example-topic` and then update the configuration in the editor that gets spawned.

You can increase the partition count for a topic the same way, or by running the following one-liner using `patch`:

```shell
kubectl patch -n kafka kafkatopic example-topic --patch '{"spec": {"partitions": 5}}' --type=merge

kafkatopic.kafka.banzaicloud.io/example-topic patched
```

> Note: Topics created by the Koperator are not enforced in any way. From the Kubernetes perspective, Kafka Topics are external resources.
