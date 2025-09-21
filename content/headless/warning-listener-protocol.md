---
---
{{< warning >}}After the cluster is created, you cannot change the way the listeners are configured without an outage. If a cluster is created with unencrypted (plain text) listener and you want to switch to SSL encrypted listeners (or the way around), you must manually delete each broker pod. The operator will restart the pods with the new listener configuration.{{< /warning >}}
