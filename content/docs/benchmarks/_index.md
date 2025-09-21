---
title: Benchmarking Kafka
weight: 900
---

How to setup the environment for the Kafka Performance Test.

## GKE

1. Create a test cluster with 3 nodes for ZooKeeper, 3 for Kafka, 1 Master node and 2 node for clients.

    Once your cluster is up and running you can set up the Kubernetes infrastructure.

1. Create a StorageClass which enables high performance disk requests.

    ```bash
    kubectl create -f - <<EOF
    apiVersion: storage.k8s.io/v1
    kind: StorageClass
    metadata:
      name: fast-ssd
    provisioner: kubernetes.io/gce-pd
    parameters:
      type: pd-ssd
    volumeBindingMode: WaitForFirstConsumer
    EOF
    ```

## EKS

1. Create a test cluster with 3 nodes for ZooKeeper, 3 for Kafka, 1 Master node and 2 node for clients.

    {{< include-headless "warning-ebs-csi-driver.md" >}}

    Once your cluster is up and running you can set up the Kubernetes infrastructure.

1. Create a StorageClass which enables high performance disk requests.

    ```bash
    kubectl create -f - <<EOF
    apiVersion: storage.k8s.io/v1
    kind: StorageClass
    metadata:
      name: fast-ssd
    provisioner: kubernetes.io/aws-ebs
    parameters:
      type: io1
      iopsPerGB: "50"
      fsType: ext4
    volumeBindingMode: WaitForFirstConsumer
    EOF
    ```

## Install other required components

1. Create a ZooKeeper cluster with 3 replicas using Pravega's Zookeeper Operator.

    ```bash
    helm repo add banzaicloud-stable https://kubernetes-charts.banzaicloud.com/
    helm install zookeeper-operator --namespace=zookeeper --create-namespace pravega/zookeeper-operator
    kubectl create -f - <<EOF
    apiVersion: zookeeper.pravega.io/v1beta1
    kind: ZookeeperCluster
    metadata:
      name: zookeeper-server
      namespace: zookeeper
    spec:
      replicas: 3
    EOF
    ```

1. Install the Koperator CustomResourceDefinition resources (adjust the version number to the Koperator release you want to install) and the corresponding version of Koperator, the Operator for managing Apache Kafka on Kubernetes.

    ```bash
    kubectl create --validate=false -f https://github.com/adobe/koperator/releases/download/v{{< param "latest_version" >}}/kafka-operator.crds.yaml
    ```

    ```bash
    helm install kafka-operator --namespace=kafka --create-namespace banzaicloud-stable/kafka-operator
    ```

1. Create a 3-broker Kafka Cluster using [this YAML file](https://raw.githubusercontent.com/banzaicloud/koperator/master/docs/benchmarks/infrastructure/kafka.yaml).

    This will install 3 brokers with fast SSD. If you would like the brokers in different zones, modify the following configurations to match your environment and use them in the broker configurations:

    ```yaml
    apiVersion: kafka.banzaicloud.io/v1beta1
    kind: KafkaCluster
    ...
    spec:
      ...
      brokerConfigGroups:
        default:
          affinity:
            nodeAffinity:
              requiredDuringSchedulingIgnoredDuringExecution:
                nodeSelectorTerms:
                - matchExpressions:
                  - key: <node-label-key>
                    operator: In
                    values:
                    - <node-label-value-zone-1>
                    - <node-label-value-zone-2>
                    - <node-label-value-zone-3>
      ...
    ```

1. Create a client container inside the cluster

    ```bash
    kubectl create -f - <<EOF
    apiVersion: v1
    kind: Pod
    metadata:
      name: kafka-test
    spec:
      containers:
      - name: kafka-test
        image: "wurstmeister/kafka:2.12-2.1.1"
        # Just spin & wait forever
        command: [ "/bin/bash", "-c", "--" ]
        args: [ "while true; do sleep 3000; done;" ]
    EOF
    ```

1. Exec into this client and create the `perftest, perftest2, perftes3` topics.

    For internal listeners exposed by a headless service (`KafkaCluster.spec.headlessServiceEnabled` is set to `true`):

    ```bash

    kubectl exec -it kafka-test -n kafka bash
    ./opt/kafka/bin/kafka-topics.sh --bootstrap-server kafka-headless.kafka:29092 --topic perftest --create --replication-factor 3 --partitions 3
    ./opt/kafka/bin/kafka-topics.sh --bootstrap-server kafka-headless.kafka:29092 --topic perftest2 --create --replication-factor 3 --partitions 3
    ./opt/kafka/bin/kafka-topics.sh --bootstrap-server kafka-headless.kafka:29092 --topic perftest3 --create --replication-factor 3 --partitions 3
    ```

    For internal listeners exposed by a regular service (`KafkaCluster.spec.headlessServiceEnabled` set to `false`):

    ```bash
    kubectl exec -it kafka-test -n kafka bash
    ./opt/kafka/bin/kafka-topics.sh --bootstrap-server kafka-all-broker.kafka:29092 --topic perftest --create --replication-factor 3 --partitions 3
    ./opt/kafka/bin/kafka-topics.sh --bootstrap-server kafka-all-broker.kafka:29092 --topic perftest2 --create --replication-factor 3 --partitions 3
    ./opt/kafka/bin/kafka-topics.sh --bootstrap-server kafka-all-broker.kafka:29092 --topic perftest3 --create --replication-factor 3 --partitions 3
    ```

Monitoring environment is automatically installed. To monitor the infrastructure we used the official Node Exporter dashboard available with id `1860`.

## Run the tests

1. Run performance test against the cluster, by building [this Docker image](https://raw.githubusercontent.com/banzaicloud/koperator/master/docs/benchmarks/loadgens/Dockerfile).

    ```bash
    docker build -t <yourname>/perfload:0.1.0 /loadgens
    docker push <yourname>/perfload:0.1.0
    ```

1. Submit the performance testing application:

    ```yaml
    kubectl create -f - <<EOF
    apiVersion: apps/v1
    kind: Deployment
    metadata:
      labels:
        app: loadtest
      name: perf-load
      namespace: kafka
    spec:
      progressDeadlineSeconds: 600
      replicas: 4
      revisionHistoryLimit: 10
      selector:
        matchLabels:
          app: loadtest
      strategy:
        rollingUpdate:
          maxSurge: 25%
          maxUnavailable: 25%
        type: RollingUpdate
      template:
        metadata:
          creationTimestamp: null
          labels:
            app: loadtest
        spec:
          containers:
          - args:
            - -brokers=kafka-0:29092,kafka-1:29092,kafka-2:29092
            - -topic=perftest
            - -required-acks=all
            - -message-size=512
            - -workers=20
            - -api-version=3.1.0
            image: yourorg/yourimage:yourtag
            imagePullPolicy: Always
            name: sangrenel
            resources: {}
            terminationMessagePath: /dev/termination-log
            terminationMessagePolicy: File
          dnsPolicy: ClusterFirst
          restartPolicy: Always
          schedulerName: default-scheduler
          securityContext: {}
          terminationGracePeriodSeconds: 30
    EOF
    ```
