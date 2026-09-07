# operator-tools Makefile
# Most common targets (test, lint, fmt, vet, generate, clean, license-check, etc.)
# are now handled by the parent Makefile at the repository root.
# This Makefile contains only unique targets specific to operator-tools.

SHELL = /bin/bash
.SHELLFLAGS := -eu -o pipefail -c

help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^([a-zA-Z_0-9-]|\/)+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

# Unique targets for operator-tools

.PHONY: check-diff
check-diff: ## Check for uncommitted changes (run from repository root)
	@echo "Note: This target should be run from the repository root directory"
	@echo "Run: make -C ../../.. check-diff-operator-tools"
