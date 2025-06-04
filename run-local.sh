#!/bin/bash

# RUN KOPERATOR LOCALLY ON KIND
### Create kind cluster
kind delete clusters e2e-kind
kind create cluster --config=tests/e2e/platforms/kind/kind_config.yaml --name=e2e-kind

### Build/Load images
kind load docker-image docker-pipeline-upstream-mirror.dr-uw2.adobeitc.com/adobe/cruise-control:2.5.133-adbe-20240313 --name e2e-kind
kind load docker-image docker-pipeline-upstream-mirror.dr-uw2.adobeitc.com/adobe/kafka:2.13-3.7.0 --name e2e-kind
docker build . -t koperator_e2e_test
kind load docker-image koperator_e2e_test:latest --name e2e-kind

### Install Helm Charts and CRDs
#### project contour
helm repo add bitnami https://charts.bitnami.com/bitnami
helm install contour bitnami/contour --namespace projectcontour --create-namespace

#### cert-manager
helm repo add jetstack https://charts.jetstack.io --force-update
helm install cert-manager jetstack/cert-manager --namespace cert-manager  --create-namespace  --version v1.16.2  --set crds.enabled=true

#### zookeeper-operator
helm repo add pravega https://charts.pravega.io
helm install zookeeper-operator pravega/zookeeper-operator --version 0.2.15 --namespace zookeeper --create-namespace --set crd.create=true

#### prometheus
helm repo add prometheus https://prometheus-community.github.io/helm-charts
helm install prometheus prometheus/kube-prometheus-stack --version 54.1.0 --namespace prometheus --create-namespace 

#### koperator
helm install kafka-operator charts/kafka-operator --set operator.image.repository=koperator_e2e_test --set operator.image.tag=latest --namespace kafka --create-namespace
kubectl apply -f charts/kafka-operator/crds/  

### Initialize Kafka Cluster
kubectl apply -f config/samples/kraft/simplekafkacluster_kraft.yaml -n kafka
kubectl config set-context --current --namespace kafka 
