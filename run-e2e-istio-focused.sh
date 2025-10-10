#!/bin/bash

export IMG_E2E=koperator_e2e_test:latest

kind delete clusters e2e-kind
kind create cluster --config=tests/e2e/platforms/kind/kind_config.yaml --name=e2e-kind
kubectl label node e2e-kind-control-plane node.kubernetes.io/exclude-from-external-load-balancers-
docker build . -t koperator_e2e_test
kind load docker-image koperator_e2e_test:latest --name e2e-kind
kind load docker-image ghcr.io/adobe/koperator/kafka:2.13-3.9.1 --name e2e-kind
kind load docker-image adobe/cruise-control:3.0.3-adbe-20250804 --name e2e-kind

sudo ~/go/bin/cloud-provider-kind &

#make test-e2e
# Focus on only Istio installation and Istio tests, skipping cert-manager, contour, zookeeper-operator, prometheus-operator
cd tests/e2e && IMG_E2E=${IMG_E2E} go test . \
       -v \
       -timeout 45m \
       -tags e2e \
       --ginkgo.focus="Installing Istio|Installing Koperator|Istio" \
       --ginkgo.show-node-events \
       --ginkgo.trace \
       --ginkgo.v
