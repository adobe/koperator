---
title: Supported versions and compatibility matrix
shorttitle: Supported versions
weight: 770
---

This page shows you the list of supported Koperator versions, and the versions of other components they are compatible with.


## Available Koperator images

> **Note**: Starting from version 0.25.0, Koperator images are published to `ghcr.io/adobe/koperator` instead of `ghcr.io/banzaicloud/kafka-operator`.

|Image|Go version|
|-|-|
|ghcr.io/adobe/koperator:{{< param "latest_version" >}}|1.26|

## Available Apache Kafka images

> **Note**: Starting from version 0.25.0, Kafka images are published to `ghcr.io/adobe/koperator/kafka` instead of `ghcr.io/banzaicloud/kafka`.

|Image|Java version|
|-|-|
|ghcr.io/adobe/koperator/kafka:2.13-3.9.2|21|

## Available JMX Exporter images

|Image|Java version|
|-|-|
|ghcr.io/adobe/koperator/jmx-javaagent:1.4.0|21|

## Available Cruise Control images

|Image|Java version|
|-|-|
|adobe/cruise-control:3.0.3-adbe-20250804|21|
