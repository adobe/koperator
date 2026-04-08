#!/bin/bash
## Create kind cluster
kind delete clusters kind-kafka
kind create cluster --config=./tests/e2e/platforms/kind/kind_config.yaml --name=kind-kafka

## Build/Load images (Kafka 3.7.0)
kind load docker-image docker-pipeline-upstream-mirror.dr-uw2.adobeitc.com/adobe/kafka:2.13-3.7.0 --name kind-kafka
### Skip if you want to run koperator locally
docker build . -t koperator_e2e_test
kind load docker-image koperator_e2e_test:latest --name kind-kafka

## Install Helm Charts and CRDs
### project contour
helm repo add contour https://projectcontour.github.io/helm-charts/
helm install contour contour/contour --namespace projectcontour --create-namespace

### cert-manager
helm repo add jetstack https://charts.jetstack.io --force-update
helm install cert-manager jetstack/cert-manager --namespace cert-manager  --create-namespace  --version v1.16.2  --set crds.enabled=true

### zookeeper-operator
helm repo add pravega https://charts.pravega.io
helm install zookeeper-operator pravega/zookeeper-operator --version 0.2.15 --namespace zookeeper --create-namespace --set crd.create=true

### prometheus
helm repo add prometheus https://prometheus-community.github.io/helm-charts
helm install prometheus prometheus/kube-prometheus-stack --version 54.1.0 --namespace prometheus --create-namespace 

## Run Koperator on Kind
### koperator - Run as container on Kind (Skip if you want to run koperator locally)
helm install kafka-operator charts/kafka-operator --set operator.image.repository=koperator_e2e_test --set operator.image.tag=latest --namespace kafka --create-namespace

## Run Koperator Locally
### Start Cloud Provider Kind in the background to enable LoadBalancer services for local koperator
sudo ~/go/bin/cloud-provider-kind &

### Start Local Koperator instance:
make install
make run

## Initialize Zookeeper and Kafka Cluster
k apply -f config/samples/simplezookeeper.yaml -n zookeeper
k create namespace kafka
k ens kafka
k apply -f config/samples/simplekafkacluster.yaml -n kafka

# NOTES for running koperator locally:
#
# If you want to run koperator locally, make sure to set `debugEnabled: true`
# in your KafkaCluster spec. This will create LoadBalancer services for the
# Kafka and Cruise Control pods, allowing your local koperator to access
# services running on the Kind cluster.
#
# Cloud Provider KIND is required to enable LoadBalancer services on Kind.
# This is necessary for local koperator access. If you don't want to run it,
# you can port-forward the services instead.
#
# Finally, you'll need to update your /etc/hosts file to direct request from 
# Koperator to the LoadBalancer IPs. You can find the LoadBalancer IPs by running:
#   kubectl get svc -n kafka
#
# Your /etc/hosts entries should look something like this:
#   172.18.0.7   kafka-0.kafka.svc.cluster.local
#   172.18.0.9   kafka-1.kafka.svc.cluster.local
#   172.18.0.10  kafka-2.kafka.svc.cluster.local
#   172.18.0.11  kafka-all-broker.kafka.svc.cluster.local
#   172.18.0.8   kafka-cruisecontrol-svc.kafka.svc.cluster.local