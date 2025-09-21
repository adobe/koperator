---
---
{{< warning >}}The ZooKeeper and the Kafka clusters need [persistent volume (PV)](https://kubernetes.io/docs/concepts/storage/persistent-volumes/) to store data. Therefore, when installing the operator on Amazon EKS with Kubernetes version 1.23 or later, you [must install the EBS CSI driver add-on](https://docs.aws.amazon.com/eks/latest/userguide/managing-ebs-csi.html) on your cluster. {{< /warning >}}
