---
title: Kafka clusters with pre-provisioned volumes
shorttitle: Static volumes
weight: 300
---


This guide describes how to configure `KafkaCluster` to deploy Apache Kafka clusters which use pre-provisioned volumes instead of dynamically provisioned ones. Using static volumes is useful in environments where dynamic volume provisioning is not supported. Koperator uses [persistent volume claim](https://kubernetes.io/docs/concepts/storage/persistent-volumes/#persistentvolumeclaims) Kubernetes resources to dynamically provision volumes for the Kafka broker log directories.

 Kubernetes provides a feature which allows binding persistent volume claims to existing persistent volumes either through the `volumeName` or the `selector` field. This allows Koperator to use pre-created [persistent volumes](https://kubernetes.io/docs/concepts/storage/persistent-volumes/#persistent-volumes) as Kafka broker log directories instead of dynamically provisioning persistent volumes. For this binding to work:

- the configuration fields specified under `storageConfigs.pvcSpec` (such as `accessModes`, `storageClassName`) must match the specification of the pre-created persistent volume, and
- the `resources.requests.storage` must fit onto the capacity of the persistent volume.

For further details on how the persistent volume claim binding works, consult the Kubernetes documentation.

In the following example, it is assumed that you (or your administrator) have already created four persistent volumes. The example shows you how to create a Kafka cluster with two brokers, each broker configured with two log directories to use the four pre-provisioned volumes:

```yaml
apiVersion: v1
kind: PersistentVolume
metadata:
  labels:
    namespace: kafka # namespace of the kafka cluster this volume is for in case there are multiple kafka clusters with the same name in different namespaces
    kafka_cr: kafka # name of the kafka cluster this volume is for
    brokerId: "0" # the id of the broker this volume is for
    mountPath: kafka-logs-1 # path mounted as broker log dir  
spec:
  ...
  capacity:
    storage: 10Gi
  volumeMode: Filesystem
  accessModes:
  - ReadWriteOnce
  storageClassName: my-storage-class
  ...
---
apiVersion: v1
kind: PersistentVolume
metadata:
  labels:
    namespace: kafka # namespace of the kafka cluster this volume is for in case there are multiple kafka clusters with the same name in different namespaces
    kafka_cr: kafka # name of the kafka cluster this volume is for
    brokerId: "0" # the id of the broker this volume is for
    mountPath: kafka-logs-2 # path mounted as broker log dir  
spec:
  ...
  capacity:
    storage: 10Gi
  volumeMode: Filesystem
  accessModes:
  - ReadWriteOnce
  storageClassName: my-storage-class
  ...
---
apiVersion: v1
kind: PersistentVolume
metadata:
  labels:
    namespace: kafka # namespace of the kafka cluster this volume is for in case there are multiple kafka clusters with the same name in different namespaces
    kafka_cr: kafka # name of the kafka cluster this volume is for
    brokerId: "1" # the id of the broker this volume is for
    mountPath: kafka-logs-1 # path mounted as broker log dir  
spec:
  ...
  capacity:
    storage: 10Gi
  volumeMode: Filesystem
  accessModes:
  - ReadWriteOnce
  storageClassName: my-storage-class
  ...
---
apiVersion: v1
kind: PersistentVolume
metadata:
  labels:
    namespace: kafka # namespace of the kafka cluster this volume is for in case there are multiple kafka clusters with the same name in different namespaces
    kafka_cr: kafka # name of the kafka cluster this volume is for
    brokerId: "1" # the id of the broker this volume is for
    mountPath: kafka-logs-2 # path mounted as broker log dir  
spec:
  ...
  capacity:
    storage: 10Gi
  volumeMode: Filesystem
  accessModes:
  - ReadWriteOnce
  storageClassName: my-storage-class
  ...
```

## Broker-level storage configuration to use pre-provisioned volumes

The `storageConfigs` specified at the broker level to use the above described pre-created persistent volumes as broker log dirs:

```yaml
apiVersion: kafka.banzaicloud.io/v1beta1
kind: KafkaCluster
metadata:
  namespace: kafka
  name: kafka
spec:
...
brokers:
  - id: 0
    brokerConfigGroup: default
    brokerConfig:
      storageConfigs:
      - mountPath: /kafka-logs-1
        pvcSpec:
          accessModes:
          - ReadWriteOnce
          resources:
            requests:
              storage: 10Gi
          selector: # bind to pre-provisioned persistent volumes by labels
            matchLabels:
              namespace: kafka
              kafka_cr: kafka
              brokerId: "0"
              # strip '/' from mount path as label selector values
              # has to start with an alphanumeric character': https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#syntax-and-character-set
              mountPath: '{{ trimPrefix "/" .MountPath }}' 
          storageClassName: my-storage-class
      - mountPath: /kafka-logs-2
        pvcSpec:
          accessModes:
          - ReadWriteOnce
          resources:
            requests:
              storage: 10Gi
          selector: # bind to pre-provisioned persistent volumes by labels
            matchLabels:
              namespace: kafka
              kafka_cr: kafka
              brokerId: "0"
              # strip '/' from mount path as label selector values
              # has to start with an alphanumeric character': https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#syntax-and-character-set
              mountPath: '{{ trimPrefix "/" .MountPath }}'
          storageClassName: my-storage-class
  - id: 1
    brokerConfigGroup: default
    brokerConfig:
      storageConfigs:
      - mountPath: /kafka-logs-1
        pvcSpec:
          accessModes:
          - ReadWriteOnce
          resources:
            requests:
              storage: 10Gi
          selector: # bind to pre-provisioned persistent volumes by labels
            matchLabels:
              namespace: kafka
              kafka_cr: kafka
              brokerId: "1"
              # strip '/' from mount path as label selector values
              # has to start with an alphanumeric character': https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#syntax-and-character-set
              mountPath: '{{ trimPrefix "/" .MountPath }}'
          storageClassName: my-storage-class
      - mountPath: /kafka-logs-2
        pvcSpec:
          accessModes:
          - ReadWriteOnce
          resources:
            requests:
              storage: 10Gi
          selector: # bind to pre-provisioned persistent volumes by labels
            matchLabels:
              namespace: kafka
              kafka_cr: kafka
              brokerId: "1"
              # strip '/' from mount path as label selector values
              # has to start with an alphanumeric character': https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#syntax-and-character-set
              mountPath: '{{ trimPrefix "/" .MountPath }}'
          storageClassName: my-storage-class
```

## Broker configuration group level storage config to use pre-provisioned volumes

The `storageConfigs` specified at the broker configuration group level to use the above described pre-created persistent volumes as broker log dirs:

```yaml
apiVersion: kafka.banzaicloud.io/v1beta1
kind: KafkaCluster
metadata:
  namespace: kafka
  name: kafka
spec:
  brokerConfigGroups:
    default:
      storageConfigs:
      - mountPath: /kafka-logs-1
        pvcSpec:
          accessModes:
          - ReadWriteOnce
          resources:
            requests:
              storage: 10Gi
          selector: # bind to pre-provisioned persistent volumes by labels
            matchLabels:
              namespace: kafka
              kafka_cr: kafka
              brokerId: '{{ .BrokerId }}'
              # strip '/' from mount path as label selector values
              # has to start with an alphanumeric character': https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#syntax-and-character-set
              mountPath: '{{ trimPrefix "/" .MountPath }}'
          storageClassName: my-storage-class
      - mountPath: /kafka-logs-2
        pvcSpec:
          accessModes:
          - ReadWriteOnce
          resources:
            requests:
              storage: 10Gi
          selector: # bind to pre-provisioned persistent volumes by labels
            matchLabels:
              namespace: kafka
              kafka_cr: kafka
              brokerId: '{{ .BrokerId }}'
              # strip '/' from mount path as label selector values
              # has to start with an alphanumeric character': https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#syntax-and-character-set
              mountPath: '{{ trimPrefix "/" .MountPath }}'
          storageClassName: my-storage-class
      - mountPath: /mountpath/that/exceeds63characters/kafka-logs-123456789123456789
        pvcSpec:
          accessModes:
          - ReadWriteOnce
          resources:
            requests:
              storage: 10Gi
          selector: # bind to pre-provisioned persistent volumes by labels
            matchLabels:
              namespace: kafka
              kafka_cr: kafka
              brokerId: '{{ .BrokerId }}'
              # use sha1sum of mountPath to not exceed the 63 char limit for label selector values
              mountPath: '{{ .MountPath | sha1sum }}'
          storageClassName: my-storage-class
...
```

## Storage config data fields

The following data fields are supported in the storage config:

- `.BrokerID` - resolves to the current broker's Id
- `.MountPath` - resolves to the value of the `mountPath` field of the current storage config

Under the hood, `go-templates` enhanced with [Sprig functions](http://masterminds.github.io/sprig/) are used to resolve these fields to values that allow alterations to the resulting value (for examples, see above the use of `trimPrefix` and `sha1sum` template functions).
