#!/bin/bash

export IMG_E2E=koperator_e2e_test:latest

kind delete clusters e2e-kind
kind create cluster --config=tests/e2e/platforms/kind/kind_config.yaml --name=e2e-kind
kubectl label node e2e-kind-control-plane node.kubernetes.io/exclude-from-external-load-balancers-
docker build . -t koperator_e2e_test
kind load docker-image koperator_e2e_test:latest --name e2e-kind
kind load docker-image docker-pipeline-upstream-mirror.dr-uw2.adobeitc.com/adobe/kafka:2.13-3.7.0 --name e2e-kind
kind load docker-image docker-pipeline-upstream-mirror.dr-uw2.adobeitc.com/adobe/cruise-control:2.5.133-adbe-20240313 --name e2e-kind

go test ./tests/e2e -v  -timeout 20m -tags e2e --ginkgo.show-node-events --ginkgo.trace --ginkgo.v
