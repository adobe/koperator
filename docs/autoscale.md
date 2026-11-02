<!--
Copyright 2026 Adobe. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
-->

# Broker autoscaling

Koperator can add or remove a data broker in response to an alert delivered to
its action receiver. Prometheus evaluates your policy, Alertmanager routes the
action alert, and Koperator updates the KafkaCluster's desired broker list.
Reconciliation and Cruise Control perform the resource and replica changes.

**Broker autoscaling is opt-in.** The
[monitoring sample](../config/samples/kafkacluster-prometheus.yaml) contains
diagnostic broker alerts without scaling commands or broker-action routes.
These sample defaults do not disable existing custom action alerts, direct
webhook integrations, or manual changes to `spec.brokers`.

Set up metrics and human notifications using the
[monitoring user guide](monitoring.md) before enabling broker actions.
Keep the Helm setting `alertManager.enable` enabled and ensure Alertmanager can
reach Koperator's webhook Service. Cruise Control must be configured and healthy.

## Prepare a scaling policy

Use a policy validated against your workload and deployment. The sample
request-rate and partition-count thresholds are not capacity recommendations.

Before enabling actions:

1. Measure idle and representative load behavior per broker: CPU, memory, disk,
   network, request latency, replication health, and partition placement.
2. Verify that adding a broker produces useful replica redistribution with
   your Cruise Control goals and load model.
3. Before scale-in, verify that the remaining data brokers can sustain the
   workload and preserve replication factor, minimum ISR, and rack placement.
4. Define a post-operation stabilization interval and prevent opposing
   scale-out/scale-in requests during it.
5. Test the policy and delivery path in a disposable cluster before production.
   Include missing metrics, broker startup, skew, and overlapping signals.

Koperator does not provide a timed cooldown. An alert's `for` duration measures
how long its expression remains true, not how long it has been since the last
operation. Enforce stabilization in your action policy or external coordinator;
keep scale-in manually approved until that control is in place.

## Configure broker-count limits

Set positive limits in the existing KafkaCluster manifest. This example allows
three to four data brokers alongside three dedicated KRaft controllers; it is
not a capacity recommendation:

```yaml
spec:
  alertManagerConfig:
    downScaleLimit: 6
    upScaleLimit: 7
```

Scale-in is skipped when the number of entries in `spec.brokers` is at or below
`downScaleLimit`. Scale-out is skipped when that count is at or above
`upScaleLimit`. Omitted or non-positive values do not provide an effective
bound.

The count includes dedicated KRaft controllers. These limits apply to
alert-triggered scaling, not manual edits of the broker list. Count limits alone
do not establish replication, placement, or resource safety.

## Define separate action alerts

Keep diagnostic alerts unchanged. Create separately named action alerts with
workload-specific conditions or an approval mechanism.

An action alert must contain `namespace` and `kafka_cr` labels and a supported
`command` annotation. For example, an approved scale-out alert delivered to
Koperator can carry:

```yaml
labels:
  alertname: KafkaScaleOutApproved
  namespace: kafka
  kafka_cr: kafka
  autoscaling: approved
annotations:
  command: upScale
  brokerConfigGroup: broker
```

This is an alert payload fragment, not a complete PrometheusRule. Prometheus
sets `alertname` from the rule's `alert` field. The `autoscaling` label is an
example routing convention, not a Koperator authorization check; restrict who
can create action rules and access the webhook.

For scale-out, `brokerConfigGroup` must name an existing configuration group
suitable for a data broker. In KRaft mode, include the appropriate broker
process role in that group. Without a matching group, Koperator builds the
broker configuration from the alert's `image`, `storageClass`, `mountPath`,
and `diskSize` annotations; provide valid image and storage settings.

For scale-in, use `command: downScale`. An optional `brokerId` label selects
that broker. Without it, Cruise Control supplies replica counts and Koperator
selects a broker with the fewest replicas. Equal minima have no guaranteed
selection order. Failed lookups or empty maps leave the broker list unchanged.
Neither explicit selection nor minimum replica count proves removal is safe.

## Route only approved actions to Koperator

In the Alertmanager configuration described in the
[monitoring guide](monitoring.md#configure-notifications-and-actions), add a
child route under the existing `route.routes` list:

```yaml
- receiver: koperator-brokers
  matchers:
  - autoscaling="approved"
  - alertname=~"KafkaScaleOutApproved|KafkaScaleInApproved"
  - namespace="kafka"
  - kafka_cr="kafka"
```

Add its receiver to the existing `receivers` list:

```yaml
- name: koperator-brokers
  webhook_configs:
  - url: http://kafka-operator-alertmanager.kafka.svc:9001/
    send_resolved: true
```

These are configuration fragments: merge them without replacing the existing
`monitoring` or storage routes and receivers. Adjust names, labels, and the
webhook address for your installation. Do not route all `alertGroup="kafka"`
alerts to the action receiver.

Koperator accepts Prometheus alert arrays and Alertmanager webhook envelopes.
Keep resolved deliveries enabled so an alert can be cleared and a later
recurrence can be processed.

## What happens after an action

The cluster must be running. Koperator applies the count limits and checks for
relevant pending or running Cruise Control tasks before changing the broker
list.

| Action | Desired-state change | Subsequent work |
|---|---|---|
| `upScale` | Appends one broker to `spec.brokers`, using an ID one greater than the largest existing ID | Creates its resources and initiates Cruise Control scale-out work |
| `downScale` | Removes the selected broker from `spec.brokers` | In the normal graceful path for initialized data brokers, evacuates replicas through Cruise Control before deleting the broker pod and associated data PVCs |

Replica movement depends on Cruise Control's goals and available model.
Check actual placement and movement counts, not only task completion. This
guide describes data-broker scaling; controller-only removal and removal of
partially initialized brokers follow different paths.

### Repeated notifications

Koperator identifies an alert by a fingerprint of its labels. Once marked
processed, repeated notifications for that fingerprint do not continuously add
or remove brokers. A resolved notification clears the entry. Changing labels
can create a new fingerprint; changing annotations alone does not.

This state is in memory and is lost on an operator restart. It is not a durable
exactly-once guarantee or a cooldown. Do not assume a skipped scaling alert is
queued for execution when other work finishes.

## Verify progress and completion

Select the intended context, namespace, and KafkaCluster:

```sh
CONTEXT=your-kubernetes-context
NAMESPACE=kafka
CLUSTER=kafka
kubectl --context "$CONTEXT" -n "$NAMESPACE" get kafkacluster "$CLUSTER" -o yaml
kubectl --context "$CONTEXT" -n "$NAMESPACE" get cruisecontroloperations
kubectl --context "$CONTEXT" -n "$NAMESPACE" get pods,pvc
```

Confirm the requested change in `spec.brokers`, inspect graceful-action states
in `status.brokersState`, and check the corresponding CruiseControlOperation
results and operator logs. Verify replica placement, in-sync replicas, and
producer/consumer availability before allowing another action.

HTTP `202 Accepted` acknowledges the alert payload, not successful scaling.
Guards can skip an action, and action-handling errors can be logged without
changing that response. Pod readiness alone is also not proof of completion.

## Pause broker autoscaling

Resolve existing broker-action alerts while resolved deliveries are still
enabled, then disable their generation and action route. Keep diagnostic
notifications and any separately approved storage actions active.

Check for pending or running operations. Disabling future alerts does not
cancel an operation already underway. Do not delete broker pods or PVCs to
interrupt scaling.
