#!/bin/bash
set -e

## Prerequisite checks
if [ -z "${SCALEOPS_TOKEN}" ]; then
  echo "Error: SCALEOPS_TOKEN environment variable is not set"
  exit 1
fi

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
helm repo add contour https://projectcontour.github.io/helm-charts/ || true
helm upgrade --install contour contour/contour --namespace projectcontour --create-namespace

### cert-manager
helm repo add jetstack https://charts.jetstack.io --force-update || true
helm upgrade --install cert-manager jetstack/cert-manager --namespace cert-manager --create-namespace --version v1.16.2 --set crds.enabled=true

### zookeeper-operator
helm repo add pravega https://charts.pravega.io || true
helm upgrade --install zookeeper-operator pravega/zookeeper-operator --version 0.2.15 --namespace zookeeper --create-namespace --set crd.create=true

### prometheus
helm repo add prometheus https://prometheus-community.github.io/helm-charts || true
helm upgrade --install prometheus prometheus/kube-prometheus-stack --version 54.1.0 --namespace prometheus --create-namespace

### scaleops
helm upgrade --install --create-namespace -n scaleops-system --repo https://registry.scaleops.com/charts/ --username scaleops --password ${SCALEOPS_TOKEN} --set scaleopsToken=${SCALEOPS_TOKEN} --set clusterName=$(kubectl config current-context) scaleops scaleops
kubectl apply -f config/scaleops/CustomOwnerGrouping.yaml
kubectl apply -f config/scaleops/KafkaBrokersPolicy.yaml
#### Scaleops Dashboard Port Forward
# kubectl port-forward <scaleops-dashboard-pod-name> 8080 -n scaleops-system
# (find pod name with: kubectl get pods -n scaleops-system)

## Run Koperator Locally
### Start Cloud Provider Kind in the background to enable LoadBalancer services for local koperator
# sudo ~/go/bin/cloud-provider-kind
# (run this manually in a separate terminal before starting koperator)

### Start Local Koperator instance:
kubectl create namespace kafka || true
kubectl ens kafka
make install
# Run koperator locally in a separate terminal:
# go run ./main.go --metrics-addr=:8090 --disable-webhooks

## Initialize Zookeeper and Kafka Cluster
kubectl apply -f config/samples/simplezookeeper.yaml -n zookeeper
kubectl apply -f config/samples/simplekafkacluster.yaml -n kafka

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
