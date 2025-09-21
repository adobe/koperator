---
title: KafkaTopic CRD schema reference (group kafka.banzaicloud.io)
linkTitle: KafkaTopic
description: |
  KafkaTopic is the Schema for the kafkatopics API
weight: 100
crd:
  name_camelcase: KafkaTopic
  name_plural: kafkatopics
  name_singular: kafkatopic
  group: kafka.banzaicloud.io
  technical_name: kafkatopics.kafka.banzaicloud.io
  scope: Namespaced
  source_repository: ../../
  source_repository_ref: master
  versions:
    - v1alpha1
  topics:
layout: crd
owner:
  - https://github.com/banzaicloud/
aliases:
  - /reference/cp-k8s-api/kafkatopics.kafka.banzaicloud.io/
technical_name: kafkatopics.kafka.banzaicloud.io
source_repository: ../../
source_repository_ref: master
---

## KafkaTopic


KafkaTopic is the Schema for the kafkatopics API
<dl class="crd-meta">
<dt class="fullname">Full name:</dt>
<dd class="fullname">kafkatopics.kafka.banzaicloud.io</dd>
<dt class="groupname">Group:</dt>
<dd class="groupname">kafka.banzaicloud.io</dd>
<dt class="singularname">Singular name:</dt>
<dd class="singularname">kafkatopic</dd>
<dt class="pluralname">Plural name:</dt>
<dd class="pluralname">kafkatopics</dd>
<dt class="scope">Scope:</dt>
<dd class="scope">Namespaced</dd>
<dt class="versions">Versions:</dt>
<dd class="versions"><a class="version" href="#v1alpha1" title="Show schema for version v1alpha1">v1alpha1</a></dd>
</dl>



<div class="crd-schema-version">

## Version v1alpha1 {#v1alpha1}



## Properties {#property-details-v1alpha1}


<div class="property depth-0">
<div class="property-header">
<h3 id="v1alpha1-.apiVersion">.apiVersion</h3>
</div>
<div class="property-body">
<div class="property-meta">
<span class="property-type">string</span>

</div>

<div class="property-description">
<p>APIVersion defines the versioned schema of this representation of an object. Servers should convert recognized schemas to the latest internal value, and may reject unrecognized values. More info: <a href="https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources">https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources</a></p>

</div>

</div>
</div>

<div class="property depth-0">
<div class="property-header">
<h3 id="v1alpha1-.kind">.kind</h3>
</div>
<div class="property-body">
<div class="property-meta">
<span class="property-type">string</span>

</div>

<div class="property-description">
<p>Kind is a string value representing the REST resource this object represents. Servers may infer this from the endpoint the client submits requests to. Cannot be updated. In CamelCase. More info: <a href="https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds">https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds</a></p>

</div>

</div>
</div>

<div class="property depth-0">
<div class="property-header">
<h3 id="v1alpha1-.metadata">.metadata</h3>
</div>
<div class="property-body">
<div class="property-meta">
<span class="property-type">object</span>

</div>

</div>
</div>

<div class="property depth-0">
<div class="property-header">
<h3 id="v1alpha1-.spec">.spec</h3>
</div>
<div class="property-body">
<div class="property-meta">
<span class="property-type">object</span>

</div>

<div class="property-description">
<p>KafkaTopicSpec defines the desired state of KafkaTopic</p>

</div>

</div>
</div>

<div class="property depth-1">
<div class="property-header">
<h3 id="v1alpha1-.spec.clusterRef">.spec.clusterRef</h3>
</div>
<div class="property-body">
<div class="property-meta">
<span class="property-type">object</span>
<span class="property-required">Required</span>
</div>

<div class="property-description">
<p>ClusterReference states a reference to a cluster for topic/user provisioning</p>

</div>

</div>
</div>

<div class="property depth-2">
<div class="property-header">
<h3 id="v1alpha1-.spec.clusterRef.name">.spec.clusterRef.name</h3>
</div>
<div class="property-body">
<div class="property-meta">
<span class="property-type">string</span>
<span class="property-required">Required</span>
</div>

</div>
</div>

<div class="property depth-2">
<div class="property-header">
<h3 id="v1alpha1-.spec.clusterRef.namespace">.spec.clusterRef.namespace</h3>
</div>
<div class="property-body">
<div class="property-meta">
<span class="property-type">string</span>

</div>

</div>
</div>

<div class="property depth-1">
<div class="property-header">
<h3 id="v1alpha1-.spec.config">.spec.config</h3>
</div>
<div class="property-body">
<div class="property-meta">
<span class="property-type">object</span>

</div>

</div>
</div>

<div class="property depth-1">
<div class="property-header">
<h3 id="v1alpha1-.spec.name">.spec.name</h3>
</div>
<div class="property-body">
<div class="property-meta">
<span class="property-type">string</span>
<span class="property-required">Required</span>
</div>

</div>
</div>

<div class="property depth-1">
<div class="property-header">
<h3 id="v1alpha1-.spec.partitions">.spec.partitions</h3>
</div>
<div class="property-body">
<div class="property-meta">
<span class="property-type">integer</span>
<span class="property-required">Required</span>
</div>

<div class="property-description">
<p>Partitions defines the desired number of partitions; must be positive, or -1 to signify using the broker&rsquo;s default</p>

</div>

</div>
</div>

<div class="property depth-1">
<div class="property-header">
<h3 id="v1alpha1-.spec.replicationFactor">.spec.replicationFactor</h3>
</div>
<div class="property-body">
<div class="property-meta">
<span class="property-type">integer</span>
<span class="property-required">Required</span>
</div>

<div class="property-description">
<p>ReplicationFactor defines the desired replication factor; must be positive, or -1 to signify using the broker&rsquo;s default</p>

</div>

</div>
</div>

<div class="property depth-0">
<div class="property-header">
<h3 id="v1alpha1-.status">.status</h3>
</div>
<div class="property-body">
<div class="property-meta">
<span class="property-type">object</span>

</div>

<div class="property-description">
<p>KafkaTopicStatus defines the observed state of KafkaTopic</p>

</div>

</div>
</div>

<div class="property depth-1">
<div class="property-header">
<h3 id="v1alpha1-.status.managedBy">.status.managedBy</h3>
</div>
<div class="property-body">
<div class="property-meta">
<span class="property-type">string</span>
<span class="property-required">Required</span>
</div>

<div class="property-description">
<p>ManagedBy describes who is the manager of the Kafka topic. When its value is not &ldquo;koperator&rdquo; then modifications to the topic configurations of the KafkaTopic CR will not be propagated to the Kafka topic. Manager of the Kafka topic can be changed by adding the &ldquo;managedBy: <manager>&rdquo; annotation to the KafkaTopic CR.</p>

</div>

</div>
</div>

<div class="property depth-1">
<div class="property-header">
<h3 id="v1alpha1-.status.state">.status.state</h3>
</div>
<div class="property-body">
<div class="property-meta">
<span class="property-type">string</span>
<span class="property-required">Required</span>
</div>

<div class="property-description">
<p>TopicState defines the state of a KafkaTopic</p>

</div>

</div>
</div>





</div>



