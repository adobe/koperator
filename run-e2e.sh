#!/bin/bash

# Check if cloud-provider-kind is available in PATH
if ! command -v cloud-provider-kind &> /dev/null; then
    echo "Error: cloud-provider-kind is not installed or not in PATH"
    echo "Please install it using: brew install cloud-provider-kind"
    exit 1
fi

export IMG_E2E=koperator_e2e_test:latest

export export KUBECONFIG=/tmp/kind
kind delete clusters e2e-kind
kind create cluster --config=tests/e2e/platforms/kind/kind_config.yaml --name=e2e-kind
kubectl label node e2e-kind-control-plane node.kubernetes.io/exclude-from-external-load-balancers-
docker build . -t koperator_e2e_test
kind load docker-image koperator_e2e_test:latest --name e2e-kind
kind load docker-image ghcr.io/adobe/koperator/kafka:2.13-3.9.1 --name e2e-kind
kind load docker-image ghcr.io/adobe/zookeeper-operator/zookeeper:3.8.4-0.2.15-adobe-20250923 --name e2e-kind
kind load docker-image adobe/cruise-control:3.0.3-adbe-20250804 --name e2e-kind

sudo cloud-provider-kind &>/tmp/cloud-provider-kind.log &


make test-e2e

kind delete cluster e2e-kind
sudo pkill -9 -f cloud-provider-kind
