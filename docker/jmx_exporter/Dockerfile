ARG JMX_EXPORTER_VERSION=1.5.0 # renovate: datasource=github-releases depName=prometheus/jmx_exporter

FROM maven:3-amazoncorretto-21@sha256:82d98fbed447e3f7dfbf1089840a51bfaeb5651cb47a9c5820139d054db3dde1 AS build
ARG JMX_EXPORTER_VERSION

# Install wget to download the release tarball
RUN yum install -y wget && yum clean all

# Download and extract the JMX exporter release
RUN wget https://github.com/prometheus/jmx_exporter/archive/refs/tags/${JMX_EXPORTER_VERSION}.tar.gz \
    -O /tmp/jmx_exporter.tar.gz && \
    tar -xzf /tmp/jmx_exporter.tar.gz -C /tmp && \
    mv /tmp/jmx_exporter-${JMX_EXPORTER_VERSION} /src && \
    rm /tmp/jmx_exporter.tar.gz

WORKDIR /src
RUN mvn -B -Dmaven.javadoc.skip=true -Dskip.javadoc=true \
    -Dmaven.compiler.source=21 -Dmaven.compiler.target=21 -Dmaven.compiler.release=21 \
    clean package -pl jmx_prometheus_javaagent -am

FROM alpine:latest@sha256:51183f2cfa6320055da30872f211093f9ff1d3cf06f39a0bdb212314c5dc7375
ARG JMX_EXPORTER_VERSION
COPY --from=build /src/jmx_prometheus_javaagent/target/jmx_prometheus_javaagent-${JMX_EXPORTER_VERSION}.jar /opt/jmx_exporter/
RUN ln -s /opt/jmx_exporter/jmx_prometheus_javaagent-${JMX_EXPORTER_VERSION}.jar /jmx_prometheus_javaagent.jar
CMD ["/bin/sh"]
