# operator-tools Makefile
# Most common targets (test, lint, fmt, vet, generate, clean, license-check, etc.)
# are now handled by the parent Makefile at the repository root.
# This Makefile contains only unique targets specific to operator-tools.

SHELL = /bin/bash
.SHELLFLAGS := -eu -o pipefail -c

PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))
BIN_DIR := $(PROJECT_DIR)/bin

# Version constants
CONTROLLER_GEN_VERSION = v0.20.0 # renovate: datasource=github-releases depName=kubernetes-sigs/controller-tools

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

export PATH := $(BIN_DIR):$(PATH)

CONTROLLER_GEN = $(BIN_DIR)/controller-gen

help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^([a-zA-Z_0-9-]|\/)+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

# Unique targets for operator-tools

.PHONY: check-diff
check-diff: ## Check for uncommitted changes (run from repository root)
	@echo "Note: This target should be run from the repository root directory"
	@echo "Run: make -C ../../.. check-diff-operator-tools"

# Local tool installation (for standalone use of this module)

bin/controller-gen: bin/controller-gen-$(CONTROLLER_GEN_VERSION) ## Symlink controller-gen-<version> into versionless controller-gen.
	@ln -sf controller-gen-$(CONTROLLER_GEN_VERSION) bin/controller-gen

bin/controller-gen-$(CONTROLLER_GEN_VERSION): ## Download versioned controller-gen.
	@mkdir -p bin
	GOBIN=$(BIN_DIR) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_GEN_VERSION)
	@mv bin/controller-gen bin/controller-gen-$(CONTROLLER_GEN_VERSION)
