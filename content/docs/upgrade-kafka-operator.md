---
title: Upgrade the operator
shorttitle: Upgrade
weight: 15
---

When upgrading your Koperator deployment to a new version, complete the following steps.

1. Download the CRDs for the new release from the [Koperator releases page](https://github.com/adobe/koperator/releases). They are included in the assets of the release.

    {{< warning >}}**Hazard of data loss** Do not delete the old CRD from the cluster. Deleting the CRD removes your Kafka cluster.{{< /warning >}}

1. Replace the KafkaCluster CRD with the new one on your cluster by running the following command (replace &lt;versionnumber> with the release you are upgrading to, for example, **v0.14.0**).

    ```bash
    kubectl replace --validate=false -f https://github.com/adobe/koperator/releases/download/<versionnumber>/kafka-operator.crds.yaml
    ```

1. Update your Koperator deployment by running:

    ```bash
    helm repo update
    helm upgrade kafka-operator --namespace=kafka banzaicloud-stable/kafka-operator
    ```
