ARG JMX_EXPORTER_VERSION=1.7.0 # renovate: datasource=github-releases depName=prometheus/jmx_exporter

FROM maven:3-amazoncorretto-25@sha256:d96c6ff2e7a3258e6f0c447340d2284a3404917bb8c5f01c8660cc4066bd382c AS build
ARG JMX_EXPORTER_VERSION

# Install tools to download and extract the release tarball
RUN yum install -y wget tar gzip && yum clean all

# Download and extract the JMX exporter release
RUN wget https://github.com/prometheus/jmx_exporter/archive/refs/tags/${JMX_EXPORTER_VERSION}.tar.gz \
    -O /tmp/jmx_exporter.tar.gz && \
    mkdir /src && \
    tar -xzf /tmp/jmx_exporter.tar.gz -C /src --strip-components=1 && \
    rm /tmp/jmx_exporter.tar.gz

WORKDIR /src
# Keep the agent compatible with Java 21 consumers while building with JDK 25.
RUN mvn -B -Dmaven.javadoc.skip=true -Dskip.javadoc=true \
    -Dmaven.compiler.source=21 -Dmaven.compiler.target=21 -Dmaven.compiler.release=21 \
    clean package -pl jmx_prometheus_javaagent -am && \
    cp "jmx_prometheus_javaagent/target/jmx_prometheus_javaagent-${JMX_EXPORTER_VERSION#v}.jar" /jmx_prometheus_javaagent.jar

FROM alpine:3.24@sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6
COPY --from=build /jmx_prometheus_javaagent.jar /opt/jmx_exporter/jmx_prometheus_javaagent.jar
RUN ln -s /opt/jmx_exporter/jmx_prometheus_javaagent.jar /jmx_prometheus_javaagent.jar
CMD ["/bin/sh"]
