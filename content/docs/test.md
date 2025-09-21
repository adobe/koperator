---
title: Test provisioned Kafka Cluster
shorttitle: Test your cluster
weight: 100
---

## Create Topic

Topic creation by default is enabled in Apache Kafka, but if it is configured otherwise, you'll need to create a topic first.

- You can use the `KafkaTopic` CR to create a topic called **my-topic** like this:

    {{< include-code "create-topic.sample" "yaml" >}}

    > Note: The previous command will fail if the cluster has not finished provisioning.

    Expected output:

    ```bash
    kafkatopic.kafka.banzaicloud.io/my-topic created
    ```

- To create a sample topic from the CLI you can run the following:

    For internal listeners exposed by a headless service (`KafkaCluster.spec.headlessServiceEnabled `set to `true`):

    ```bash
    kubectl -n kafka run kafka-topics -it --image=ghcr.io/banzaicloud/kafka:2.13-3.1.0 --rm=true --restart=Never -- /opt/kafka/bin/kafka-topics.sh --bootstrap-server kafka-headless.kafka:29092 --topic my-topic --create --partitions 1 --replication-factor 1
    ```

    For internal listeners exposed by a regular service (`KafkaCluster.spec.headlessServiceEnabled` set to `false`):

    ```bash
    kubectl -n kafka run kafka-topics -it --image=ghcr.io/banzaicloud/kafka:2.13-3.1.0 --rm=true --restart=Never -- /opt/kafka/bin/kafka-topics.sh --bootstrap-server kafka-all-broker.kafka:29092 --topic my-topic --create --partitions 1 --replication-factor 1
    ```

After you have created a topic, produce and consume some messages:

- [Send and receive messages without SSL within a cluster](#internal-nossl)
- [Send and receive messages with SSL within a cluster](#internal-ssl)

## Send and receive messages without SSL within a cluster {#internal-nossl}

You can use the following commands to send and receive messages within a Kubernetes cluster when SSL encryption is disabled for Kafka.

- Produce messages:

    1. Start the producer container

        ```bash
        kubectl run \
        -n kafka \
        kafka-producer \
        -it \
        --image=ghcr.io/banzaicloud/kafka:2.13-3.1.0 \
        --rm=true \
        --restart=Never \
        -- \
        /opt/kafka/bin/kafka-console-producer.sh \
        --bootstrap-server kafka-headless:29092 \
        --topic my-topic
        ```

    1. Wait for the producer container to run, this may take a couple seconds.

        Expected output:

        ```bash
        If you don't see a command prompt, try pressing enter.
        ```

    1. Press enter to get a command prompt.

        Expected output:

        ```bash
        >
        ```

    1. Type your messages and press enter, each line will be sent through Kafka.

        Example:

        ```bash
        > test
        > message
        >

    1. Stop the container. (You can CTRL-D out of it.)

        Expected output:

        ```bash
        pod "kafka-producer" deleted
        ```

- Consume messages:

    1. Start the consumer container.

        ```bash
        kubectl run \
        -n kafka \
        kafka-consumer \
        -it \
        --image=ghcr.io/banzaicloud/kafka:2.13-3.1.0 \
        --rm=true \
        --restart=Never \
        -- \
        /opt/kafka/bin/kafka-console-consumer.sh \
        --bootstrap-server kafka-headless:29092 \
        --topic my-topic \
        --from-beginning
        ```

    1. Wait for the consumer container to run, this may take a couple seconds.

        Expected output:

        ```bash
        If you don't see a command prompt, try pressing enter.
        ```

    1. The messages sent by the producer should be displayed here.

        Example:

        ```bash
        test
        message
        ```

    1. Stop the container. (You can CTRL-C out of it.)

        Expected output:

        ```bash
        Processed a total of 3 messages
        pod "kafka-consumer" deleted
        pod kafka/kafka-consumer terminated (Error)
        ```

## Send and receive messages with SSL within a cluster {#internal-ssl}

You can use the following procedure to send and receive messages within a Kubernetes cluster [when SSL encryption is enabled for Kafka]({{< relref "/docs/ssl.md#enable-ssl" >}}). To test a Kafka instance secured by SSL we recommend using [kcat](https://github.com/edenhill/kcat).

> To use the java client instead of kcat, generate the proper truststore and keystore using the [official docs](https://kafka.apache.org/documentation/#security_ssl).

1. Create a Kafka user. The client will use this user account to access Kafka. You can use the KafkaUser custom resource to customize the access rights as needed. For example:

    {{< include-code "create-kafkauser.sample" "bash" >}}

1. To use Kafka inside the cluster, create a Pod which contains `kcat`. Create a `kafka-test` pod in the `kafka` namespace. Note that the value of the **secretName** parameter must be the same as you used when creating the KafkaUser resource, for example, example-kafkauser-secret.

    {{< include-code "kafkacat-ssl.sample" "bash" >}}

1. Wait until the pod is created, then exec into the container:

    ```bash
    kubectl exec -it -n kafka kafka-test -- sh
    ```

1. Run the following command to check that you can connect to the brokers.

    ```bash
    kcat -L -b kafka-headless:29092 -X security.protocol=SSL -X ssl.key.location=/ssl/certs/tls.key -X ssl.certificate.location=/ssl/certs/tls.crt -X ssl.ca.location=/ssl/certs/ca.crt
    ```

    The first line of the output should indicate that the communication is encrypted, for example:

    ```text
    Metadata for all topics (from broker -1: ssl://kafka-headless:29092/bootstrap):
    ```

1. Produce some test messages. Run:

    ```bash
    kcat -P -b kafka-headless:29092 -t my-topic \
    -X security.protocol=SSL \
    -X ssl.key.location=/ssl/certs/tls.key \
    -X ssl.certificate.location=/ssl/certs/tls.crt \
    -X ssl.ca.location=/ssl/certs/ca.crt
    ```

    And type some test messages.

1. Consume some messages.
    The following command will use the certificate provisioned with the cluster to connect to Kafka. If you'd like to create and use a different user, create a `KafkaUser` CR, for details, see the [SSL documentation](../ssl/).

    ```bash
    kcat -C -b kafka-headless:29092 -t my-topic \
    -X security.protocol=SSL \
    -X ssl.key.location=/ssl/certs/tls.key \
    -X ssl.certificate.location=/ssl/certs/tls.crt \
    -X ssl.ca.location=/ssl/certs/ca.crt
    ```

    You should see the messages you have created.

## Send and receive messages outside a cluster

### Prerequisites {#external-prerequisites}

1. Producers and consumers that are not in the same Kubernetes cluster can access the Kafka cluster only if an [external listener]({{< relref "/docs/external-listener/index.md" >}}) is configured in your KafkaCluster CR. Check that the **listenersConfig.externalListeners** section exists in the KafkaCluster CR.
1. Obtain the external address and port number of the cluster by running the following commands.

    <!-- FIXME How is this different if we use nodeport for the external listener? -->
    - If the external listener uses a LoadBalancer:

        ```bash
        export SERVICE_IP=$(kubectl get svc --namespace kafka -o jsonpath="{.status.loadBalancer.ingress[0].hostname}" envoy-loadbalancer-external-kafka); echo $SERVICE_IP

        export SERVICE_PORTS=($(kubectl get svc --namespace kafka -o jsonpath="{.spec.ports[*].port}" envoy-loadbalancer-external-kafka)); echo ${SERVICE_PORTS[@]}

        # depending on the shell you are using, arrays may be indexed starting from 0 or 1
        export SERVICE_PORT=${SERVICE_PORTS[@]:0:1}; echo $SERVICE_PORT
        ```

1. If the external listener of your Kafka cluster accepts encrypted connections, proceed to [SSL enabled](#external-ssl). Otherwise, proceed to [SSL disabled](#external-nossl).

### SSL disabled {#external-nossl}

1. Produce some test messages on the the external client.

    - If you have [kcat](https://github.com/edenhill/kcat) installed, run:

        ```bash
        kcat -P -b $SERVICE_IP:$SERVICE_PORT -t my-topic
        ```

    - If you have the Java Kafka client installed, run:

        ```bash
        kafka-console-producer.sh --bootstrap-server $SERVICE_IP:$SERVICE_PORT --topic my-topic
        ```

    And type some test messages.

1. Consume some messages.

    - If you have [kcat](https://github.com/edenhill/kcat) installed, run:

        ```bash
        kcat -C -b $SERVICE_IP:$SERVICE_PORT -t my-topic
        ```

    - If you have the Java Kafka client installed, run:

        ```bash
        kafka-console-consumer.sh --bootstrap-server $SERVICE_IP:$SERVICE_PORT --topic my-topic --from-beginning
        ```

    You should see the messages you have created.

### SSL enabled {#external-ssl}

You can use the following procedure to send and receive messages from an external host that is outside a Kubernetes cluster when SSL encryption is enabled for Kafka. To test a Kafka instance secured by SSL we recommend using [kcat](https://github.com/edenhill/kcat).

> To use the java client instead of kcat, generate the proper truststore and keystore using the [official docs](https://kafka.apache.org/documentation/#security_ssl).

1. Install kcat.

    - __MacOS__:

        ```bash
        brew install kcat
        ```

    - __Ubuntu__:

        ```bash
        apt-get update
        apt-get install kcat
        ```

1. Connect to the Kubernetes cluster that runs your Kafka deployment.

1. Create a Kafka user. The client will use this user account to access Kafka. You can use the KafkaUser custom resource to customize the access rights as needed. For example:

    {{< include-code "create-kafkauser.sample" "bash" >}}

1. Download the certificate and the key of the user, and the CA certificate used to verify the certificate of the Kafka server. These are available in the Kubernetes Secret created for the KafkaUser.

    ```bash
    kubectl get secrets -n kafka <name-of-the-user-secret> -o jsonpath="{['data']['tls\.crt']}" | base64 -D > client.crt.pem
    kubectl get secrets -n kafka <name-of-the-user-secret> -o jsonpath="{['data']['tls\.key']}" | base64 -D > client.key.pem
    kubectl get secrets -n kafka <name-of-the-user-secret> -o jsonpath="{['data']['ca\.crt']}" | base64 -D > ca.crt.pem
    ```

1. Copy the downloaded certificates to a location that is accessible to the external host.

1. If you haven't done so already, [obtain the external address and port number of the cluster](#external-prerequisites).

1. Produce some test messages on the host that is outside your cluster.

    ```bash
    kcat -b $SERVICE_IP:$SERVICE_PORT -P -X security.protocol=SSL \
    -X ssl.key.location=client.key.pem \
    -X ssl.certificate.location=client.crt.pem \
    -X ssl.ca.location=ca.crt.pem \
    -t my-topic
    ```

    And type some test messages.

1. Consume some messages.

    ```bash
    kcat -b $SERVICE_IP:$SERVICE_PORT -C -X security.protocol=SSL \
    -X ssl.key.location=client.key.pem \
    -X ssl.certificate.location=client.crt.pem \
    -X ssl.ca.location=ca.crt.pem \
    -t my-topic
    ```

    You should see the messages you have created.