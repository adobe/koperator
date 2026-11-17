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

After an external task is submitted, the operator retries conflicting status
writes using fresh API-server reads. These persistence retries do not submit
the external task again. They retain the task identity, response metadata,
retry history, and selected stop-operation type. A replaced operation, removed
current task, or different task identity produces an error rather than accepting
a stale result.

This is not exactly-once execution. If all writes fail, or the process stops
after Cruise Control accepts a request but before its task ID is recorded, the
external task can outlive the recorded operation state. Recorded task IDs allow
subsequent reconciliation to query task progress. Automatic adoption of an
unrecorded submission is not implemented; inspect Cruise Control's task history
when investigating this window, and do not infer that a missing status means
the external operation never ran.

## Cleanup ownership during provisioning

KafkaCluster, managed KafkaTopic, and KafkaUser resources acquire cleanup
finalizers before provisioning. A failed finalizer write prevents protected
side effects. Reconciliation continues after a successful write rather than
depending on a finalizer-only watch event.

KafkaUser records `kafka.banzaicloud.io/acl-principal` before attempting ACL
creation. A nonempty value is the certificate-derived cleanup identity; an
empty value records that ACL submission has not started. An absent annotation
is legacy/unknown state, not evidence that no ACLs exist. Do not remove or edit
this annotation. KafkaUser uses an uncached Kubernetes client so certificate
and Secret absence used during initialization and cleanup is authoritative.
This adds direct reads; it is a correctness change, not an API-traffic
optimization.

Deletion does not create certificates to discover a user's principal.
Recorded principals survive a failed status write or an incomplete Secret
cache. Legacy users without a recorded principal still require their existing
certificate for ACL cleanup; if it is missing or invalid, restore the cleanup
identity rather than removing the finalizer and abandoning ACLs. The annotation
does not recover external work whose identity was never recorded by an older
operator, and rolling back to an older operator stops recording these markers.

For cert-manager users, cleanup stops the owned Certificate before deleting
user-owned Secrets. Regenerated Secrets without owner references are adopted
for cleanup only when they identify that owned Certificate. Shared or
externally owned Secrets are preserved. Cleanup retries while the Certificate
is still deleting; it does not wait synchronously for issuance or approval.