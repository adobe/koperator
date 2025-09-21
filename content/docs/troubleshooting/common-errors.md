---
title: Common errors
weight: 100
---

## Upgrade failed

If you get the following error in the logs of the Koperator, update your KafkaCluster CRD. This error typically occurs when you upgrade your Koperator to a new version, but forget to update the KafkaCluster CRD.

```bash
Error: UPGRADE FAILED: cannot patch "kafka" with kind KafkaCluster: KafkaCluster.kafka.banzaicloud.io "kafka" is invalid
```

The recommended way to upgrade the Koperator is to upgrade the KafkaCluster CRD, then update the Koperator. For details, see {{% xref "/docs/upgrade-kafka-operator.md" %}}.
