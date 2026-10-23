ARG JMX_EXPORTER_VERSION=1.7.0 # renovate: datasource=github-releases depName=prometheus/jmx_exporter

FROM maven:3-amazoncorretto-21@sha256:0ef83b8506e92f34278bf2acc3500ecf1f4bd2cd1159091e1f1e707adc73546a AS build
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

FROM alpine:3.24@sha256:be2656061b536c1883c4034790e21d0a09c5aaaf51968b89368a3c0371ea2c9a
COPY --from=build /jmx_prometheus_javaagent.jar /opt/jmx_exporter/jmx_prometheus_javaagent.jar
RUN ln -s /opt/jmx_exporter/jmx_prometheus_javaagent.jar /jmx_prometheus_javaagent.jar
CMD ["/bin/sh"]
