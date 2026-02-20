SHELL = /bin/bash
.SHELLFLAGS := -eu -o pipefail -c

PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))
BIN_DIR := $(PROJECT_DIR)/bin
BOILERPLATE_DIR := $(PROJECT_DIR)/hack/boilerplate

# Image URL to use all building/pushing image targets
TAG ?= $(shell git describe --tags --abbrev=0 --match '[0-9].*[0-9].*[0-9]*' 2>/dev/null )
IMG ?= ghcr.io/adobe/kafka-operator:$(TAG)

# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd"

RELEASE_TYPE ?= p
RELEASE_MSG ?= "koperator release"

REL_TAG = $(shell ./scripts/increment_version.sh -${RELEASE_TYPE} ${TAG})

# Version constants
GOLANGCI_VERSION = 2.10.1 # renovate: datasource=github-releases depName=golangci/golangci-lint
LICENSEI_VERSION = 0.9.0 # renovate: datasource=github-releases depName=goph/licensei
CONTROLLER_GEN_VERSION = v0.20.1 # renovate: datasource=github-releases depName=kubernetes-sigs/controller-tools
ENVTEST_K8S_VERSION = 1.35.0 # renovate: datasource=github-releases depName=kubernetes-sigs/controller-tools extractVersion=^envtest-v(?<version>.+)$
SETUP_ENVTEST_VERSION := latest
ADDLICENSE_VERSION := 1.2.0 # renovate: datasource=github-releases depName=google/addlicense
GOTEMPLATE_VERSION := 3.12.0 # renovate: datasource=github-releases depName=cznic/gotemplate
MOCKGEN_VERSION := 0.6.0 # renovate: datasource=github-releases depName=uber-go/mock

GOPROXY=https://proxy.golang.org

# Directories to run golangci-lint in
LINT_DIRS := . api properties tests/e2e \
	third_party/github.com/banzaicloud/operator-tools \
	third_party/github.com/banzaicloud/k8s-objectmatcher \
	third_party/github.com/banzaicloud/go-cruise-control

# Directories to run licensei check/cache in
LICENSE_CHECK_DIRS := . \
	third_party/github.com/banzaicloud/operator-tools \
	third_party/github.com/banzaicloud/k8s-objectmatcher \
	third_party/github.com/banzaicloud/go-cruise-control

# Directories to run licensei header in
LICENSE_HEADER_DIRS := \
	third_party/github.com/banzaicloud/k8s-objectmatcher \
	third_party/github.com/banzaicloud/go-cruise-control

# Use BIN_DIR to form an absolute, stable path regardless of `cd` usage
CONTROLLER_GEN = $(BIN_DIR)/controller-gen

KUSTOMIZE_BASE = config/overlays/specific-manager-version

HELM_CRD_PATH = charts/kafka-operator/crds

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

export PATH := $(PWD)/bin:$(PATH)

help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^([a-zA-Z_0-9-]|\/)+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

all: test manager ## Run 'test' and 'manager' targets.

.PHONY: check
check: test lint ## Run tests and linters

.PHONY: clean
clean: ## Clean build artifacts and test binaries
	@echo "Cleaning build artifacts..."
	@if [ -d "bin" ]; then \
		chmod -R u+w bin/ 2>/dev/null || true; \
		rm -rf bin/; \
	fi
	@rm -f cover.out
	@rm -f manager_image_patch.yaml
	@echo "Cleaning third_party/github.com/banzaicloud/operator-tools..."
	@if [ -d "third_party/github.com/banzaicloud/operator-tools/bin" ]; then \
		chmod -R u+w third_party/github.com/banzaicloud/operator-tools/bin/ 2>/dev/null || true; \
		rm -rf third_party/github.com/banzaicloud/operator-tools/bin/; \
	fi
	@echo "Cleaning third_party/github.com/banzaicloud/k8s-objectmatcher..."
	@if [ -d "third_party/github.com/banzaicloud/k8s-objectmatcher/bin" ]; then \
		chmod -R u+w third_party/github.com/banzaicloud/k8s-objectmatcher/bin/ 2>/dev/null || true; \
		rm -rf third_party/github.com/banzaicloud/k8s-objectmatcher/bin/; \
	fi

bin/golangci-lint: bin/golangci-lint-${GOLANGCI_VERSION} ## Symlink golangi-lint-<version> into versionless golangci-lint.
	@ln -sf golangci-lint-${GOLANGCI_VERSION} bin/golangci-lint
bin/golangci-lint-${GOLANGCI_VERSION}: ## Download versioned golangci-lint.
	@mkdir -p bin
	curl -sfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | bash -s -- -b ./bin v${GOLANGCI_VERSION}
	@mv bin/golangci-lint $@

.PHONY: lint
lint: bin/golangci-lint ## Run linter analysis.
	@for dir in $(LINT_DIRS); do \
		echo "Running lint in $$dir"; \
		(cd $$dir && $(CURDIR)/bin/golangci-lint run -c $(CURDIR)/.golangci.yml --timeout=5m) || exit 1; \
	done

.PHONY: lint-fix
lint-fix: bin/golangci-lint ## Run linter with automatic fixes.
	@for dir in $(LINT_DIRS); do \
		echo "Running lint-fix in $$dir"; \
		(cd $$dir && $(CURDIR)/bin/golangci-lint run -c $(CURDIR)/.golangci.yml --fix --timeout=5m) || exit 1; \
	done

.PHONY: lint-clean
lint-clean: bin/golangci-lint ## Clean linter cache.
	@echo "Cleaning golangci-lint cache..."
	bin/golangci-lint cache clean

bin/licensei: bin/licensei-${LICENSEI_VERSION} ## Symlink licensei-<version> into versionless licensei.
	@ln -sf licensei-${LICENSEI_VERSION} bin/licensei
bin/licensei-${LICENSEI_VERSION}: ## Download versioned licensei.
	@mkdir -p bin
	curl -sfL https://raw.githubusercontent.com/goph/licensei/master/install.sh | bash -s v${LICENSEI_VERSION}
	@mv bin/licensei $@

.PHONY: license-check
license-check: bin/licensei ## Run license check.
	@for dir in $(LICENSE_CHECK_DIRS); do \
		echo "Running license check in $$dir..."; \
		(cd $$dir && $(CURDIR)/bin/licensei check) || exit 1; \
	done

.PHONY: license-cache
license-cache: bin/licensei ## Generate license cache.
	@for dir in $(LICENSE_CHECK_DIRS); do \
		echo "Generating license cache in $$dir..."; \
		(cd $$dir && $(CURDIR)/bin/licensei cache) || exit 1; \
	done

.PHONY: license
license: bin/licensei ## Add license headers to source files.
	@for dir in $(LICENSE_HEADER_DIRS); do \
		echo "Adding license headers in $$dir..."; \
		(cd $$dir && $(CURDIR)/bin/licensei header) || exit 1; \
	done

install-kustomize: ## Install kustomize.
	@ if ! which bin/kustomize &>/dev/null; then\
		scripts/install_kustomize.sh;\
	fi

# Run tests
test: generate fmt vet bin/setup-envtest
	cd api && go test ./...
	KUBEBUILDER_ASSETS=$$($(BIN_DIR)/setup-envtest --print path --bin-dir $(BIN_DIR) use $(ENVTEST_K8S_VERSION)) \
	go test ./... \
		-coverprofile cover.out \
		-v \
		-failfast \
		-test.v \
		-test.paniconexit0 \
		-timeout 1h
	cd properties && go test -coverprofile cover.out -cover -failfast -v -covermode=count ./pkg/... ./internal/...
	@echo "Running tests in third_party/github.com/banzaicloud/operator-tools..."
	cd third_party/github.com/banzaicloud/operator-tools && \
	KUBEBUILDER_ASSETS=$$($(BIN_DIR)/setup-envtest --print path --bin-dir $(BIN_DIR) use $(ENVTEST_K8S_VERSION)) \
	go test ./... -v -failfast
	@echo "Running tests in third_party/github.com/banzaicloud/k8s-objectmatcher..."
	cd third_party/github.com/banzaicloud/k8s-objectmatcher && go test ./...
	@echo "Running tests in third_party/github.com/banzaicloud/go-cruise-control..."
	cd third_party/github.com/banzaicloud/go-cruise-control && \
	go test -v -parallel 2 -failfast ./... -cover -covermode=count -coverprofile cover.out -test.v -test.paniconexit0

# Run e2e tests
test-e2e:
	cd tests/e2e && IMG_E2E=${IMG_E2E} go test . \
		-v \
		-timeout 45m \
		-tags e2e \
		--ginkgo.show-node-events \
		--ginkgo.trace \
		--ginkgo.v

manager: generate fmt vet ## Generate (kubebuilder) and build manager binary.
	go build -o bin/manager main.go

# Run against the configured Kubernetes cluster in ~/.kube/config
run: generate fmt vet
	go run ./main.go

# Install CRDs into a cluster by manually creating or replacing the CRD depending on whether is currently existing
# Apply is not applicable as the last-applied-configuration annotation would exceed the size limit enforced by the api server
install: manifests ## Install generated CRDs into the configured Kubernetes cluster.
	kubectl create -f config/base/crds || kubectl replace -f config/base/crds

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: install-kustomize install ## Deploy controller into the configured Kubernetes cluster.
	# creates the kafka namespace
	bin/kustomize build config | kubectl apply -f -
	./scripts/image_patch.sh "${KUSTOMIZE_BASE}/manager_image_patch.yaml" ${IMG}
	bin/kustomize build $(KUSTOMIZE_BASE) | kubectl apply -f -

# Generate manifests e.g. CRD, RBAC etc.
manifests: bin/controller-gen ## Generate (Kubebuilder) manifests e.g. CRD, RBAC etc.
	cd api && $(CONTROLLER_GEN) $(CRD_OPTIONS) webhook paths="./..." output:crd:artifacts:config=../config/base/crds output:webhook:artifacts:config=../config/base/webhook
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role paths="./controllers/..." output:rbac:artifacts:config=./config/base/rbac
	## Regenerate CRDs and RBAC for the helm chart
	cp config/base/crds/kafka.banzaicloud.io_cruisecontroloperations.yaml $(HELM_CRD_PATH)/cruisecontroloperations.yaml
	cp config/base/crds/kafka.banzaicloud.io_kafkaclusters.yaml $(HELM_CRD_PATH)/kafkaclusters.yaml
	cp config/base/crds/kafka.banzaicloud.io_kafkatopics.yaml $(HELM_CRD_PATH)/kafkatopics.yaml
	cp config/base/crds/kafka.banzaicloud.io_kafkausers.yaml $(HELM_CRD_PATH)/kafkausers.yaml
	@sed -n '1,/# RBAC_RULES_START - Do not edit between markers, managed by make manifests/p' charts/kafka-operator/templates/operator-rbac.yaml > charts/kafka-operator/templates/operator-rbac.yaml.tmp
	@awk '/^rules:$$/,0' config/base/rbac/role.yaml | tail -n +2 >> charts/kafka-operator/templates/operator-rbac.yaml.tmp
	@sed -n '/# RBAC_RULES_END/,$$p' charts/kafka-operator/templates/operator-rbac.yaml >> charts/kafka-operator/templates/operator-rbac.yaml.tmp
	@mv charts/kafka-operator/templates/operator-rbac.yaml.tmp charts/kafka-operator/templates/operator-rbac.yaml

fmt: ## Run go fmt against code.
	go fmt ./...
	cd api && go fmt ./...
	cd properties && go fmt ./...
	cd tests/e2e && go fmt ./...
	@echo "Running fmt in third_party/github.com/banzaicloud/k8s-objectmatcher..."
	cd third_party/github.com/banzaicloud/k8s-objectmatcher && go fmt ./...
	@echo "Running fmt in third_party/github.com/banzaicloud/go-cruise-control..."
	cd third_party/github.com/banzaicloud/go-cruise-control && go fmt ./...

vet: ## Run go vet against code.
	go vet ./...
	cd api && go fmt ./...
	cd properties && go vet ./...
	cd tests/e2e && go vet ./...
	@echo "Running vet in third_party/github.com/banzaicloud/k8s-objectmatcher..."
	cd third_party/github.com/banzaicloud/k8s-objectmatcher && go vet ./...
	@echo "Running vet in third_party/github.com/banzaicloud/go-cruise-control..."
	cd third_party/github.com/banzaicloud/go-cruise-control && go vet ./...

generate: bin/controller-gen gen-license-header ## Generate source code for APIs, Mocks, etc.
	cd api && $(CONTROLLER_GEN) object:headerFile=$(BOILERPLATE_DIR)/header.go.generated.txt paths="./..."
	@echo "Running generate in third_party/github.com/banzaicloud/operator-tools..."
	cd third_party/github.com/banzaicloud/operator-tools && \
	$(CONTROLLER_GEN) object:headerFile=./hack/boilerplate.go.txt paths=./pkg/secret/... && \
	$(CONTROLLER_GEN) object:headerFile=./hack/boilerplate.go.txt paths=./pkg/volume/... && \
	$(CONTROLLER_GEN) object:headerFile=./hack/boilerplate.go.txt paths=./pkg/prometheus/... && \
	$(CONTROLLER_GEN) object:headerFile=./hack/boilerplate.go.txt paths=./pkg/types/... && \
	$(CONTROLLER_GEN) object:headerFile=./hack/boilerplate.go.txt paths=./pkg/typeoverride/... && \
	$(CONTROLLER_GEN) object:headerFile=./hack/boilerplate.go.txt paths=./pkg/helm/...
	@echo "Generating type documentation for operator-tools..."
	cd third_party/github.com/banzaicloud/operator-tools && go run cmd/docs.go

.PHONY: check-diff
check-diff: generate ## Check for uncommitted changes
	@echo "Checking for uncommitted changes ..."
	git diff --exit-code

docker-build: ## Build the operator docker image.
	docker build . -t ${IMG}

docker-push: ## Push the operator docker image.
	docker push ${IMG}

# PLATFORMS defines the target platforms for the manager image be built to provide support to multiple
# architectures. (i.e. make docker-buildx IMG=myregistry/mypoperator:0.0.1). To use this option you need to:
# - be able to use docker buildx. More info: https://docs.docker.com/build/buildx/
# - have enabled BuildKit. More info: https://docs.docker.com/develop/develop-images/build_enhancements/
# - be able to push the image to your registry (i.e. if you do not set a valid value via IMG=<myregistry/image:<tag>> then the export will fail)
# To adequately provide solutions that are compatible with multiple platforms, you should consider using this option.
PLATFORMS ?= linux/arm64,linux/amd64
.PHONY: docker-buildx
docker-buildx: ## Build and push docker image for the manager for cross-platform support
	- docker buildx create --name koperator-builder
	docker buildx use koperator-builder
	docker buildx build --push --platform=$(PLATFORMS) --tag ${IMG} -f Dockerfile .
	- docker buildx rm koperator-builder

bin/controller-gen: bin/controller-gen-$(CONTROLLER_GEN_VERSION) ## Symlink controller-gen-<version> into versionless controller-gen.
	@ln -sf controller-gen-$(CONTROLLER_GEN_VERSION) bin/controller-gen

bin/controller-gen-$(CONTROLLER_GEN_VERSION): ## Download versioned controller-gen.
	GOBIN=$(PWD)/bin go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_GEN_VERSION)
	mv bin/controller-gen bin/controller-gen-$(CONTROLLER_GEN_VERSION)

# find or download setup-envtest

# https://github.com/kubernetes-sigs/controller-runtime/commits/main/tools/setup-envtest

bin/setup-envtest: $(BIN_DIR)/setup-envtest-$(SETUP_ENVTEST_VERSION) ## Symlink setup-envtest-<version> into versionless setup-envtest.
	@ln -sf setup-envtest-$(SETUP_ENVTEST_VERSION) $(BIN_DIR)/setup-envtest

$(BIN_DIR)/setup-envtest-$(SETUP_ENVTEST_VERSION): ## Download versioned setup-envtest.
	@mkdir -p $(BIN_DIR)
	@GOBIN=$(BIN_DIR) go install sigs.k8s.io/controller-runtime/tools/setup-envtest@$(SETUP_ENVTEST_VERSION)
	@mv $(BIN_DIR)/setup-envtest $(BIN_DIR)/setup-envtest-$(SETUP_ENVTEST_VERSION)

check-release: ## Release confirmation.
	@echo "A new tag (${REL_TAG}) will be pushed to Github, and a new Docker image will be released. Are you sure? [y/N] " && read ans && [ $${ans:-N} == y ]

release: check-release ## Tag and push a release.
	git tag -a ${REL_TAG} -m ${RELEASE_MSG}
	git push origin ${REL_TAG}

define update-module-deps
	echo "Updating $(1) deps"; \
	cd $(1); \
	go mod tidy; \
	for m in $$(go list -mod=readonly -m -f '{{ if and (not .Replace) (not .Indirect) (not .Main)}}{{.Path}}{{end}}' all); do \
		go get -u $$m; \
	done; \
	go mod tidy
endef

update-go-deps: ## Update Go modules dependencies.
	@echo "Updating third_party modules first (deepest to shallowest)..."
	@for gomod in $$(find ./third_party -name "go.mod" 2>/dev/null | awk '{print length, $$0}' | sort -rn | cut -d' ' -f2-); do \
		dir=$$(dirname $$gomod); \
		($(call update-module-deps,$$dir)); \
	done
	@echo "Updating properties, api, and root modules..."
	@for dir in ./properties ./api .; do \
		if [ -f $$dir/go.mod ]; then \
			($(call update-module-deps,$$dir)); \
		fi \
	done
	@echo "Updating tests/e2e module last..."
	@if [ -f ./tests/e2e/go.mod ]; then \
		($(call update-module-deps,./tests/e2e)); \
	fi

tidy: ## Run go mod tidy in all Go modules.
	@echo "Finding all directories with go.mod files..."
	@for gomod in $$(find . -name "go.mod" | sort); do \
		dir=$$(dirname $$gomod); \
		( \
		echo "Running go mod tidy in $$dir"; \
		cd $$dir; \
		go mod tidy \
		) \
	done

bin/addlicense: $(BIN_DIR)/addlicense-$(ADDLICENSE_VERSION) ## Symlink addlicense-<version> into versionless addlicense.
	@ln -sf addlicense-$(ADDLICENSE_VERSION) $(BIN_DIR)/addlicense

$(BIN_DIR)/addlicense-$(ADDLICENSE_VERSION): ## Download versioned addlicense.
	@mkdir -p $(BIN_DIR)
	@GOBIN=$(BIN_DIR) go install github.com/google/addlicense@v$(ADDLICENSE_VERSION)
	@mv $(BIN_DIR)/addlicense $(BIN_DIR)/addlicense-$(ADDLICENSE_VERSION)

ADDLICENSE_SOURCE_DIRS := api controllers internal pkg properties scripts tests/e2e
ADDLICENSE_OPTS_IGNORE := -ignore '**/*.yml' -ignore '**/*.yaml' -ignore '**/*.xml'

.PHONY: license-header-check
license-header-check: gen-license-header bin/addlicense ## Find missing license header in source code files.
	bin/addlicense \
		-check \
		-f $(BOILERPLATE_DIR)/header.generated.txt \
		$(ADDLICENSE_OPTS_IGNORE) \
		$(ADDLICENSE_SOURCE_DIRS)

.PHONY: license-header-fix
license-header-fix: gen-license-header bin/addlicense ## Fix missing license header in source code files.
	bin/addlicense \
		-f $(BOILERPLATE_DIR)/header.generated.txt \
		$(ADDLICENSE_OPTS_IGNORE) \
		$(ADDLICENSE_SOURCE_DIRS)

bin/gotemplate: $(BIN_DIR)/gotemplate-$(GOTEMPLATE_VERSION) ## Symlink gotemplate-<version> into versionless gotemplate.
	@ln -sf gotemplate-$(GOTEMPLATE_VERSION) $(BIN_DIR)/gotemplate

$(BIN_DIR)/gotemplate-$(GOTEMPLATE_VERSION): ## Download versioned gotemplate.
	@mkdir -p $(BIN_DIR)
	@GOBIN=$(BIN_DIR) go install github.com/coveooss/gotemplate/v3@v$(GOTEMPLATE_VERSION)
	@mv $(BIN_DIR)/gotemplate $(BIN_DIR)/gotemplate-$(GOTEMPLATE_VERSION)

.PHONY: gen-license-header
gen-license-header: bin/gotemplate ## Generate license header used in source code files.
	GOTEMPLATE_NO_STDIN=true \
	$(BIN_DIR)/gotemplate run \
		--follow-symlinks \
		--import="$(BOILERPLATE_DIR)/vars.yml" \
		--source="$(BOILERPLATE_DIR)"


bin/mockgen: $(BIN_DIR)/mockgen-$(MOCKGEN_VERSION) ## Symlink mockgen-<version> into versionless mockgen.
	@ln -sf mockgen-$(MOCKGEN_VERSION) $(BIN_DIR)/mockgen

$(BIN_DIR)/mockgen-$(MOCKGEN_VERSION): ## Download versioned mockgen.
	@mkdir -p $(BIN_DIR)
	@GOBIN=$(BIN_DIR) go install go.uber.org/mock/mockgen@v$(MOCKGEN_VERSION)
	@mv $(BIN_DIR)/mockgen $(BIN_DIR)/mockgen-$(MOCKGEN_VERSION)

.PHONY: mock-generate
mock-generate: bin/mockgen ## Generate mocks for specified interfaces.
	$(BIN_DIR)/mockgen \
		-copyright_file $(BOILERPLATE_DIR)/header.generated.txt \
		-package mocks \
		-source pkg/scale/types.go \
		-destination controllers/tests/mocks/scale.go
	$(BIN_DIR)/mockgen \
	    -copyright_file $(BOILERPLATE_DIR)/header.generated.txt \
	    -package mocks \
		-destination pkg/resources/kafka/mocks/Client.go \
		sigs.k8s.io/controller-runtime/pkg/client Client
	$(BIN_DIR)/mockgen \
		-copyright_file $(BOILERPLATE_DIR)/header.generated.txt \
		-package mocks \
		-source pkg/kafkaclient/client.go \
		-destination pkg/resources/kafka/mocks/KafkaClient.go
