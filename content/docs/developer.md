---
title: Developer Guide
weight: 1000
---



## Contributing

If you find this project useful here's how you can help:

- Send a pull request with your new features and bug fixes
- Help new users with issues they may encounter
- Support the development of this project and [star this repo](https://github.com/banzaicloud/koperator/)!

When you are opening a PR to Koperator the first time we will require you to sign a standard CLA.

## How to run Koperator in your cluster with your changes

Koperator is built on the [kubebuilder](https://github.com/kubernetes-sigs/kubebuilder) project.

To build the operator and run tests:

1. Run `make`

If you make changes and would like to try your own version, create your own image:

1. `make docker-build IMG={YOUR_USERNAME}/kafka-operator:v0.0.1`
2. `make docker-push IMG={YOUR_USERNAME}/kafka-operator:v0.0.1`
3. `make deploy IMG={YOUR_USERNAME}/kafka-operator:v0.0.1`

Watch the operator's logs with:

`kubectl logs -f -n kafka kafka-operator-controller-manager-0 -c manager`

Alternatively, run the operator on your machine:

1. `export $KUBECONFIG`
2. `make install`
3. `make run`

Create CR and let the operator set up Kafka in your cluster (you can change the `spec` of `Kafka` for your needs in the yaml file):

> Remember you need an Apache ZooKeeper server to run Kafka

`kubectl create -n kafka -f config/samples/simplekafkacluster.yaml`

## Limitations on minikube

Minikube does not have a load balancer implementation, thus our envoy service will not get an external IP and the operator will get stuck at this point.

A possible solution to overcome this problem is to use https://github.com/elsonrodriguez/minikube-lb-patch. The operator will be able to proceed if you run the following command:

```go
kubectl run minikube-lb-patch --replicas=1 --image=elsonrodriguez/minikube-lb-patch:0.1 --namespace=kube-system
```
