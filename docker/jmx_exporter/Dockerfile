ARG JMX_EXPORTER_VERSION=1.7.0 # renovate: datasource=github-releases depName=prometheus/jmx_exporter

FROM maven:3-amazoncorretto-21@sha256:313b3afb28619c7374224b8bb65f9f6cae1dffce3b172e1bd8afe85e4cf05610 AS build
ARG JMX_EXPORTER_VERSION

# Install wget to download the release tarball
RUN yum install -y wget && yum clean all

# Download and extract the JMX exporter release
RUN wget https://github.com/prometheus/jmx_exporter/archive/refs/tags/${JMX_EXPORTER_VERSION}.tar.gz \
    -O /tmp/jmx_exporter.tar.gz && \
    mkdir /src && \
    tar -xzf /tmp/jmx_exporter.tar.gz -C /src --strip-components=1 && \
    rm /tmp/jmx_exporter.tar.gz

WORKDIR /src
RUN mvn -B -Dmaven.javadoc.skip=true -Dskip.javadoc=true \
    -Dmaven.compiler.source=21 -Dmaven.compiler.target=21 -Dmaven.compiler.release=21 \
    clean package -pl jmx_prometheus_javaagent -am && \
    cp "jmx_prometheus_javaagent/target/jmx_prometheus_javaagent-${JMX_EXPORTER_VERSION#v}.jar" /jmx_prometheus_javaagent.jar

FROM alpine:3.24@sha256:3f4d6b32e26703c6082839234f43769d5046b3ac7c910033911d1e71d8cd630a
COPY --from=build /jmx_prometheus_javaagent.jar /opt/jmx_exporter/jmx_prometheus_javaagent.jar
RUN ln -s /opt/jmx_exporter/jmx_prometheus_javaagent.jar /jmx_prometheus_javaagent.jar
CMD ["/bin/sh"]
