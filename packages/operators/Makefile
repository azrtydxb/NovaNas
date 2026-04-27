# NovaNas operators Makefile.
#
# Primary targets:
#   make generate    -- regenerate zz_generated.deepcopy.go via controller-gen
#   make build       -- build the operator manager binary
#   make test        -- run unit tests
#
# NovaNas defines no CRDs (see ADR 0005); there is intentionally no
# `manifests` target. The remaining api/v1alpha1 type (BlockVolume) is
# carried for an in-progress runtime refactor and never emitted as a
# CRD manifest.

GOBIN ?= $(shell go env GOPATH)/bin
CONTROLLER_GEN ?= $(GOBIN)/controller-gen

.PHONY: all
all: generate build

##@ Codegen

.PHONY: controller-gen
controller-gen: ## Install controller-gen into $GOBIN if missing.
	@if [ ! -x $(CONTROLLER_GEN) ]; then \
		echo "installing controller-gen into $(GOBIN)"; \
		GOBIN=$(GOBIN) go install sigs.k8s.io/controller-tools/cmd/controller-gen@latest; \
	fi

.PHONY: generate
generate: controller-gen ## Regenerate DeepCopy methods for the BlockVolume type.
	$(CONTROLLER_GEN) object paths="./api/v1alpha1/..."

##@ Build

.PHONY: build
build: ## Build the operator manager binary.
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/novanas-operators ./

.PHONY: test
test: ## Run unit tests.
	go test -short ./...

.PHONY: vet
vet: ## Run go vet.
	go vet ./...

##@ Docker

DOCKER ?= docker
REGISTRY ?= ghcr.io/azrtydxb
VERSION ?= latest

.PHONY: docker-build
docker-build: ## Build the operator image.
	$(DOCKER) build -t $(REGISTRY)/novanas-operators:$(VERSION) -f Dockerfile ../..
