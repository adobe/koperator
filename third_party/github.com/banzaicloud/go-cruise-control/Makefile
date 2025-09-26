SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

CRUISE_CONTROL_VERSION ?= 2.5.118-adbe-20230504
export CRUISE_CONTROL_IMAGE ?= docker.io/adobe/cruise-control:latest

##@ General

help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

fmt: ## Run go fmt against code
	@go fmt ./...

vet: ## Run go vet against code
	@go vet ./...

test: ## Run go test against code
	@go test \
		-v \
		-parallel 2 \
		-failfast \
		./... \
		-cover \
		-covermode=count \
		-coverprofile cover.out \
		-test.v \
		-test.paniconexit0

integration-test: ## Run go integration test against code
	@cd integration_test && \
 	go test \
 		-v \
 		-failfast \
 		-test.v \
 		-test.paniconexit0 \
 		-timeout 2h \
 		-ginkgo.v .

go.work:
	@go work init
	@go work use . integration_test


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

PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))

include makefile.d/*.mk
