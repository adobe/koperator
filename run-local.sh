#!/bin/bash

# RUN KOPERATOR LOCALLY ON KIND
### Create kind cluster
kind delete clusters e2e-kind
kind create cluster --config=tests/e2e/platforms/kind/kind_config.yaml --name=e2e-kind

### Build/Load images
kind load docker-image adobe/cruise-control:3.0.3-adbe-20250804 --name e2e-kind
kind load docker-image ghcr.io/adobe/koperator/kafka:2.13-3.9.1 --name e2e-kind
docker build . -t koperator_e2e_test
kind load docker-image koperator_e2e_test:latest --name e2e-kind

### Install Helm Charts and CRDs
#### project contour
helm repo add bitnami https://charts.bitnami.com/bitnami
helm install contour bitnami/contour --version 15.4.0 --namespace projectcontour --create-namespace --set installCRDs=true

#### cert-manager
helm repo add jetstack https://charts.jetstack.io --force-update
helm install cert-manager jetstack/cert-manager --namespace cert-manager  --create-namespace  --version v1.11.0  --set installCRDs=false
# Install cert-manager CRDs manually
kubectl apply -f https://github.com/jetstack/cert-manager/releases/download/v1.11.0/cert-manager.crds.yaml

#### zookeeper-operator
helm install zookeeper-operator oci://ghcr.io/adobe/helm-charts/zookeeper-operator --version 0.2.15-adbe-20250923 --namespace zookeeper --create-namespace --set crd.create=false
# Install zookeeper-operator CRDs manually
kubectl apply -f https://raw.githubusercontent.com/adobe/zookeeper-operator/0.2.15-adbe-20250923/config/crd/bases/zookeeper.pravega.io_zookeeperclusters.yaml

#### prometheus
helm repo add prometheus https://prometheus-community.github.io/helm-charts
helm install prometheus prometheus/kube-prometheus-stack --version 54.1.0 --namespace prometheus --create-namespace \
  --set prometheusOperator.createCustomResource=true \
  --set defaultRules.enabled=false \
  --set alertmanager.enabled=false \
  --set grafana.enabled=false \
  --set kubeApiServer.enabled=false \
  --set kubelet.enabled=false \
  --set kubeControllerManager.enabled=false \
  --set coreDNS.enabled=false \
  --set kubeEtcd.enabled=false \
  --set kubeScheduler.enabled=false \
  --set kubeProxy.enabled=false \
  --set kubeStateMetrics.enabled=false \
  --set nodeExporter.enabled=false \
  --set prometheus.enabled=false

#### koperator
helm install kafka-operator charts/kafka-operator --set operator.image.repository=koperator_e2e_test --set operator.image.tag=latest --namespace kafka --create-namespace
kubectl apply -f charts/kafka-operator/crds/

### Initialize Kafka Cluster
kubectl apply -f config/samples/kraft/simplekafkacluster_kraft.yaml -n kafka
kubectl config set-context --current --namespace kafka
