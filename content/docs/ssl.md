---
title: Securing Kafka With SSL
shorttitle: SSL
weight: 300
---

The Koperator makes securing your Apache Kafka cluster with SSL simple.

## Enable SSL encryption in Apache Kafka {#enable-ssl}

To create an Apache Kafka cluster which has listener(s) with SSL encryption enabled, you must enable SSL encryption and configure the secrets in the **listenersConfig** section of your **KafkaCluster** Custom Resource. You can either provide your own CA certificate and the corresponding private key, or let the operator to create them for you from your cluster configuration. Using **sslSecrets**, Koperator generates client and server certificates signed using CA. The server certificate is shared across listeners. The client certificate is used by the Koperator, Cruise Control, and Cruise Control Metrics Reporter to communicate Kafka brokers using listener with SSL enabled.

Providing custom certificates per listener is supported from Koperator version 0.21.0+. Having configurations where certain external listeners use user provided certificates while others rely on the auto-generated ones provided by Koperator are also supported. See details below.

## Using auto-generated certificates (**ssLSecrets**)

{{< include-headless "warning-listener-protocol.md" >}}

The following example enables SSL and automatically generates the certificates:

{{< include-code "enable-ssl.sample" "yaml" >}}

If `sslSecrets.create` is `false`, the operator will look for the secret at `sslSecrets.tlsSecretName` in the namespace of the **KafkaCluster** custom resource and expect these values:

| Key          | Value              |
|:------------:|:-------------------|
| `caCert`     | The CA certificate |
| `caKey`      | The CA private key |

## Using own certificates

### Listeners not used for internal broker and controller communication

In [this **KafkaCluster** custom resource](https://github.com/adobe/koperator/blob/master/config/samples/kafkacluster_with_ssl_hybrid_customcert.yaml), SSL is enabled for all listeners, and certificates are automatically generated for "inner" and "controller" listeners. The "external" and "internal" listeners will use the user-provided certificates. The **serverSSLCertSecret** key is a reference to the Kubernetes secret that contains the server certificate for the listener to be used for SSL communication.

In the server secret, the following keys must be set:

| Key              | Value                                     |
|:----------------:|:------------------------------------------|
| `keystore.jks`   | Certificate and private key in JKS format |
| `truststore.jks` | Trusted CA certificate in JKS format      |
| `password`       | Password for the key and trust store      |

The certificates in the listener configuration must be in JKS format.

### Listeners used for internal broker or controller communication

In [this **KafkaCluster** custom resource](https://github.com/adobe/koperator/blob/master/config/samples/kafkacluster_with_ssl_groups_customcert.yaml), SSL is enabled for all listeners, and user-provided server certificates. In that case, when a custom certificate is used for a listener which is used for internal broker or controller communication, you must also specify the client certificate. The client certificate will be used by Koperator, Cruise Control, Cruise Control Metrics Reporter to communicate on SSL. The **clientSSLCertSecret** key is a reference to the Kubernetes secret where the custom client SSL certificate can be provided. The client certificate must be signed by the same CA authority as the server certificate for the corresponding listener. The **clientSSLCertSecret** has to be in the **KafkaCluster** custom resource spec field.
The client secret must contain the keystore and truststore JKS files and the password for them in base64 encoded format.

In the server secret the following keys must be set:

| Key              | Value                                     |
|:----------------:|:------------------------------------------|
| `keystore.jks`   | Certificate and private key in JKS format |
| `truststore.jks` | Trusted CA certificate in JKS format      |
| `password`       | Password for the key and trust store      |

In the client secret the following keys must be set:

| Key              | Value                                     |
|:----------------:|:------------------------------------------|
| `keystore.jks`   | Certificate and private key in JKS format |
| `truststore.jks` | Trusted CA certificate in JKS format      |
| `password`       | Password for the key and trust store      |

### Generate JKS certificate

Certificates in JKS format can be generated using OpenSSL and keystore applications. You can also use [this script](https://github.com/confluentinc/confluent-platform-security-tools/blob/master/kafka-generate-ssl.sh). The `keystore.jks` file must contain only one **PrivateKeyEntry**.

Kafka listeners use 2-way-SSL mutual authentication, so you must properly set the CNAME (Common Name) fields and if needed the SAN (Subject Alternative Name) fields in the certificates. In the following description we assume that the Kafka cluster is in the `kafka` namespace.

- **For the client certificate**, CNAME must be "kafka-controller.kafka.mgt.cluster.local" (where .kafka. is the namespace of the kafka cluster).
- **For internal listeners which are exposed by a headless service** (kafka-headless), CNAME must be "kafka-headless.kafka.svc.cluster.local", and the SAN field must contain the following:

    - *.kafka-headless.kafka.svc.cluster.local
    - kafka-headless.kafka.svc.cluster.local
    - *.kafka-headless.kafka.svc
    - kafka-headless.kafka.svc
    - *.kafka-headless.kafka
    - kafka-headless.kafka
    - kafka-headless

- **For internal listeners which are exposed by a normal service** (kafka-all-broker), CNAME must be "kafka-all-broker.kafka.svc.cluster.local"
- **For external listeners**, you need to use the advertised load balancer hostname as CNAME. The hostname need to be specified in the **KafkaCluster** custom resource with **hostnameOverride**, and the **accessMethod** has to be "LoadBalancer". For details about this override, see Step 5 in {{% xref "/docs/external-listener/index.md#loadbalancer" %}}.

## Using Kafka ACLs with SSL

> {{< include-headless "kafka-operator-intro.md" >}}

If you choose not to enable ACLs for your Apache Kafka cluster, you may still use the `KafkaUser` resource to create new certificates for your applications.
You can leave the `topicGrants` out as they will not have any effect.

1. To enable ACL support for your Apache Kafka cluster, pass the following configurations along with your `brokerConfig`:

    ```yaml
    authorizer.class.name=kafka.security.authorizer.AclAuthorizer
    allow.everyone.if.no.acl.found=false
    ```

1. The operator will ensure that cruise control and itself can still access the cluster, however, to create new clients
you will need to generate new certificates signed by the CA, and ensure ACLs on the topic. The operator can automate this process for you using the `KafkaUser` CRD.
    For example, to create a new producer for the topic `test-topic` against the KafkaCluster `kafka`, apply the following configuration:

    ```bash
    cat << EOF | kubectl apply -n kafka -f -
    apiVersion: kafka.banzaicloud.io/v1alpha1
    kind: KafkaUser
    metadata:
      name: example-producer
      namespace: kafka
    spec:
      clusterRef:
        name: kafka
      secretName: example-producer-secret
      topicGrants:
        - topicName: test-topic
          accessType: write
    EOF
    ```

    This will create a user and store its credentials in the secret `example-producer-secret`. The secret contains these fields:

    | Key          | Value                |
    |:------------:|:---------------------|
    | `ca.crt`     | The CA certificate   |
    | `tls.crt`    | The user certificate |
    | `tls.key`    | The user private key |

1. You can then mount these secrets to your pod. Alternatively, you can write them to your local machine by running:

    ```bash
    kubectl get secret example-producer-secret -o jsonpath="{['data']['ca\.crt']}" | base64 -d > ca.crt
    kubectl get secret example-producer-secret -o jsonpath="{['data']['tls\.crt']}" | base64 -d > tls.crt
    kubectl get secret example-producer-secret -o jsonpath="{['data']['tls\.key']}" | base64 -d > tls.key
    ```

1. To create a consumer for the topic, run this command:

    ```bash
    cat << EOF | kubectl apply -n kafka -f -
    apiVersion: kafka.banzaicloud.io/v1alpha1
    kind: KafkaUser
    metadata:
      name: example-consumer
      namespace: kafka
    spec:
      clusterRef:
        name: kafka
      secretName: example-consumer-secret
      includeJKS: true
      topicGrants:
        - topicName: test-topic
          accessType: read
    EOF
    ```

1. The operator can also include a Java keystore format (JKS) with your user secret if you'd like. Add `includeJKS: true` to the `spec` like shown above, and then the user-secret will gain these additional fields:

    | Key                     | Value                |
    |:-----------------------:|:---------------------|
    | `tls.jks`               | The java keystore containing both the user keys and the CA (use this for your keystore AND truststore) |
    | `pass.txt`              | The password to decrypt the JKS (this will be randomly generated) |
