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