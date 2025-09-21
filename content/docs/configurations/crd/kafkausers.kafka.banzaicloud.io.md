---
title: KafkaUser CRD schema reference (group kafka.banzaicloud.io)
linkTitle: KafkaUser
description: |
  KafkaUser is the Schema for the kafka users API
weight: 100
crd:
  name_camelcase: KafkaUser
  name_plural: kafkausers
  name_singular: kafkauser
  group: kafka.banzaicloud.io
  technical_name: kafkausers.kafka.banzaicloud.io
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
  - /reference/cp-k8s-api/kafkausers.kafka.banzaicloud.io/
technical_name: kafkausers.kafka.banzaicloud.io
source_repository: ../../
source_repository_ref: master
---

## KafkaUser


KafkaUser is the Schema for the kafka users API
<dl class="crd-meta">
<dt class="fullname">Full name:</dt>
<dd class="fullname">kafkausers.kafka.banzaicloud.io</dd>
<dt class="groupname">Group:</dt>
<dd class="groupname">kafka.banzaicloud.io</dd>
<dt class="singularname">Singular name:</dt>
<dd class="singularname">kafkauser</dd>
<dt class="pluralname">Plural name:</dt>
<dd class="pluralname">kafkausers</dd>
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
<p>KafkaUserSpec defines the desired state of KafkaUser</p>

</div>

</div>
</div>

<div class="property depth-1">
<div class="property-header">
<h3 id="v1alpha1-.spec.annotations">.spec.annotations</h3>
</div>
<div class="property-body">
<div class="property-meta">
<span class="property-type">object</span>

</div>

<div class="property-description">
<p>Annotations defines the annotations placed on the certificate or certificate signing request object</p>

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
<h3 id="v1alpha1-.spec.createCert">.spec.createCert</h3>
</div>
<div class="property-body">
<div class="property-meta">
<span class="property-type">boolean</span>

</div>

</div>
</div>

<div class="property depth-1">
<div class="property-header">
<h3 id="v1alpha1-.spec.dnsNames">.spec.dnsNames</h3>
</div>
<div class="property-body">
<div class="property-meta">
<span class="property-type">array</span>

</div>

</div>
</div>

<div class="property depth-2">
<div class="property-header">
<h3 id="v1alpha1-.spec.dnsNames[*]">.spec.dnsNames[*]</h3>
</div>
<div class="property-body">
<div class="property-meta">
<span class="property-type">string</span>

</div>

</div>
</div>

<div class="property depth-1">
<div class="property-header">
<h3 id="v1alpha1-.spec.includeJKS">.spec.includeJKS</h3>
</div>
<div class="property-body">
<div class="property-meta">
<span class="property-type">boolean</span>

</div>

</div>
</div>

<div class="property depth-1">
<div class="property-header">
<h3 id="v1alpha1-.spec.pkiBackendSpec">.spec.pkiBackendSpec</h3>
</div>
<div class="property-body">
<div class="property-meta">
<span class="property-type">object</span>

</div>

</div>
</div>

<div class="property depth-2">
<div class="property-header">
<h3 id="v1alpha1-.spec.pkiBackendSpec.issuerRef">.spec.pkiBackendSpec.issuerRef</h3>
</div>
<div class="property-body">
<div class="property-meta">
<span class="property-type">object</span>

</div>

<div class="property-description">
<p>ObjectReference is a reference to an object with a given name, kind and group.</p>

</div>

</div>
</div>

<div class="property depth-3">
<div class="property-header">
<h3 id="v1alpha1-.spec.pkiBackendSpec.issuerRef.group">.spec.pkiBackendSpec.issuerRef.group</h3>
</div>
<div class="property-body">
<div class="property-meta">
<span class="property-type">string</span>

</div>

<div class="property-description">
<p>Group of the resource being referred to.</p>

</div>

</div>
</div>

<div class="property depth-3">
<div class="property-header">
<h3 id="v1alpha1-.spec.pkiBackendSpec.issuerRef.kind">.spec.pkiBackendSpec.issuerRef.kind</h3>
</div>
<div class="property-body">
<div class="property-meta">
<span class="property-type">string</span>

</div>

<div class="property-description">
<p>Kind of the resource being referred to.</p>

</div>

</div>
</div>

<div class="property depth-3">
<div class="property-header">
<h3 id="v1alpha1-.spec.pkiBackendSpec.issuerRef.name">.spec.pkiBackendSpec.issuerRef.name</h3>
</div>
<div class="property-body">
<div class="property-meta">
<span class="property-type">string</span>
<span class="property-required">Required</span>
</div>

<div class="property-description">
<p>Name of the resource being referred to.</p>

</div>

</div>
</div>

<div class="property depth-2">
<div class="property-header">
<h3 id="v1alpha1-.spec.pkiBackendSpec.pkiBackend">.spec.pkiBackendSpec.pkiBackend</h3>
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
<h3 id="v1alpha1-.spec.pkiBackendSpec.signerName">.spec.pkiBackendSpec.signerName</h3>
</div>
<div class="property-body">
<div class="property-meta">
<span class="property-type">string</span>

</div>

<div class="property-description">
<p>SignerName indicates requested signer, and is a qualified name.</p>

</div>

</div>
</div>

<div class="property depth-1">
<div class="property-header">
<h3 id="v1alpha1-.spec.secretName">.spec.secretName</h3>
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
<h3 id="v1alpha1-.spec.topicGrants">.spec.topicGrants</h3>
</div>
<div class="property-body">
<div class="property-meta">
<span class="property-type">array</span>

</div>

</div>
</div>

<div class="property depth-2">
<div class="property-header">
<h3 id="v1alpha1-.spec.topicGrants[*]">.spec.topicGrants[*]</h3>
</div>
<div class="property-body">
<div class="property-meta">
<span class="property-type">object</span>

</div>

<div class="property-description">
<p>UserTopicGrant is the desired permissions for the KafkaUser</p>

</div>

</div>
</div>

<div class="property depth-3">
<div class="property-header">
<h3 id="v1alpha1-.spec.topicGrants[*].accessType">.spec.topicGrants[*].accessType</h3>
</div>
<div class="property-body">
<div class="property-meta">
<span class="property-type">string</span>
<span class="property-required">Required</span>
</div>

<div class="property-description">
<p>KafkaAccessType hold info about Kafka ACL</p>

</div>

</div>
</div>

<div class="property depth-3">
<div class="property-header">
<h3 id="v1alpha1-.spec.topicGrants[*].patternType">.spec.topicGrants[*].patternType</h3>
</div>
<div class="property-body">
<div class="property-meta">
<span class="property-type">string</span>

</div>

<div class="property-description">
<p>KafkaPatternType hold the Resource Pattern Type of kafka ACL</p>

</div>

</div>
</div>

<div class="property depth-3">
<div class="property-header">
<h3 id="v1alpha1-.spec.topicGrants[*].topicName">.spec.topicGrants[*].topicName</h3>
</div>
<div class="property-body">
<div class="property-meta">
<span class="property-type">string</span>
<span class="property-required">Required</span>
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
<p>KafkaUserStatus defines the observed state of KafkaUser</p>

</div>

</div>
</div>

<div class="property depth-1">
<div class="property-header">
<h3 id="v1alpha1-.status.acls">.status.acls</h3>
</div>
<div class="property-body">
<div class="property-meta">
<span class="property-type">array</span>

</div>

</div>
</div>

<div class="property depth-2">
<div class="property-header">
<h3 id="v1alpha1-.status.acls[*]">.status.acls[*]</h3>
</div>
<div class="property-body">
<div class="property-meta">
<span class="property-type">string</span>

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
<p>UserState defines the state of a KafkaUser</p>

</div>

</div>
</div>





</div>



