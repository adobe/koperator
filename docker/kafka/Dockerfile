ARG scala_version=2.13
# Renovate is pinned to the 3.x line (see the apache/kafka packageRule in renovate.json).
# Kafka 4.x removes ZooKeeper support entirely (KRaft-only) and needs an operator-side
# migration; when this repo is ready for that, drop the allowedVersions constraint too.
# Also bump java_version to 25 (next LTS after 21) as part of that migration.
ARG KAFKA_VERSION=3.9.2 # renovate: datasource=github-tags depName=apache/kafka
ARG java_version=21

FROM alpine:latest@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b AS kafka_dist

ARG scala_version
ARG KAFKA_VERSION
# archive.apache.org retains every historical release permanently; downloads.apache.org
# only mirrors the newest active releases and 404s once a pinned version ages out.
ARG kafka_distro_base_url=https://archive.apache.org/dist/kafka

ENV kafka_distro=kafka_$scala_version-$KAFKA_VERSION.tgz
ENV kafka_distro_asc=$kafka_distro.asc

WORKDIR /var/tmp

# Download, verify, extract and clean up Kafka in single layer
RUN apk add --no-cache gnupg wget && \
    wget -q $kafka_distro_base_url/$KAFKA_VERSION/$kafka_distro && \
    wget -q $kafka_distro_base_url/$KAFKA_VERSION/$kafka_distro_asc && \
    wget -q $kafka_distro_base_url/KEYS && \
    gpg --import KEYS && \
    gpg --verify $kafka_distro_asc $kafka_distro && \
    tar -xzf $kafka_distro && \
    rm -rf kafka_$scala_version-$KAFKA_VERSION/bin/windows \
           kafka_$scala_version-$KAFKA_VERSION/site-docs \
           $kafka_distro $kafka_distro_asc KEYS && \
    find kafka_$scala_version-$KAFKA_VERSION -name "*.bat" -delete


# backported from https://github.com/docker-library/openjdk/blob/master/18/jdk/slim-bullseye/Dockerfile
FROM debian:bullseye-slim@sha256:cba95a21c96c1f5fc2470081829363eed57706634f7dc26e8c6712934303d57a

ARG scala_version
ARG KAFKA_VERSION
ARG java_version
ARG VERSION
ARG BUILT_AT
ARG COMMIT

RUN set -eux; \
	apt-get update; \
	apt-get install -y --no-install-recommends \
		ca-certificates p11-kit \
	; \
	rm -rf /var/lib/apt/lists/*

ENV JAVA_HOME=/usr/local/openjdk-${java_version}
ENV PATH=$JAVA_HOME/bin:$PATH

# Default to UTF-8 file.encoding
ENV LANG=C.UTF-8
ENV JAVA_VERSION=${java_version}

RUN set -eux; \
	\
	arch="$(dpkg --print-architecture)"; \
	case "$arch" in \
		'amd64') \
			downloadUrl="https://download.oracle.com/java/${java_version}/latest/jdk-${java_version}_linux-x64_bin.tar.gz"; \
			downloadSha256="https://download.oracle.com/java/${java_version}/latest/jdk-${java_version}_linux-x64_bin.tar.gz.sha256"; \
			;; \
		'arm64') \
			downloadUrl="https://download.oracle.com/java/${java_version}/latest/jdk-${java_version}_linux-aarch64_bin.tar.gz"; \
			downloadSha256="https://download.oracle.com/java/${java_version}/latest/jdk-${java_version}_linux-aarch64_bin.tar.gz.sha256"; \
			;; \
		*) echo >&2 "error: unsupported architecture: '$arch'"; exit 1 ;; \
	esac; \
	\
	savedAptMark="$(apt-mark showmanual)"; \
	apt-get update; \
	apt-get install -y --no-install-recommends \
		wget \
	; \
	rm -rf /var/lib/apt/lists/*; \
	\
	wget --progress=dot:giga -O openjdk.tgz "$downloadUrl"; \
    SHA256=$(wget -qO- $downloadSha256) ; \
	echo "$SHA256 *openjdk.tgz" | sha256sum --strict --check -; \
	\
	mkdir -p "$JAVA_HOME"; \
	tar --extract \
		--file openjdk.tgz \
		--directory "$JAVA_HOME" \
		--strip-components 1 \
		--no-same-owner \
	; \
	rm openjdk.tgz*; \
	\
# Remove JMODs and src.zip (not needed for runtime, only for development)
	rm -rf "$JAVA_HOME/jmods" "$JAVA_HOME/lib/src.zip"; \
	\
	apt-mark auto '.*' > /dev/null; \
	[ -z "$savedAptMark" ] || apt-mark manual $savedAptMark > /dev/null; \
	apt-get purge -y --auto-remove -o APT::AutoRemove::RecommendsImportant=false; \
	\
# update "cacerts" bundle to use Debian's CA certificates (and make sure it stays up-to-date with changes to Debian's store)
# see https://github.com/docker-library/openjdk/issues/327
#     http://rabexc.org/posts/certificates-not-working-java#comment-4099504075
#     https://salsa.debian.org/java-team/ca-certificates-java/blob/3e51a84e9104823319abeb31f880580e46f45a98/debian/jks-keystore.hook.in
#     https://git.alpinelinux.org/aports/tree/community/java-cacerts/APKBUILD?id=761af65f38b4570093461e6546dcf6b179d2b624#n29
	{ \
		echo '#!/usr/bin/env bash'; \
		echo 'set -Eeuo pipefail'; \
		echo 'trust extract --overwrite --format=java-cacerts --filter=ca-anchors --purpose=server-auth "$JAVA_HOME/lib/security/cacerts"'; \
	} > /etc/ca-certificates/update.d/docker-openjdk; \
	chmod +x /etc/ca-certificates/update.d/docker-openjdk; \
	/etc/ca-certificates/update.d/docker-openjdk; \
	\
# https://github.com/docker-library/openjdk/issues/331#issuecomment-498834472
	find "$JAVA_HOME/lib" -name '*.so' -exec dirname '{}' ';' | sort -u > /etc/ld.so.conf.d/docker-openjdk.conf; \
	ldconfig; \
	\
# https://github.com/docker-library/openjdk/issues/212#issuecomment-420979840
# https://openjdk.java.net/jeps/341
	java -Xshare:dump; \
	\
# basic smoke test
	fileEncoding="$(echo 'System.out.println(System.getProperty("file.encoding"))' | jshell -s -)"; [ "$fileEncoding" = 'UTF-8' ]; rm -rf ~/.java; \
	javac --version; \
	java --version

ENV KAFKA_VERSION=$KAFKA_VERSION \
    SCALA_VERSION=$scala_version \
    KAFKA_HOME=/opt/kafka

ENV PATH=${PATH}:${KAFKA_HOME}/bin

# Install minimal dependencies, setup Kafka, and clean up in single layer
RUN apt-get update && \
    apt-get install -y --no-install-recommends curl && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/* && \
    mkdir -p ${KAFKA_HOME}/libs/extensions

COPY --from=kafka_dist /var/tmp/kafka_$scala_version-$KAFKA_VERSION ${KAFKA_HOME}
COPY opt/kafka/config/log4j.properties ${KAFKA_HOME}/config/log4j.properties

# Optimize Kafka installation: remove unnecessary files and set permissions
RUN chmod a+x ${KAFKA_HOME}/bin/*.sh && \
    chmod g+rwX ${KAFKA_HOME} ${KAFKA_HOME}/libs/extensions && \
    rm -rf ${KAFKA_HOME}/bin/windows ${KAFKA_HOME}/site-docs && \
    find ${KAFKA_HOME} -name "*.bat" -delete

# Add metadata labels
LABEL org.opencontainers.image.title="Apache Kafka"
LABEL org.opencontainers.image.description="Apache Kafka with Oracle JDK ${java_version}"
LABEL org.opencontainers.image.vendor="Adobe"
LABEL org.opencontainers.image.licenses="Apache-2.0"
LABEL org.opencontainers.image.source="https://github.com/adobe/koperator"
LABEL org.opencontainers.image.documentation="https://github.com/adobe/koperator/blob/main/docker/kafka/README.md"
LABEL org.opencontainers.image.version="${VERSION}"
LABEL org.opencontainers.image.created="${BUILT_AT}"
LABEL org.opencontainers.image.revision="${COMMIT}"

CMD ["kafka-server-start.sh"]
