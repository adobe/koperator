# Dedicated KRaft metadata storage for broker-only nodes

`brokerConfig.metadataStorage` is **opt-in**. Omit it to preserve the existing
storage and startup behavior. It is supported only for KRaft nodes with exactly
`processRoles: [broker]`, not controllers, combined nodes, ZooKeeper nodes, or
brokers still using ZooKeeper during a ZK-to-KRaft migration. Brokers whose
ZK-to-KRaft migration is finalized are supported: their data directories keep
version 0 `meta.properties` (`broker.id`) and usually have no
`bootstrap.checkpoint`, which the migration accepts.

The setting uses the existing `StorageConfig` shape, but requires `pvcSpec` and
disallows `emptyDir` and block-mode PVCs. Every data `storageConfigs` entry of
the broker must be PVC-backed as well: an `emptyDir` data directory is lost on
pod replacement, so it cannot be a migration source, and durable metadata gains
nothing beside ephemeral replicas. It creates a separate `<cluster>-<node-id>-metadata` PVC,
mounts it at `mountPath`, and generates
`metadata.log.dir=<mountPath>/kafka`. Because the broker ConfigMap is live-mounted,
the operator adds `metadata.log.dir` only once no broker pod without the metadata
volume remains, and never creates a metadata-storage pod whose ConfigMap lacks it;
once published the setting is kept. It **does not** add that path to `log.dirs`,
Cruise Control capacity/add/remove/rebalance operations, or topic storage
assignments. `storageConfigs` remain topic-data disks.

Group inheritance works like other broker configuration, with one distinction:
a broker-local `metadataStorage` replaces the entire group metadata storage
object; it does not merge partial PVC specs. Omitting a local value inherits the
group value. Do not put this setting in a group used by controllers.

## Opt-in configuration

Enable it once in the broker-only config group; keep broker IDs, roles and all
data storage unchanged. The operator migrates the brokers itself, one at a time
(see [Automatic rollout](#automatic-rollout)):

```yaml
spec:
  kRaft: true
  brokerConfigGroups:
    # Keep every existing field of the group used only by broker-only nodes.
    broker:
      processRoles:
        - broker
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

To pilot on a single broker first, put the same `metadataStorage` object in that
broker's `brokerConfig` instead, then move it to the group once satisfied.

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

These are checked once, before enabling the flag; the operator automates the
per-broker steps.

1. Install the updated operator, admission webhook and both the installed CRD
   and chart CRD before opting in. Verify that admission is enabled and working.
   Helm does not automatically upgrade existing CRDs.
2. Use the broker's existing Kafka **3.9 or newer** image with the static-quorum
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
   directory. Validate its `meta.properties` cluster ID and node ID (`node.id`
   for version 1, `broker.id` for version 0 or unversioned files). The source
   `bootstrap.checkpoint` is copied when present; brokers migrated from
   ZooKeeper normally have none, and the destination then has none either. Take CSI snapshots/backups using your existing
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

## Automatic rollout

Enabling `metadataStorage` is the only required change. The operator:

1. Creates every opted-in broker's metadata PVC up front.
2. Replaces at most **one** broker pod per cluster for migration at a time, using
   graceful deletion and the normal rolling-upgrade gates. Before each
   migration restart it additionally requires that no other opted-in broker's
   migration is in progress (old pod terminating/missing, or replacement not
   yet `Ready`) and that no **other** broker has offline or out-of-sync replicas.
   This applies regardless of `concurrentBrokerRestartCountPerRack`, and also to
   crashed pods that would otherwise bypass the rolling-upgrade gates.
3. Runs the `migrate-broker-metadata` init container in the replacement pod,
   which copies the stopped broker's metadata onto the new PVC before Kafka
   starts. A failure keeps the pod in its init container and halts the rollout;
   nothing else is migrated until it is resolved.
4. Records `status.brokersState["<id>"].metadataStorageState: Ready` once the
   replacement pod is Ready, then waits for in-sync replicas to recover before
   migrating the next broker.

Monitor progress:

```sh
kubectl -n "$NAMESPACE" get kafkacluster "$CLUSTER" \
  -o jsonpath='{range .status.brokersState.*}{.metadataStorageState}{"\n"}{end}'
kubectl -n "$NAMESPACE" get pods,pvc -l "kafka_cr=$CLUSTER"
kubectl -n "$NAMESPACE" logs "$POD" -c migrate-broker-metadata
```

The init container logs each phase (validation, format, copy, publish, source
retirement, completion), so a slow copy is distinguishable from a hung one.
Success includes `KRaft metadata storage ready for broker <id>`. A failure
blocks Kafka startup; diagnose it without deleting source metadata,
destination ownership markers or backups.

Readiness is recorded durably as an annotation on the metadata PVC and projected
to the status. Admission rejects removing any data disk of a broker with
`metadataStorage` until that state is `Ready`, so a data-removal update cannot
race the migration. If admission was bypassed, reconciliation keeps every data
PVC and stops with an error naming the disk to restore. After every broker is
`Ready`, a later, separate change may remove old **data** disks through the
existing Cruise Control drain workflow. Back up retained metadata before
deleting a source PVC.

The gates check broker registration and replica health, not client error rates
or capacity headroom; keep watching those during the rollout.

### What the rollout gates do (and do not do)

The normal rolling-upgrade path waits on missing/terminating/pending pods and
rejects further restarts when it observes offline/out-of-sync replicas beyond
`failureThreshold`. Migration restarts additionally pass the dedicated
one-per-cluster migration gate described above, including on the
terminated-container repair path. The unschedulable-pod repair path (a pod
pinned to a removed PVC) is not gated, because waiting cannot fix it.
Broker-only pods do not gain a new metadata-readiness probe in this change.

No setting guarantees full ISR, spare capacity, client availability, or zero
transient client errors under all failure scenarios. Leader election/retries
can cause transient errors. Broker-only restarts do not use the controller
quorum-readiness gate, so check controller quorum health before enabling the flag.

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
  Only the complete stopped-source `__cluster_metadata-0` and, when present,
  `bootstrap.checkpoint` are copied. Without a source bootstrap, the
  formatter's bootstrap is removed so the destination mirrors the source. Source `meta.properties` and topic replicas
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

## Broker removal

Removing a broker from `spec.brokers` uses the existing graceful downscale. When
the broker pod is deleted, the operator deletes its data PVCs **and** its
metadata PVC, so the broker ID can later be reused with or without metadata
storage. Back up the metadata PVC first if your recovery policy requires it.

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
