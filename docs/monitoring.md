The documentation of the Koperator project is available at the [Banzai Cloud Documentation Page](https://banzaicloud.com/docs/supertubes/kafka-operator/monitoring/).

## Kubernetes API client metrics

The operator exposes controller-runtime's opt-in REST-client metrics on its
existing metrics endpoint:

- `rest_client_request_duration_seconds`
- `rest_client_dns_resolution_duration_seconds`
- `rest_client_request_size_bytes`
- `rest_client_response_size_bytes`
- `rest_client_rate_limiter_duration_seconds`
- `rest_client_request_retries_total`

The existing `rest_client_requests_total` counter remains enabled. Families
appear after the corresponding operation occurs; DNS and retry metrics may be
absent until a lookup or retry is needed. No resource names, UIDs, or error
messages are added as labels.

Before changing cache, polling, apply, or concurrency behavior, compare API
request rates, latency, throttling time, retries, controller queue depth,
reconcile duration, and operator memory under the same workload. For example:

```promql
sum by (method, code) (rate(rest_client_requests_total[5m]))
```

```promql
histogram_quantile(0.99,
  sum by (verb, le) (rate(rest_client_request_duration_seconds_bucket[5m])))
```

```promql
sum by (verb) (rate(rest_client_rate_limiter_duration_seconds_sum[5m]))
```

Record cluster size, resource counts, workload, test duration, leader changes,
and error rates with each measurement. These metrics establish instrumentation,
not evidence that an optimization improves performance. This change does not
alter metrics authentication, scraper routing, worker counts, or cache scope.

## Cruise Control execution-result persistence

The operator retries conflicting execution-result status writes with a fresh
API-server read, without resubmitting the request. A failed refresh or a change
to the submitted task or its target cluster stops the retry. This does not
apply to `/state` health probes.

When a stop request fails, the operator keeps the running task as the current
task, appends the failed stop to `status.failedTasks`, and retries the stop
later. The finalizer stays until the operator knows the running task's state.

This is not exactly-once execution. If a submission goes unrecorded, a later
reconciliation can resubmit it once Cruise Control is idle. Check Cruise
Control's task history and use the task ID from the persistence error log when
available.

## Cleanup ownership during provisioning

KafkaCluster, managed KafkaTopic, and KafkaUser resources acquire cleanup
finalizers before provisioning. A failed finalizer write prevents protected
side effects. Reconciliation continues after a successful write rather than
depending on a finalizer-only watch event.

KafkaUser records its ACL cleanup identity in `status.aclPrincipal` after the
Kafka connection opens and before the first ACL is created. A nonempty value is
the principal used for its ACLs; an empty value records that ACL submission has
not started. An absent value is legacy/unknown state, not evidence that no ACLs
exist. The principal depends on the user type:

- Certificate users (`spec.createCert` unset or `true`) use the subject of the
  issued certificate, for example `CN=alice,O=example`. The operator reads it
  only from a Secret that the KafkaUser controls.
- Users with `spec.createCert: false` use `CN=<KafkaUser name>`. Deletion
  always revokes that principal and never needs a certificate.

At deletion the operator revokes the recorded principal and every principal in
`status.acls`, which lists the ACLs the operator last created. An empty record
skips Kafka only when `status.acls` is also empty. Both fields live in the
status subresource, so edits to KafkaUser metadata or spec cannot change which
ACLs are deleted or skip their cleanup; annotations are never trusted for it.
Grant `kafkausers/status` write access only to the operator. Deletion removes
exactly the bindings the operator creates for each grant and keeps other
bindings of the same principal. KafkaUser reads
through the shared informer cache. Reads that decide cleanup from a Secret or
Certificate being absent, such as recording the identity before provisioning,
resolving a legacy identity, and cert-manager cleanup, go directly to the API
server so a stale cache cannot cause them.

Deletion does not create certificates to discover a user's principal.
Recorded principals survive a failed final status write or an incomplete Secret
cache. Legacy certificate users without a recorded principal or `status.acls`
still require their existing certificate, in a Secret the user controls, for
ACL cleanup; the operator records that principal before PKI cleanup. If the
certificate is missing, invalid, or in a Secret the user does not control,
deletion stays blocked; see
[Recovering a blocked KafkaUser deletion](#recovering-a-blocked-kafkauser-deletion).
The record does not recover external work whose identity was never recorded by
an older operator.

Rolling back to an older operator stops recording identities. An older
operator that writes status replaces it and drops `status.aclPrincipal`, so the
newer operator treats that user as legacy again. An older operator can also
create ACLs and fail before writing status, which leaves an earlier empty
record in place. Before relying on deletion to revoke access for users that an
older version reconciled after the identity was recorded, compare their Kafka
ACLs with `status.aclPrincipal` and `status.acls`.

A managed KafkaTopic deletes its Kafka topic only when its status shows that the
operator created or adopted it: `creating` is recorded before a creation request
and `created` after creation or adoption. Without that state, deleting the
KafkaTopic releases the finalizer without contacting Kafka and leaves any
same-named Kafka topic in place. After Kafka accepts a deletion, the state is
`deleting` and the operator waits for the name to disappear without asking
again, so a topic recreated under the same name is never deleted. If the name
stays present, the topic may have been recreated; an administrator decides
whether to delete that topic in Kafka, after which the operator releases the
finalizer itself.

For cert-manager users, cleanup stops the owned Certificate before deleting
user-owned Secrets. Regenerated Secrets without owner references are adopted
for cleanup only when they identify that owned Certificate. Cleanup follows the
Secret the Certificate actually writes, even if `spec.secretName` changed later,
and checks again after the Certificate is gone for a Secret an in-flight
issuance published meanwhile. A Secret cert-manager writes after cleanup
finishes cannot be attributed safely and is not deleted; run cert-manager with
`--enable-certificate-owner-ref` so the Certificate owns its Secret and Kubernetes
garbage collection removes it. Shared or
externally owned Secrets are preserved. Cleanup retries while the Certificate
is still deleting; it does not wait synchronously for issuance or approval.

### Recovering a blocked KafkaUser deletion

A certificate KafkaUser that stays in `Terminating` with the operator log
message `KafkaUser deletion blocked` has no recorded ACL cleanup identity or
`status.acls`, and the operator cannot read one from its certificate Secret.
Users with `spec.createCert: false` never need this procedure. The log entry names that
Secret. Confirm the state:

```bash
kubectl get kafkauser <name> -n <namespace> -o jsonpath='{.status.aclPrincipal}'
```

An empty output with the blocked message means the identity is unknown. Recover
it in one of these ways, then let the operator retry. It deletes the ACLs and
removes the finalizer itself.

1. Restore the certificate Secret named by `spec.secretName`, for example from
   a backup, with the original `tls.crt` and its controller reference to the
   KafkaUser. The operator reads the principal from the certificate subject.
   This is not possible in a terminating namespace; use the next option there. For cert-manager users whose Certificate still
   exists, cert-manager can reissue the Secret with the same subject; wait for
   it instead of creating one by hand.
2. If the certificate cannot be restored, a cluster administrator who can write
   `kafkausers/status` records the principal after verifying it in Kafka. List
   the cluster ACLs and find the entries for this user's topics. Kafka shows
   the principal as `User:<DN>`; record only the `<DN>` part, exactly as shown:

   ```bash
   kubectl patch kafkauser <name> -n <namespace> --subresource=status \
     --type=merge -p '{"status":{"aclPrincipal":"<DN>"}}'
   ```

Do not use these to unblock deletion:

- Do not remove the finalizer. That leaves the user's ACLs in Kafka.
- Do not set `status.aclPrincipal` to an empty value. Empty means no ACLs were
  ever submitted, so the operator removes the finalizer without deleting any.
- Do not add or edit annotations such as `kafka.banzaicloud.io/acl-principal`.
  The operator ignores KafkaUser metadata when it chooses which ACLs to delete.
- Do not record a principal you have not verified. The operator deletes the
  topic ACLs that match the recorded principal.
