---
title: CruiseControlOperation to manage Cruise Control
shorttitle: Cruise Control Operations
weight: 200
---


Koperator version 0.22 introduces the `CruiseControlOperation` custom resource. Koperator executes the Cruise Control related task based on the state of the `CruiseControlOperation` custom resource. This gives you better control over Cruise Control, improving reliability, configurability, and observability.

## Overview

When a broker is added or removed from the Kafka cluster or when new storage is added for a broker, Koperator creates a `CruiseControlOperation` custom resource.
This custom resource describes a task that Cruise Control executes to move the partitions.
Koperator watches the created `CruiseControlOperation` custom resource and updates its state based on the result of the Cruise Control task.
Koperator can re-execute the task if it fails.

Cruise Control can execute only one task at a time, so the priority of the tasks depends on the type of the operation:

- Upscale operations are executed first, then
- downscale operations, then
- rebalance operations.

The following Cruise Control tasks are supported:

- [add_broker](https://github.com/linkedin/cruise-control/wiki/REST-APIs#add-a-list-of-new-brokers-to-kafka-cluster) (`GracefulUpscale`)
- [remove_broker](https://github.com/linkedin/cruise-control/wiki/REST-APIs#decommission-a-list-of-brokers-from-the-kafka-cluster) (`GracefulDownscale`)
- [rebalance](https://github.com/linkedin/cruise-control/wiki/REST-APIs#trigger-a-workload-balance) (`GracefulDiskRebalance`)

You can follow the progress of the operation through the `KafkaCluster` custom resource's status and through the `CruiseControlOperation` custom resource's status.  
The following example shows the steps of an `add_broker` (`GracefulUpscale*`) operation, but the same applies for the Kafka cluster `remove_broker` (`GracefulDownScale*`) and `rebalance` (when the `volumeState` is `GracefulDiskRebalance*`) operations.

1. Upscale the Kafka cluster by adding a new broker with id "3" into the `KafkaCluster` CR:

    ```yaml
    spec:
    ...  
      brokers:
        - id: 0
          brokerConfigGroup: "default"
        - id: 1
          brokerConfigGroup: "default"
        - id: 2
          brokerConfigGroup: "default"
        - id: 3
          brokerConfigGroup: "default"
    ...
    ```

1. A new broker pod is created and the `cruiseControlOperationReference` is added to the `KafkaCluster` status.  
   This is the reference of the created `CruiseControlOperation` custom resource.  
   The `cruiseControlState` shows the `CruiseControlOperation` state: `GracefulUpscaleScheduled`, meaning that `CruiseControlOperation` has been created and is waiting for the `add_broker` task to be finished.

    ```yaml
    status:
    ...
      brokersState:
        "3":
         ...
          gracefulActionState:
            cruiseControlOperationReference:
              name: kafka-addbroker-mhh72
            cruiseControlState: GracefulUpscaleScheduled
            volumeStates:
              /kafka-logs:
                cruiseControlOperationReference:
                  name: kafka-rebalance-h6ntt
                cruiseControlVolumeState: GracefulDiskRebalanceScheduled
              /kafka-logs2:
                cruiseControlOperationReference:
                  name: kafka-rebalance-h6ntt
                cruiseControlVolumeState: GracefulDiskRebalanceScheduled
        ...
    ```

1. The `add_broker` Cruise Control task is in progress:

    ```yaml
    status:
    ...
      brokersState:
        "3":
         ...
          gracefulActionState:
            cruiseControlOperationReference:
              name: kafka-addbroker-mhh72
            cruiseControlState: GracefulUpscaleRunning
        ...
    ```

1. When the `add_broker` Cruise Control task is completed:

    ```yaml
    status:
    ...
      brokersState:
        "3":
         ...
          gracefulActionState:
            cruiseControlOperationReference:
              name: kafka-addbroker-mhh72
            cruiseControlState: GracefulUpscaleSucceeded
        ...
    ```

There are two other possible states of `cruiseControlState`, `GracefulUpscaleCompletedWithError` and `GracefulUpscalePaused`.  

- `GracefulUpscalePaused` is a special state. For details, see [Control the created CruiseControlOperation](#control-cruisecontroloperation).
- The `GracefulUpscaleCompletedWithError` occurs when the Cruise Control task fails. If the `cruiseControlOperation.spec.errorPolicy` is set to `retry` (which is the default value), Koperator re-executes the failed task every `30s` until it succeeds. During the re-execution the `cruiseControlState` returns to `GracefulUpscaleRunning`.

    ```yaml
    status:
    ...
      brokersState:
        "3":
         ...
          gracefulActionState:
            cruiseControlOperationReference:
              name: kafka-addbroker-mhh72
            cruiseControlState: GracefulUpscaleCompletedWithError
        ...
    ```

## CruiseControlOperation CR overview

The kafka-addbroker-mhh72 `CruiseControlOperation` custom resource from the previous example looks like:

```yaml
kind: CruiseControlOperation
metadata:
...
  name: kafka-addbroker-mhh72
...
spec:
...
status:
  currentTask:
    finished: "2022-11-18T09:31:40Z"
    httpRequest: http://kafka-cruisecontrol-svc.kafka.svc.cluster.local:8090/kafkacruisecontrol/add_broker?allow_capacity_estimation=true&brokerid=3&data_from=VALID_WINDOWS&dryrun=false&exclude_recently_demoted_brokers=true&exclude_recently_removed_brokers=true&json=true&use_ready_default_goals=true
    httpResponseCode: 200
    id: 222e30f0-1e7a-4c87-901c-bed2854d69b7
    operation: add_broker
    parameters:
      brokerid: "3"
      exclude_recently_demoted_brokers: "true"
      exclude_recently_removed_brokers: "true"
    started: "2022-11-18T09:30:48Z"
    state: Completed
    summary:
      Data to move: "0"
      Intra broker data to move: "0"
      Number of intra broker replica movements: "0"
      Number of leader movements: "0"
      Number of replica movements: "36"
      Provision recommendation: '[ReplicaDistributionGoal] Remove at least 4 brokers.'
      Recent windows: "1" 
  errorPolicy: retry
  retryCount: 0
```

- The `status.currentTask` describes the Cruise Control task.  
- The `httpRequest` field contains the whole POST HTTP request that has been executed.  
- The `id` is the Cruise Control task identifier number.  
- The `state` shows the progress of the request.  
- The `summary` is Cruise Control's optimization proposal. It shows the scope of the changes that Cruise Control will apply through the operation.  
- The `retryCount` field shows the number of retries when a task has failed and `cruiseControlOperation.spec.errorPolicy` is set to `retry`. In this case, the `status.failedTask` field shows the history of the failed tasks (including their error messages).  
For further information on the fields, see the [source code](https://github.com/adobe/koperator/blob/master/api/v1alpha1/cruisecontroloperation_types.go).

## Control the created CruiseControlOperation {#control-cruisecontroloperation}

### Stop a task

The task execution can be stopped gracefully when the `CruiseControlOperation` is deleted. In this case the corresponding `cruiseControlState` or the `cruiseControlVolumeState` will transition to `Graceful*Succeeded`.

### Handle failed tasks

`cruiseControlOperation.spec.errorPolicy` defines how the failed Cruise Control task should be handled. When the `errorPolicy` is set to `retry`, Koperator re-executes the failed task every 30 seconds. When it is set to `ignore`, Koperator treats the failed task as completed, thus the `cruiseControlState` or the `cruiseControlVolumeState` transitions to `Graceful*Succeeded`.

### Pause a task

When there is a Cruise Control task which can not be completed without an error and the `cruiseControlOperation.spec.errorPolicy` is set to `retry`, Koperator will re-execute the task until it succeeds. You can pause automatic re-execution by adding the following label on the corresponding `CruiseControlOperation` custom resource. For details see [this example](#example-pause). To continue the task, remove the label (or set to any other value than `true`).

Pausing is useful when the reason of the error can not be fixed any time soon but you want to retry the operation later when the problem is resolved.

A paused `CruiseControlOperation` tasks are ignored when selecting operations for execution: when a new `CruiseControlOperation` with the same operation type (`status.currentTask.operation`) is created, the new one is executed and the paused one is skipped.

```yaml
kind: CruiseControlOperation
metadata:
...
  name: kafka-addbroker-mhh72
  labels:
    pause: "true"
...
```

### Automatic cleanup

You can set automatic cleanup time for the created `CruiseControlOperations` in the `KafkaCluster` custom resource.  
In the following example, the finished (completed successfully or `completedWithError` and `errorPolicy: ignore`) `CruiseControlOperation` custom resources are automatically deleted after 300 seconds.

```yaml
apiVersion: kafka.banzaicloud.io/v1beta1
kind: KafkaCluster
...
spec:
...
  cruiseControlConfig:
    cruiseControlOperationSpec:
      ttlSecondsAfterFinished: 300
...
```

### Example for the ignore and pause use-cases {#example-pause}

This example shows how to ignore and pause an operation.

1. Using the [original example with four Kafka brokers from the Overview](#overview) as the starting point, this example removes two brokers at the same time by editing the `KafkaCluster` custom resource and deleting broker 2 and broker 3.

    ```yaml
     Spec:
     ...  
       brokers:
        - id: 0
          brokerConfigGroup: "default"
        - id: 1
          brokerConfigGroup: "default"
    ```

1. The brokers (`kafka-removebroker-lg7qm`, `kafka-removebroker-4plfq`) will have separate `remove_broker` operations. The example shows that the first one is already in running state.

    ```yaml
    status:
    ...
      brokersState:
        "2":
         ...
          gracefulActionState:
            cruiseControlOperationReference:
              name: kafka-removebroker-lg7qm
            cruiseControlState: GracefulDownscaleRunning
         ...
         "3":
         gracefulActionState:
            cruiseControlOperationReference:
              name: kafka-removebroker-4plfq
            cruiseControlState: GracefulDownscaleScheduled
        ...
    ```

1. Assume that something unexpected happened, so the `remove_broker` operation enters the `GracefulDownscaleCompletedWithError` state.

    ```yaml
    status:
    ...
      brokersState:
        "2":
         ...
          gracefulActionState:
            cruiseControlOperationReference:
              name: kafka-removebroker-lg7qm
            cruiseControlState: GracefulDownscaleCompletedWithError
         ...
         "3":
         gracefulActionState:
            cruiseControlOperationReference:
              name: kafka-removebroker-4plfq
            cruiseControlState: GracefulDownscaleScheduled
        ...
    ```

1. At this point, you can decide how to handle this problem using one of the three possible options: retry it (which is the default behavior), ignore the error, or use the `pause` label to pause the operation and let Koperator execute the next operation.

    - Ignore use-case: To ignore the error, set the `cruiseControlOperation.spec.errorPolicy` field to `ignore`. The operation will be considered as a successful operation, and the broker pod and the persistent volume will be removed from the Kubernetes cluster and from the `KafkaCluster` status. Koperator will continue to execute the next task: `remove_broker` for  `kafka-removebroker-4plfq`.

        ```yaml
        status:
        ...
          brokersState:
            ...
             "3":
             gracefulActionState:
                cruiseControlOperationReference:
                  name: kafka-removebroker-4plfq
                cruiseControlState: GracefulDownscaleRunning
            ...
        ```

    - Pause use-case: To pause this task, add the `pause: true` label to the `kafka-removebroker-lg7qm` `CruiseControlOperation`. Koperator won't try to re-execute this task, and moves on to the next `remove_broker` operation.

        ```yaml
        status:
        ...
          brokersState:
            "2":
             ...
              gracefulActionState:
                cruiseControlOperationReference:
                  name: kafka-removebroker-lg7qm
                cruiseControlState: GracefulDownscalePaused
             ...
             "3":
             gracefulActionState:
                cruiseControlOperationReference:
                  name: kafka-removebroker-4plfq
                cruiseControlState: GracefulDownscaleRunning
            ...
        ```

        When the second `remove_broker` operation is finished, only the paused task remains:

        ```yaml
        status:
        ...
          brokersState:
            "2":
             ...
              gracefulActionState:
                cruiseControlOperationReference:
                  name: kafka-removebroker-lg7qm
                cruiseControlState: GracefulDownscalePaused
             ...
        ```

        When the problem has been resolved, you can retry removing broker 2 by removing the `pause` label.

        ```yaml
        status:
        ...
          brokersState:
            "2":
             ...
              gracefulActionState:
                cruiseControlOperationReference:
                  name: kafka-removebroker-lg7qm
                cruiseControlState: GracefulDownscaleRunning
             ...
        ```

        If everything goes well, the broker is removed.
