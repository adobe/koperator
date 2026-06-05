#!/bin/bash

## PREREQUISITES
# 1. Install Kind: https://kind.sigs.k8s.io/docs/user/quick-start/
# 2. Start Docker Daemon and ensure it's running
# 3. If using SCALEOPS, set SCALEOPS_TOKEN env variable with your ScaleOps API token
# 4. Install and Start cloud-provider-kind to enable LoadBalancer services on Kind (Required for Local Debugging). https://github.com/kubernetes-sigs/cloud-provider-kind

## USAGE
# ./run-local.sh [--local] [--scaleops]
#
# --local     Run koperator as a local process instead of as a container on Kind.
#             Starts cloud-provider-kind and runs `make install && make run`.
# --scaleops  Install the ScaleOps helm chart. Requires SCALEOPS_TOKEN to be set.


## IMPORTANT NOTES (for running koperator locally with --local flag)
#
# Make sure to set `debugEnabled: true` in your KafkaCluster spec. This will
# create LoadBalancer services for the Kafka and Cruise Control pods, allowing
# your local koperator to access services running on the Kind cluster.
#
# Cloud Provider KIND is required to enable LoadBalancer services on Kind.
# If you don't want to run it, you can port-forward the services instead. If you are running in local
# mode and notice that your kafka services don't have an external IP, it's because cloud-provider-kind
# either isn't running or has some issue. Local koperator won't be able to communicate
# with kafka pods without these.
#
# Finally, you'll need to update your /etc/hosts file to direct requests from
# Koperator to the LoadBalancer IPs. You can find the LoadBalancer IPs by running:
#   kubectl get svc -n kafka
#
# Your /etc/hosts entries should look something like this:
#   172.18.0.7   kafka-0.kafka.svc.cluster.local
#   172.18.0.9   kafka-1.kafka.svc.cluster.local
#   172.18.0.10  kafka-2.kafka.svc.cluster.local
#   172.18.0.11  kafka-all-broker.kafka.svc.cluster.local
#   172.18.0.8   kafka-cruisecontrol-svc.kafka.svc.cluster.local


## ATTACHING A DEBUGGER TO LOCAL KOPERATOR
# If you need to debug your local koperator, you can find the logs in /tmp/koperator.log.
# Additionally, you can attach a debugger to the koperator process using VSCODE. Instead of running `make run`,
# start koperator as a Go application with debug enabled from VSCode, and set breakpoints as needed.
# This can be done by simply opening main.go in VSCode, going to the DEBUG Tab, and clicking Run and Debug.

LOCAL=false
SCALEOPS=false

while [[ $# -gt 0 ]]; do
  case $1 in
    --local)    LOCAL=true;    shift ;;
    --scaleops) SCALEOPS=true; shift ;;
    *) echo "Unknown flag: $1"; exit 1 ;;
  esac
done

if $SCALEOPS && [[ -n "${SCALEOPS_TOKEN}" ]]; then
  echo "Error: --scaleops requires SCALEOPS_TOKEN to be set"
  exit 1
fi

## Check if Docker daemon is running
if ! docker ps &>/dev/null; then
  echo "Error: Docker daemon is not running. Please start Docker and try again."
  exit 1
fi

## Create kind cluster
kind delete clusters kind-kafka
kind create cluster --config=./tests/e2e/platforms/kind/kind_config.yaml --name=kind-kafka

## Validate kubectl context is set to kind
CURRENT_CONTEXT=$(kubectl config current-context)
if [[ ! "$CURRENT_CONTEXT" =~ kind ]]; then
  echo "Error: kubectl context is not set to a kind cluster. Current context: $CURRENT_CONTEXT"
  exit 1
fi

## Build/Load images (Kafka 3.7.0)
kind load docker-image docker-pipeline-upstream-mirror.dr-uw2.adobeitc.com/adobe/kafka:2.13-3.7.0 --name kind-kafka

if ! $LOCAL; then
  docker build . -t koperator_e2e_test
  kind load docker-image koperator_e2e_test:latest --name kind-kafka
fi

## Install Helm Charts and CRDs
### project contour
helm repo add contour https://projectcontour.github.io/helm-charts/ --force-update
helm upgrade --install contour contour/contour --namespace projectcontour --create-namespace

### cert-manager
helm repo add jetstack https://charts.jetstack.io --force-update
helm upgrade --install cert-manager jetstack/cert-manager --namespace cert-manager --create-namespace --version v1.16.2 --set crds.enabled=true

### zookeeper-operator
helm repo add pravega https://charts.pravega.io --force-update
helm upgrade --install zookeeper-operator pravega/zookeeper-operator --version 0.2.15 --namespace zookeeper --create-namespace --set crd.create=true

### prometheus
helm repo add prometheus https://prometheus-community.github.io/helm-charts --force-update
helm upgrade --install prometheus prometheus/kube-prometheus-stack --version 54.1.0 --namespace prometheus --create-namespace

### scaleops
if $SCALEOPS; then
  helm upgrade --install --create-namespace -n scaleops-system \
    --repo https://registry.scaleops.com/charts/ \
    --username scaleops --password "${SCALEOPS_TOKEN}" \
    --set scaleopsToken="${SCALEOPS_TOKEN}" \
    --set clusterName="$(kubectl config current-context)" \
    scaleops scaleops
  kubectl apply -f config/scaleops/CustomOwnerGrouping.yaml
fi

## Run Koperator
if $LOCAL; then
  ## Check if cloud-provider-kind started successfully
  if ! pgrep -f cloud-provider-kind &>/dev/null; then
    echo "Warning: cloud-provider-kind failed to start. LoadBalancer services may not work properly."
    echo "Check /tmp/cloudproviderkind.log for details."
  fi

  kubectl get namespace kafka &>/dev/null || kubectl create namespace kafka
  kubectl config set-context --current --namespace=kafka
  make install

else
  helm upgrade --install kafka-operator charts/kafka-operator \
    --set operator.image.repository=koperator_e2e_test \
    --set operator.image.tag=latest \
    --set prometheusMetrics.enabled=false \
    --namespace kafka --create-namespace
fi

## Initialize Zookeeper and Kafka Cluster
kubectl apply -f config/samples/simplezookeeper.yaml -n zookeeper

if ! $LOCAL; then
  kubectl wait --for=condition=Ready pod -l app.kubernetes.io/name=kafka-operator -n kafka --timeout=120s
  sleep 5
fi

kubectl apply -f config/samples/simplekafkacluster.yaml -n kafka

## Start Local Koperator
if $LOCAL; then
  make run 
fi
