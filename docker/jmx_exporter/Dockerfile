ARG JMX_EXPORTER_VERSION=1.5.0 # renovate: datasource=github-releases depName=prometheus/jmx_exporter

FROM maven:3-amazoncorretto-21@sha256:9e8aa99b4a55060004696aea04eb698ffaf3098752d682f56dd73cca0ed7dae4 AS build
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

FROM alpine:latest@sha256:5b10f432ef3da1b8d4c7eb6c487f2f5a8f096bc91145e68878dd4a5019afde11
ARG JMX_EXPORTER_VERSION
COPY --from=build /src/jmx_prometheus_javaagent/target/jmx_prometheus_javaagent-${JMX_EXPORTER_VERSION}.jar /opt/jmx_exporter/
RUN ln -s /opt/jmx_exporter/jmx_prometheus_javaagent-${JMX_EXPORTER_VERSION}.jar /jmx_prometheus_javaagent.jar
CMD ["/bin/sh"]
