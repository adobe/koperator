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
