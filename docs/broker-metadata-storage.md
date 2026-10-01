# Dedicated KRaft metadata storage for broker-only nodes

`brokerConfig.metadataStorage` is **opt-in**. Omit it to preserve the existing
storage and startup behavior. It is supported only for KRaft nodes with exactly
`processRoles: [broker]`, not controllers, combined nodes, ZooKeeper nodes, or
brokers still using ZooKeeper during a ZK-to-KRaft migration.

The setting uses the existing `StorageConfig` shape, but requires `pvcSpec` and
disallows `emptyDir` and block-mode PVCs. It creates a separate `<cluster>-<node-id>-metadata` PVC,
mounts it at `mountPath`, and generates
`metadata.log.dir=<mountPath>/kafka`. It **does not** add that path to `log.dirs`,
Cruise Control capacity/add/remove/rebalance operations, or topic storage
assignments. `storageConfigs` remain topic-data disks.

Group inheritance works like other broker configuration, with one distinction:
a broker-local `metadataStorage` replaces the entire group metadata storage
object; it does not merge partial PVC specs. Omitting a local value inherits the
group value. Do not put this setting in a group used by controllers.

## Opt-in configuration

For an existing broker, add **only** this field to its existing configuration;
keep its broker ID, role, config group and all data storage unchanged:

```yaml
spec:
  kRaft: true
  rollingUpgradeConfig:
    failureThreshold: 1
    concurrentBrokerRestartCountPerRack: 1
  brokers:
    # Keep every existing broker/controller entry; this is the selected broker.
    - id: 100
      brokerConfigGroup: broker
      brokerConfig:
        metadataStorage:
          mountPath: /csi-kafka-metadata
          pvcSpec:
            accessModes:
              - ReadWriteOnce
            storageClassName: YOUR_EXISTING_CSI_STORAGE_CLASS
            resources:
              requests:
                storage: 20Gi
```

The storage class, size and availability-zone topology are examples, not sizing
recommendations. The metadata mount must not equal, contain, or be contained by
any data/custom/system mount. It must be a clean absolute path. For **new**
clusters, the same object can be placed directly in the broker-only config
group. See the optional example in
[`simplekafkacluster_kraft.yaml`](../config/samples/kraft/simplekafkacluster_kraft.yaml).
The operator generates this property's value, overriding a previous
`metadata.log.dir` pin when dedicated storage is enabled. KafkaCluster-producing
Helm charts must explicitly emit `metadataStorage`; adding it only to chart
values or to `additionalVolumes` does not enable this API feature.

Once enabled, admission and reconciliation reject removal, mount relocation
and PVC-spec replacement. Storage requests may grow when the storage class
supports expansion, but may not shrink. Reverse migration is **not supported**.
Deleting a PVC, changing its purpose/identity annotations, downgrading the
operator, or bypassing admission is not a rollback procedure.

## Preconditions for a live migration

1. Install the updated operator, admission webhook and both the installed CRD
   and chart CRD before opting in. Verify that admission is enabled and working.
   Helm does not automatically upgrade existing CRDs.
2. Use the broker's existing Kafka **3.7 or newer** image with the static-quorum
   formatter used by the existing operator. The image must provide Bash, Kafka's
   `kafka-storage.sh`, `cp -a`, `mv`, `cmp`, and `sync`. Formatting must produce a
   valid unique `directory.id`. Rehearse against that exact image and CSI driver
   in a non-production cluster before a production rollout. Do not combine this
   rollout with a Kafka/metadata-version upgrade.
3. Confirm a healthy controller quorum and healthy broker registration. This
   feature does not migrate, wipe, format, or alter **any controller** metadata,
   directory, PVC, or recovery mechanism.
4. For RF3 across three AZs, verify that **every affected partition**, including
   internal topics, has three healthy in-sync replicas across the expected AZs.
   Verify `min.insync.replicas` and producer acknowledgements; RF3 alone is not
   enough. Two remaining replicas must have headroom for the unavailable
   broker's leaders/traffic. A topic with `min.insync.replicas=3` cannot maintain
   acknowledged writes while one RF3 replica is offline.
5. Pause unrelated upgrades, data-disk drains, reassignments, scaling and
   disruptive node maintenance. Preserve the source data disk until migration,
   restart and full ISR recovery are verified. Admission rejects changing data
   `storageConfigs` in the same update that enables metadata storage.
6. Verify that all source data PVCs are mounted and readable, and that exactly
   one contains `<data-mount>/kafka/__cluster_metadata-0`. The source may be
   `/csi-kafka-logs1/kafka`, `/csi-kafka-logs2/kafka`, or another mounted data
   directory. Validate its `meta.properties` node/cluster IDs and the source
   `bootstrap.checkpoint`. Take CSI snapshots/backups using your existing
   application-consistent procedure. Do not copy or move metadata while Kafka
   is running.
7. Size the metadata PVC for the current complete metadata log, snapshots,
   bootstrap checkpoint and future growth. Initial staging uses one payload
   copy on that PVC; the source backup is a rename on the old data volume.
   Ensure the new PVC can bind/schedule in the broker's AZ and is writable using
   the broker's existing security context. Check the configured termination
   grace period accommodates shutdown.
   The migration init container uses the broker's resource requirements (not the
   tiny reporter-copy init limit), with a 256 MiB maximum Java heap for formatting;
   allow JVM/native overhead as well.

## One broker at a time

Use a **broker-local** setting for the live rollout. Updating the shared group
would opt in every broker at once; do not rely on concurrency settings alone as
a substitute for staged observation.

1. Record the selected broker's old pod UID/name, PVCs, source path and IDs.
   Check all preconditions above.
2. Apply the selected broker's `metadataStorage` field without changing its
   data storage or any controller configuration. Do not replace the whole
   brokers list with the illustrative YAML fragment.
3. The operator creates the dedicated PVC and changes that broker's desired
   pod/config. Normal rolling-upgrade gates select a restart, delete the old
   pod **gracefully**, and create a replacement only after no matching old pod
   remains. The `migrate-broker-metadata` init container runs before Kafka.
   Monitor the old pod's termination and confirm it is not still running.
4. Monitor the init container and PVC:

   ```sh
   kubectl -n "$NAMESPACE" get pods,pvc -l "kafka_cr=$CLUSTER,brokerId=$BROKER_ID"
   kubectl -n "$NAMESPACE" logs "$NEW_POD" -c migrate-broker-metadata
   kubectl -n "$NAMESPACE" logs "$NEW_POD" -c kafka
   ```

   Success includes `KRaft metadata storage ready for broker <id>`. Failure
   blocks Kafka startup; diagnose it before proceeding, without deleting source
   metadata, destination ownership markers or backups.
5. Verify the new ConfigMap has metadata storage only in `metadata.log.dir`,
   and topic volumes only in `log.dirs`. Verify destination identities and
   migration completion marker, the retained source backup, and absence of
   `__cluster_metadata-0` from **all** mounted data `kafka` roots. Do not modify
   live metadata during verification.
6. Verify broker registration, actual partition ISR restoration, no offline/
   under-replicated partitions, controller-quorum health, client error rates and
   disk capacity. Pod `Ready` or the init success message alone is insufficient.
   Keep the source disk until recovery is proven. Optionally rehearse a second
   normal restart of this broker and repeat recovery checks.
   Reconciliation retains every existing data PVC until it observes a Ready
   replacement pod using the metadata PVC with a successful migration init
   container. It persists that observation on the metadata PVC. This protects
   against a subsequent data-removal update racing the initial migration;
   it does not replace the full ISR and client-health checks above.
7. Only then opt in the next broker, repeating the same checks. After every
   broker has recovered, a later, separate change may remove old **data** disks
   through the existing Cruise Control drain workflow. Back up retained metadata
   before deleting the source PVC.

### What the existing rollout gates do (and do not do)

With AUS5-style `failureThreshold: 1` and
`concurrentBrokerRestartCountPerRack: 1`, the existing normal path waits on
missing/terminating/pending pods and rejects further normal restarts when it
observes a broker with offline/out-of-sync replicas (or the recorded error
threshold). This is not a new global maintenance coordinator. The existing
terminated-container/unschedulable-pod repair paths have different gates.
Broker-only pods do not gain a new metadata-readiness probe in this change.

No setting guarantees full ISR, spare capacity, client availability, or zero
transient client errors under all failure scenarios. Leader election/retries
can cause transient errors. Broker-only restarts do not use the controller
quorum-readiness gate, so explicitly check controller quorum before each step.

**Fencing limitation:** ordinary graceful Kubernetes pod deletion waits for
container shutdown, which is the offline-copy precondition. RWO is **not**
process fencing. Do not force-delete the old pod, reduce its grace period to
zero, manually create a replacement, or perform migration while a node is
partitioned/unreachable and its old Kafka process may still run. The operator
does not provide external STONITH/storage fencing. Resolve/fence that node and
prove the old process is stopped before continuing.

## Durable phases and failure handling

The init container validates broker-only role, generated config and all mounted
data-directory identities. A fresh broker is allowed only when the metadata PVC
was created without existing pods, data PVCs, or a known running broker version, and no
formatted data directory or source metadata is present. An existing broker with
missing or multiple sources fails closed rather than rebuilding an empty cache.

On the metadata PVC:

* `.koperator-metadata-migration` records immutable node ID, cluster ID and source
  mount (or `fresh`) before mutation.
* `.koperator-metadata-stage` is formatted by Kafka with a **new** directory ID.
  Only the complete stopped-source `__cluster_metadata-0` and
  `bootstrap.checkpoint` are copied. Source `meta.properties` and topic replicas
  are never copied/reformatted. Partial copies can be retried from the intact
  stopped source.
* The owned staging directory is synced and atomically renamed to `kafka`.
  `.koperator-metadata-owner` identifies that publication. Unknown destination/
  staging content or mismatched identities causes an error.
* The source `__cluster_metadata-0` is atomically **moved** to
  `<source-mount>/.koperator-metadata-backup-<node-id>`, outside `log.dirs`.
  Source topic directories, `meta.properties`, and bootstrap remain untouched.
* `.koperator-metadata-complete` is written after backup. An interruption after
  publishing or backup resumes using the owned destination and retained backup.
  A completed restart uses the destination and never resets its quorum cache.
  Once complete, removing the old source disk is supported.

Do not manually erase markers to bypass an error. Correct storage permissions,
identity/config conflicts, capacity or image/tool issues, then retry through
normal operator replacement. If source/destination evidence disagrees, stop the
rollout and use your incident/recovery process. Never restore the old active
metadata into a live data `kafka` root alongside the new destination.

The original source backup is **not** a current replica after Kafka starts on
the destination. Keep it and the original data `meta.properties`/bootstrap for
your agreed recovery-retention window; preserve a CSI snapshot before the data
PVC is drained/deleted. Nothing automatically prunes backups. There is no safe
automatic reverse migration, operator downgrade, or failback to this stale
backup. Recovery after destination loss requires a separately reviewed, offline
broker recovery procedure. Controller disaster recovery remains unchanged.

## Kafka 3.9.2 migration smoke test

The shell-fixture unit tests exercise failure/retry phases. A separate smoke
test starts a real, isolated controller and broker, writes a topic record,
migrates from each of `logs1` and `logs2`, and verifies the record and metadata
quorum after two broker restarts:

```sh
docker run --rm --network none --entrypoint /bin/bash \
  -v "$PWD/scripts/test-broker-metadata-migration.sh:/test/smoke.sh:ro" \
  -v "$PWD/pkg/resources/kafka/migrate-broker-metadata.sh:/test/migrate-broker-metadata.sh:ro" \
  apache/kafka:3.9.2 /test/smoke.sh
```

This checks real Kafka formatting and metadata recovery, not RF3/AZ disruption
gates, Kubernetes pod fencing, or production CSI behavior. Those still require
the staged deployment rehearsal and recovery checks described above.
