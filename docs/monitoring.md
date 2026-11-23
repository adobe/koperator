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

During deletion the operator keeps polling the original task ID. A stop
acknowledgement does not count as completion of that task. A rejected stop
records a failure without the request URL or raw error text and leaves the
finalizer in place. If a later poll omits the original task ID, the operator
treats the missing result as a failure and can finish deletion without confirming
the task's outcome. Do not delete a broker-drain operation to confirm that its
brokers are safe to remove; verify the drain in Cruise Control first.

This is not exactly-once execution. A later reconcile can resubmit unrecorded
work when Cruise Control is idle. Finalization stops can be repeated while the
original task is still reported running. Check Cruise Control's task history
and use the task ID from the persistence error log when available.
