---
title: Upgrade the operator
shorttitle: Upgrade
weight: 15
---

When upgrading your Koperator deployment to a new version, complete the following steps.

1. Update the CRDs for the new release from the main repository.

    {{< warning >}}**Hazard of data loss** Do not delete the old CRD from the cluster. Deleting the CRD removes your Kafka cluster.{{< /warning >}}

1. Replace the KafkaCluster CRDs with the new ones on your cluster by running the following commands:

    ```bash
    kubectl apply -f https://raw.githubusercontent.com/adobe/koperator/refs/heads/master/config/base/crds/kafka.banzaicloud.io_cruisecontroloperations.yaml
    kubectl apply -f https://raw.githubusercontent.com/adobe/koperator/refs/heads/master/config/base/crds/kafka.banzaicloud.io_kafkaclusters.yaml
    kubectl apply -f https://raw.githubusercontent.com/adobe/koperator/refs/heads/master/config/base/crds/kafka.banzaicloud.io_kafkatopics.yaml
    kubectl apply -f https://raw.githubusercontent.com/adobe/koperator/refs/heads/master/config/base/crds/kafka.banzaicloud.io_kafkausers.yaml
    ```

1. Update your Koperator deployment by running:

    ```bash
    helm upgrade kafka-operator \
    oci://ghcr.io/adobe/helm-charts/kafka-operator \
    --namespace=kafka
    ```
