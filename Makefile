## OpenShell Harness — build, push, and test
##
## Tests (CI mode auto-detects from CI env var):
##   make test              # vet + unit tests
##   make test-local        # local gateway integration
##   make test-kind         # kind integration (self-contained cluster)
##   make test-remote       # OCP integration (needs KUBECONFIG)
##   make test-all          # unit + all integrations
##
## Images (dev builds, tagged from git describe):
##   make dev-sandbox       # build sandbox image (native arch)
##   make dev-push          # build + push sandbox image (multi-arch)
## Release images are built and pushed by .github/workflows/images.yml.

REGISTRY      ?= ghcr.io/robbycochran/harness-openshell
CONTAINER_CLI ?= podman
PLATFORM      := linux/amd64
VERSION       := $(shell git describe --tags --always 2>/dev/null || echo dev)
LDFLAGS       := -s -w -X main.version=$(VERSION)

IMAGE  := $(REGISTRY):sandbox-$(VERSION)

.PHONY: all cli \
        vet lint test test-local test-kind test-remote test-all \
        dev-sandbox dev-push tag clean help

## ── CLI ──────────────────────────────────────────────────────────────

## Build CLI + sandbox image for local dev
all: cli dev-sandbox

## Build the harness CLI binary
cli:
	CGO_ENABLED=0 go build -ldflags '$(LDFLAGS)' -o harness .
	@echo "Built: ./harness ($(VERSION))"

## ── Lint targets ─────────────────────────────────────────────────────

## Run go vet
vet:
	go vet ./...

## Run golangci-lint (install: https://golangci-lint.run/usage/install/)
lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed, running go vet instead"; \
		$(MAKE) vet; \
	fi

## ── Test targets ──────────────────────────────────────────────────────
## CI mode auto-detects from the CI env var (set by GitHub Actions).
## Locally: full tests with credentials. On GHA: no-credential mode.

## Vet + unit tests
test: vet
	CGO_ENABLED=0 go test ./...

## Config test suite (no gateway needed for most tests)
test-suite: cli
	./test/suite/run.sh

## Config test suite with live sandbox tests
test-suite-live: cli
	./test/suite/run.sh --live

## Local gateway integration (unit tests run separately via 'make test')
test-local: cli
	./test/test-flow.sh local-container

## Kind: self-contained cluster lifecycle
## Builds sandbox image locally and pre-loads into kind (no registry push needed).
## Use KEEP=1 to keep the cluster after tests (for debugging).
test-kind: cli
	$(CONTAINER_CLI) build -t $(IMAGE) profiles/images/sandbox-default/
	@echo ""
	HARNESS_OS_IMAGE=$(IMAGE) CONTAINER_CLI=$(CONTAINER_CLI) ./test/kind-lifecycle.sh $(if $(KEEP),--keep)

## Remote (OCP): requires KUBECONFIG set
test-remote: cli dev-sandbox
	@test -n "$${KUBECONFIG}" || { echo "ERROR: Set KUBECONFIG for OCP (e.g. export KUBECONFIG=infracluster/kubeconfig)"; exit 1; }
	@echo ""
	HARNESS_OS_IMAGE=$(IMAGE) ./test/test-flow.sh openshift

## All: unit + local + kind + remote
test-all: test test-local test-kind test-remote

## ── Dev image builds ─────────────────────────────────────────────────

## Build dev sandbox image locally (native arch only)
dev-sandbox:
	$(CONTAINER_CLI) build -t $(IMAGE) profiles/images/sandbox-default/
	@echo "Built: $(IMAGE)"

## Build and push dev sandbox image (multi-arch)
dev-push:
	@$(CONTAINER_CLI) rmi --force $(IMAGE) 2>/dev/null || true
	@$(CONTAINER_CLI) manifest rm $(IMAGE) 2>/dev/null || true
	$(CONTAINER_CLI) build --platform linux/amd64 --manifest $(IMAGE) profiles/images/sandbox-default/
	$(CONTAINER_CLI) build --platform linux/arm64 --manifest $(IMAGE) profiles/images/sandbox-default/
	$(CONTAINER_CLI) manifest push $(IMAGE)
	@echo "Pushed: $(IMAGE) (multi-arch)"

## ── Convenience targets ───────────────────────────────────────────────

## Show the current version tag (from git describe)
tag:
	@echo $(VERSION)

## Clean built binaries
clean:
	rm -f harness
	@echo "Cleaned binaries"

## Show available targets
help:
	@grep -E '^## ' Makefile | sed 's/## //'
