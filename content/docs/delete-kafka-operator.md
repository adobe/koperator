---
title: Delete the operator
shorttitle: Uninstall
weight: 950
---

In case you want to delete the Koperator from your cluster, note that because of dependencies between the various components, they must be deleted in specific order.

{{< warning >}}It’s important to delete the Koperator deployment as the last step.
{{< /warning >}}

## Uninstall Koperator

1. Delete the Prometheus instance used by the Kafka cluster. If you used the sample Prometheus instance from the Koperator repository you can use the following command, otherwise do this step manually according to the way you deployed the Prometheus instance.

    ```
    kubectl delete \
        -n kafka \
        -f https://raw.githubusercontent.com/banzaicloud/koperator/{{< param "latest_version" >}}/config/samples/kafkacluster-prometheus.yaml
    ```

    Expected output:

    ```
    clusterrole.rbac.authorization.k8s.io/prometheus deleted
    clusterrolebinding.rbac.authorization.k8s.io/prometheus deleted
    prometheus.monitoring.coreos.com/kafka-prometheus deleted
    prometheusrule.monitoring.coreos.com/kafka-alerts deleted
    serviceaccount/prometheus deleted
    servicemonitor.monitoring.coreos.com/cruisecontrol-servicemonitor deleted
    servicemonitor.monitoring.coreos.com/kafka-servicemonitor deleted
    ```

1. Delete KafkaCluster Custom Resource (CR) that represent the Kafka cluster and Cruise Control.

    ```bash
    kubectl delete kafkaclusters -n kafka kafka
    ```

    Example output:

    ```
    kafkacluster.kafka.banzaicloud.io/kafka deleted
    ```
    
    Wait for the Kafka resources (Pods, PersistentVolumeClaims, Configmaps, etc) to be removed.

    ```
    kubectl get pods -n kafka
    ```

    Expected output:

    ```
    NAME                                       READY   STATUS    RESTARTS   AGE
    kafka-operator-operator-8458b45587-286f9   2/2     Running   0          62s
    ```

    You would also need to delete other Koperator-managed CRs (if any) following the same fashion

    > Note: KafkaCluster, KafkaTopic and KafkaUser custom resources are protected with Kubernetes finalizers, so those won’t be actually deleted from Kubernetes until the Koperator removes those finalizers. After the Koperator has finished cleaning up everything, it removes the finalizers. In case you delete the Koperator deployment before it cleans up everything, you need to remove the finalizers manually.


1. Uninstall Koperator deployment.

    ```
    helm uninstall kafka-operator -n kafka
    ```

    Expected output:

    ```
    release "kafka-operator" uninstalled
    ```

1. Delete Koperator Custom Resource Definitions (CRDs).
    ```
    kubectl delete -f https://github.com/adobe/koperator/releases/download/v{{< param "latest_version" >}}/kafka-operator.crds.yaml
    ```

## Uninstall Prometheus operator

1. Uninstall the prometheus-operator deployment.

    ```
    helm uninstall -n prometheus prometheus
    ```

    Expected output:

    ```
    release "prometheus" uninstalled
    ```

1. If no other cluster resources uses prometheus-operator CRDs, delete the prometheus-operator's CRDs.

    > Note: Red Hat OpenShift clusters require those CRDs to function so do not delete those on such clusters.

    ```
    kubectl get crd | grep 'monitoring.coreos.com'| awk '{print $1};' | xargs kubectl delete crd
    ```

## Uninstall Zookeeper Operator

1. Delete Zookeeper CR.

    ```
    kubectl delete zookeeperclusters -n zookeeper zookeeper-server
    ```

    Expected output:

    ```
    zookeeperclusters.zookeeper.pravega.io/zookeeper-server deleted
    ```

    Wait for the Zookeeper resources (Deployment, PersistentVolumeClaims, Configmaps, etc) to be removed.

    ```
    kubectl get pods -n zookeeper
    ```

    Expected output:

    ```
    NAME                                  READY   STATUS    RESTARTS   AGE
    zookeeper-operator-5857967dcc-gm5l5   1/1     Running   0          3m22s
    ```

1. Uninstall the zookeeper-operator deployment.

    ```
    helm uninstall zookeeper-operator -n zookeeper
    ```

1. If no other cluster resource uses Zookeeper CRDs, delete Zookeeper Operator's CRDs

    ```
    kubectl delete customresourcedefinition zookeeperclusters.zookeeper.pravega.io
    ```

## Uninstall Cert-Manager

### Uninstall with Helm

1. Uninstall cert-manager deployment.

    ```
    helm uninstall -n cert-manager cert-manager
    ```

    Expected output:

    ```
    release "cert-manager" uninstalled
    ```

1. If no other cluster resource uses cert-manager CRDs, delete cert-manager's CRDs:

    ```
    kubectl delete -f https://github.com/jetstack/cert-manager/releases/download/v1.11.0/cert-manager.crds.yaml
    ```

    Expected output:

    ```
    customresourcedefinition.apiextensions.k8s.io/certificaterequests.cert-manager.io deleted
    customresourcedefinition.apiextensions.k8s.io/certificates.cert-manager.io deleted
    customresourcedefinition.apiextensions.k8s.io/challenges.acme.cert-manager.io deleted
    customresourcedefinition.apiextensions.k8s.io/clusterissuers.cert-manager.io deleted
    customresourcedefinition.apiextensions.k8s.io/issuers.cert-manager.io deleted
    customresourcedefinition.apiextensions.k8s.io/orders.acme.cert-manager.io deleted
    ```
