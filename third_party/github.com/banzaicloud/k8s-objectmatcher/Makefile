# k8s-objectmatcher Makefile
# Most common targets (test, lint, fmt, vet, clean, license, license-check, etc.)
# are now handled by the parent Makefile at the repository root.
# This Makefile contains only unique targets specific to k8s-objectmatcher.

SHELL = /bin/bash
.SHELLFLAGS := -eu -o pipefail -c

PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))

help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^([a-zA-Z_0-9-]|\/)+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

# Unique targets for k8s-objectmatcher

.PHONY: test-integration
test-integration: ## Run integration tests.
	cd tests && go test -integration -v ./...
