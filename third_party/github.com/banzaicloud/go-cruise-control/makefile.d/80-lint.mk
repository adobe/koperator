##@ Linter

GOLANGCI_VERSION = 1.51.2

bin/golangci-lint: bin/golangci-lint-$(GOLANGCI_VERSION)
	@ln -sf golangci-lint-$(GOLANGCI_VERSION) bin/golangci-lint
bin/golangci-lint-$(GOLANGCI_VERSION):
	@mkdir -p bin
	curl -sfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | bash -s -- -b ./bin/ v$(GOLANGCI_VERSION)
	@mv bin/golangci-lint $@

.PHONY: lint
lint: bin/golangci-lint ## Run linter
	@bin/golangci-lint run --timeout=240s

.PHONY: lint-fix
lint-fix: bin/golangci-lint ## Run linter and fix issues
	@bin/golangci-lint run --fix

.PHONY: lint-clean
lint-clean: bin/golangci-lint ## Clean linter cache
	@bin/golangci-lint cache clean
