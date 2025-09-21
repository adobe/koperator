---
title: Troubleshooting the operator
shorttitle: Troubleshooting
weight: 980
---

The following tips and commands can help you to troubleshoot your Koperator installation.

## First things to do

1. Verify that the Koperator pod is running. Issue the following command: `kubectl get pods -n kafka|grep kafka-operator`
    The output should include a running pod, for example:

    ```bash
    NAME                                          READY   STATUS      RESTARTS   AGE
    kafka-operator-operator-6968c67c7b-9d2xq   2/2     Running   0          10m
    ```

1. Verify that the Kafka broker pods are running. Issue the following command: `kubectl get pods -n kafka`
    The output should include a numbered running pod for each broker, with names like *kafka-0-zcxk7*, *kafka-1-2nhj5*, and so on, for example:

    ```bash
    NAME                                       READY   STATUS    RESTARTS   AGE
    kafka-0-zcxk7                              1/1     Running   0          3h16m
    kafka-1-2nhj5                              1/1     Running   0          3h15m
    kafka-2-z4t84                              1/1     Running   0          3h15m
    kafka-cruisecontrol-7f77ccf997-cqhsw       1/1     Running   1          3h15m
    kafka-operator-operator-6968c67c7b-9d2xq   2/2     Running   0          3h17m
    prometheus-kafka-prometheus-0              2/2     Running   1          3h16m
    ```

1. If you see any problems, check the logs of the affected pod, for example:

    ```bash
    kubectl logs kafka-0-zcxk7 -n kafka
    ```

1. Check the status (State) of your resources. For example:

    ```bash
    kubectl get KafkaCluster kafka -n kafka -o jsonpath="{.status}" |jq
    ```

1. Check the status of your ZooKeeper deployment, and the logs of the zookeeper-operator and zookeeper pods.

    ```bash
    kubectl get pods -n zookeeper
    ```

## Check the KafkaCluster configuration

You can display the current configuration of your Kafka cluster using the following command:
`kubectl describe KafkaCluster kafka -n kafka`

The output looks like the following:

```yaml
apiVersion: kafka.banzaicloud.io/v1beta1
kind: KafkaCluster
metadata:
  creationTimestamp: "2022-11-21T16:02:55Z"
  finalizers:
  - finalizer.kafkaclusters.kafka.banzaicloud.io
  - topics.kafkaclusters.kafka.banzaicloud.io
  - users.kafkaclusters.kafka.banzaicloud.io
  generation: 4
  labels:
    controller-tools.k8s.io: "1.0"
  name: kafka
  namespace: kafka
  resourceVersion: "3474369"
  uid: f8744017-1264-47d4-8b9c-9ee982728ecc
spec:
  brokerConfigGroups:
    default:
      storageConfigs:
      - mountPath: /kafka-logs
        pvcSpec:
          accessModes:
          - ReadWriteOnce
          resources:
            requests:
              storage: 10Gi
      terminationGracePeriodSeconds: 120
  brokers:
  - brokerConfigGroup: default
    id: 0
  - brokerConfigGroup: default
    id: 1
  clusterImage: ghcr.io/adobe/kafka:2.13-3.9.1
  cruiseControlConfig:
    clusterConfig: |
      {
        "min.insync.replicas": 3
      }
    config: |
    ...
    cruiseControlTaskSpec:
      RetryDurationMinutes: 0
  disruptionBudget: {}
  envoyConfig: {}
  headlessServiceEnabled: true
  istioIngressConfig: {}
  listenersConfig:
    externalListeners:
    - containerPort: 9094
      externalStartingPort: 19090
      name: external
      type: plaintext
    internalListeners:
    - containerPort: 29092
      name: plaintext
      type: plaintext
      usedForInnerBrokerCommunication: true
    - containerPort: 29093
      name: controller
      type: plaintext
      usedForControllerCommunication: true
      usedForInnerBrokerCommunication: false
  monitoringConfig: {}
  oneBrokerPerNode: false
  readOnlyConfig: |
    auto.create.topics.enable=false
    cruise.control.metrics.topic.auto.create=true
    cruise.control.metrics.topic.num.partitions=1
    cruise.control.metrics.topic.replication.factor=2
  rollingUpgradeConfig:
    failureThreshold: 1
  zkAddresses:
  - zookeeper-server-client.zookeeper:2181
status:
  alertCount: 0
  brokersState:
    "0":
      configurationBackup: H4sIAAAAAAAA/6pWykxRsjLQUUoqys9OLXLOz0vLTHcvyi8tULJSSklNSyzNKVGqBQQAAP//D49kqiYAAAA=
      configurationState: ConfigInSync
      gracefulActionState:
        cruiseControlState: GracefulUpscaleSucceeded
        volumeStates:
          /kafka-logs:
            cruiseControlOperationReference:
              name: kafka-rebalance-bhs7n
            cruiseControlVolumeState: GracefulDiskRebalanceSucceeded
      image: ghcr.io/adobe/kafka:2.13-3.9.1
      perBrokerConfigurationState: PerBrokerConfigInSync
      rackAwarenessState: ""
      version: 3.1.0
    "1":
      configurationBackup: H4sIAAAAAAAA/6pWykxRsjLUUUoqys9OLXLOz0vLTHcvyi8tULJSSklNSyzNKVGqBQQAAP//pYq+WyYAAAA=
      configurationState: ConfigInSync
      gracefulActionState:
        cruiseControlState: GracefulUpscaleSucceeded
        volumeStates:
          /kafka-logs:
            cruiseControlOperationReference:
              name: kafka-rebalance-bhs7n
            cruiseControlVolumeState: GracefulDiskRebalanceSucceeded
      image: ghcr.io/adobe/kafka:2.13-3.9.1
      perBrokerConfigurationState: PerBrokerConfigInSync
      rackAwarenessState: ""
      version: 3.1.0
  cruiseControlTopicStatus: CruiseControlTopicReady
  listenerStatuses:
    externalListeners:
      external:
      - address: a0abb7ab2e4a142d793f0ec0cb9b58ae-1185784192.eu-north-1.elb.amazonaws.com:29092
        name: any-broker
      - address: a0abb7ab2e4a142d793f0ec0cb9b58ae-1185784192.eu-north-1.elb.amazonaws.com:19090
        name: broker-0
      - address: a0abb7ab2e4a142d793f0ec0cb9b58ae-1185784192.eu-north-1.elb.amazonaws.com:19091
        name: broker-1
    internalListeners:
      plaintext:
      - address: kafka-headless.kafka.svc.cluster.local:29092
        name: headless
      - address: kafka-0.kafka-headless.kafka.svc.cluster.local:29092
        name: broker-0
      - address: kafka-1.kafka-headless.kafka.svc.cluster.local:29092
        name: broker-1
  rollingUpgradeStatus:
    errorCount: 0
    lastSuccess: ""
  state: ClusterRunning
```

## Getting Support

If you encounter any problems that the documentation does not address, [file an issue](https://github.com/adobe/koperator/issues).

Various [support]({{< relref "/docs/support.md">}}) channels are also available for Koperator.

Before asking for help, prepare the following information to make troubleshooting faster:

- Koperator version
- Kubernetes version (**kubectl version**)
- Helm/chart version (if you installed the Koperator with Helm)
- Koperator logs, for example **kubectl logs kafka-operator-operator-6968c67c7b-9d2xq manager -n kafka** and **kubectl logs kafka-operator-operator-6968c67c7b-9d2xq kube-rbac-proxy -n kafka**
- Kafka broker logs
- Koperator configuration
- Kafka cluster configuration (**kubectl describe KafkaCluster kafka -n kafka**)
- ZooKeeper configuration (**kubectl describe ZookeeperCluster zookeeper-server -n zookeeper**)
- ZooKeeper logs (**kubectl logs zookeeper-operator-5c9b597bcc-vkdz9 -n zookeeper**)
Do not forget to remove any sensitive information (for example, passwords and private keys) before sharing.
