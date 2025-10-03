# go-cruise-control Makefile
# Most common targets (test, lint, fmt, vet, license-check, etc.)
# are now handled by the parent Makefile at the repository root.
# This Makefile contains only unique targets specific to go-cruise-control.

SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

CRUISE_CONTROL_VERSION ?= 2.5.118-adbe-20230504
export CRUISE_CONTROL_IMAGE ?= docker.io/adobe/cruise-control:latest

PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))

##@ General

help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Unique targets for go-cruise-control

.PHONY: integration-test
integration-test: ## Run go integration test against code
	@cd integration_test && \
 	go test \
 		-v \
 		-failfast \
 		-test.v \
 		-test.paniconexit0 \
 		-timeout 2h \
 		-ginkgo.v .

.PHONY: go.work
go.work: ## Create Go workspace
	@go work init
	@go work use . integration_test

.PHONY: update-go-deps
update-go-deps: ## Update Go modules dependencies.
	@echo "Finding all directories with go.mod files..."
	@for gomod in $$(find . -name "go.mod" | sort); do \
		dir=$$(dirname $$gomod); \
		( \
		echo "Updating $$dir deps"; \
		cd $$dir; \
		go mod tidy; \
		for m in $$(go list -mod=readonly -m -f '{{ if and (not .Replace) (not .Indirect) (not .Main)}}{{.Path}}{{end}}' all); do \
			go get -u $$m; \
		done; \
		go mod tidy \
		) \
	done

# Include additional targets from makefile.d/
include makefile.d/*.mk
