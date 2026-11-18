# Kafka docker image

`adobe/kafka` docker image build configuration.

A new `kafka-*` tag created in this repo triggers the image build and push to [adobe/kafka](https://hub.docker.com/r/adobe/kafka/tags?page=1&ordering=last_updated) docker hub repo.

Tags should be `kafka-<scala_version>-<kafka_version>` e.g `kafak-2.13-2.6.2`

# Upstream base

This is based on [wurstmeister/kafka-docker](https://github.com/wurstmeister/kafka-docker) with the following additions:
1. Use `openjdk 21` in order to support container based resource monitoring
2. Use custom `log4j.properties` to get all kafka logs to stdout only

## Alternate JDK downloads

The Dockerfile defaults to Oracle's public download URL without authentication.
Override `java_download_base_url` and `java_download_version` to use another source.
The Dockerfile appends `java_version` and `latest` for Oracle, or `java_version`
and `java_download_version` for alternate sources. Neither version belongs in the
base URL. Alternate sources must provide the Oracle-style archive filenames and
matching `.sha256` files for each architecture.

For authenticated downloads, pass an optional API key as a BuildKit secret:

```bash
--secret id=JAVA_DOWNLOAD_APIKEY,env=JAVA_DOWNLOAD_APIKEY
```

The key is sent as an `Authorization: Bearer` header, is not stored in the image,
and is not forwarded to a different host on redirects.
Release builds also default to Oracle. To use an alternate source, add the
commented overrides to the release workflow's `build-args`, configure the GitHub
Actions secret `JAVA_DOWNLOAD_BASE_URL`, and select a matching `java_download_version`.
For authenticated downloads, also enable the commented `secrets` input and
configure `JAVA_DOWNLOAD_APIKEY`.
