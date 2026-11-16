# Kafka docker image

`adobe/kafka` docker image build configuration.

A new `kafka-*` tag created in this repo triggers the image build and push to [adobe/kafka](https://hub.docker.com/r/adobe/kafka/tags?page=1&ordering=last_updated) docker hub repo.

Tags should be `kafka-<scala_version>-<kafka_version>` e.g `kafak-2.13-2.6.2`

# Upstream base

This is based on [wurstmeister/kafka-docker](https://github.com/wurstmeister/kafka-docker) with the following additions:
1. Use Oracle JDK 25 LTS with container based resource monitoring
2. Use custom `log4j.properties` to get all kafka logs to stdout only

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

The Kafka image build targets Java 25 LTS independently of the Kafka 4.x migration.
Kafka remains on 3.9.x to preserve ZooKeeper support.

Operator defaults, samples, benchmark manifests, and the vendored Cruise Control
Compose configuration still reference the published Java 21 Kafka image. After
validating Kafka startup, SASL/TLS authentication, and the JMX exporter on Java 25,
publish a new `kafka-<scala_version>-<kafka_version>-jdk<jdk_version>` release tag.
Then update those image references and refresh any pinned image digests together.
Do not point them at an unpublished tag or reuse a Java 21 image digest.

The rebuilt JMX exporter uses JDK 25 to compile Java 21 bytecode, so the agent
supports both Java 21 and Java 25; the jar is functionally equivalent to the one
built with the previous JDK 21 builder, so no agent rebuild is required before
updating consumers. The release workflow tags the agent image with the
`JMX_EXPORTER_VERSION` from its Dockerfile and skips tags that already exist, so
the JDK 25 builder only takes effect at the next jmx_exporter upstream release.
