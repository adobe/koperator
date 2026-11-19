# Kafka docker image

`adobe/kafka` docker image build configuration.

A new `kafka-*` tag created in this repo triggers the image build and push to [adobe/kafka](https://hub.docker.com/r/adobe/kafka/tags?page=1&ordering=last_updated) docker hub repo.

Tags should be `kafka-<scala_version>-<kafka_version>`, for example
`kafka-2.13-4.2.2`. Release-candidate tags may append `-rc<number>`,
followed by an optional `-jdk<version>` suffix. The Scala and Kafka versions
must match the Dockerfile defaults.

# Upstream base

This is based on [wurstmeister/kafka-docker](https://github.com/wurstmeister/kafka-docker) with the following additions:
1. Use Oracle JDK 25 LTS with container based resource monitoring
2. Use custom `log4j2.properties` to get all Kafka logs to stdout only

## Java 25 runtime defaults

The image sets `JDK_JAVA_OPTIONS` to:

```text
--enable-native-access=ALL-UNNAMED --illegal-native-access=deny -XX:+UseCompactObjectHeaders
```

These options permit Kafka's classpath-based native compression libraries, reject
native access that has not been granted, and enable compact object headers. The
CDS archive is generated with the same options.

The defaults apply to Java launches in this image, including Kafka's storage and
command-line tools, without replacing `KAFKA_OPTS` used for the JMX agent. They
are not added to the operator's shared JVM defaults: Java 21 images reject the
native-access deny option and compact object headers.

## Java 25 rollout

This branch targets Kafka 4.2.2 on Oracle JDK 25 LTS. Kafka 4 is KRaft-only;
do not use this image with ZooKeeper brokers or unfinished migrations.

Operator defaults, samples, benchmark manifests, and the vendored Cruise Control
Compose configuration still reference the published Java 21 Kafka image. After
validating Kafka startup, SASL/TLS authentication, and the JMX exporter on Java 25,
publish a new `kafka-<scala_version>-<kafka_version>-jdk<jdk_version>` release tag.
Then update those image references and refresh any pinned image digests together.
Do not point them at an unpublished tag or reuse a Java 21 image digest.

The rebuilt JMX exporter uses JDK 25 to compile Java 21 bytecode, preserving
compatibility with Cruise Control's Java 21 runtime. Its architecture-independent
JAR is built and tested on the native build platform, without QEMU. Publish and
validate a new image tag before changing deployed JMX image references.
