## OpenShell Harness — build, push, and test
##
## Tests (CI mode auto-detects from CI env var):
##   make test              # vet + unit + bats (~2min)
##   make test-local        # local gateway integration
##   make test-kind         # kind integration (self-contained cluster)
##   make test-remote       # OCP integration (needs KUBECONFIG)
##   make test-all          # unit + all integrations
##
## Images (dev builds, tagged from git describe):
##   make dev-sandbox       # build sandbox image (native arch)
##   make dev-runner        # build runner image
##   make dev-push          # build + push both (sandbox multi-arch)
## Release images are built and pushed by .github/workflows/images.yml.

REGISTRY      ?= ghcr.io/robbycochran/harness-openshell
CONTAINER_CLI ?= podman
PLATFORM      := linux/amd64
VERSION       := $(shell git describe --tags --always 2>/dev/null || echo dev)
LDFLAGS       := -s -w -X main.version=$(VERSION)

DEV_SANDBOX_IMAGE  := $(REGISTRY):sandbox-$(VERSION)
DEV_RUNNER_IMAGE   := $(REGISTRY):runner-$(VERSION)

.PHONY: all cli cli-runner \
        vet lint test test-local test-kind test-remote test-all \
        dev-sandbox dev-runner dev-push clean help

## ── CLI ──────────────────────────────────────────────────────────────

## Build CLI + sandbox image for local dev
all: cli dev-sandbox

## Build the harness CLI binary
cli:
	CGO_ENABLED=0 go build -ldflags '$(LDFLAGS)' -o harness .
	@echo "Built: ./harness ($(VERSION))"

## ── Images ────────────────────────────────────────────────────────────

## Cross-compile harness binary for the runner image
cli-runner:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags '$(LDFLAGS)' -o build/runner/harness .
	@echo "Built: build/runner/harness ($(VERSION))"

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

## Vet + unit tests + bats (fast, ~2min, no gateway needed)
test: vet
	CGO_ENABLED=0 go test ./...
	bats test/preflight.bats

## Local gateway integration (unit tests run separately via 'make test')
test-local: cli
	./test/test-flow.sh local

## Kind: self-contained cluster lifecycle
## Builds sandbox image locally and pre-loads into kind (no registry push needed).
## Use KEEP=1 to keep the cluster after tests (for debugging).
test-kind: cli
	$(CONTAINER_CLI) build -t $(DEV_SANDBOX_IMAGE) sandbox/
	@echo ""
	SANDBOX_IMAGE=$(DEV_SANDBOX_IMAGE) CONTAINER_CLI=$(CONTAINER_CLI) ./test/kind-lifecycle.sh $(if $(KEEP),--keep)

## Remote (OCP): requires KUBECONFIG set
test-remote: cli dev-sandbox dev-runner
	@test -n "$${KUBECONFIG}" || { echo "ERROR: Set KUBECONFIG for OCP (e.g. export KUBECONFIG=infracluster/kubeconfig)"; exit 1; }
	@echo ""
	SANDBOX_IMAGE=$(DEV_SANDBOX_IMAGE) RUNNER_IMAGE=$(DEV_RUNNER_IMAGE) ./test/test-flow.sh ocp

## All: unit + local + kind + remote
test-all: test test-local test-kind test-remote

## ── Dev image builds ─────────────────────────────────────────────────

## Build dev sandbox image locally (native arch only)
dev-sandbox:
	$(CONTAINER_CLI) build -t $(DEV_SANDBOX_IMAGE) sandbox/
	@echo "Built: $(DEV_SANDBOX_IMAGE)"

## Build dev runner image locally
dev-runner: cli-runner
	$(CONTAINER_CLI) build --platform $(PLATFORM) -t $(DEV_RUNNER_IMAGE) build/runner/
	@echo "Built: $(DEV_RUNNER_IMAGE)"

## Build and push dev images (sandbox: multi-arch, runner: amd64)
dev-push: cli-runner
	@$(CONTAINER_CLI) rmi --force $(DEV_SANDBOX_IMAGE) 2>/dev/null || true
	@$(CONTAINER_CLI) manifest rm $(DEV_SANDBOX_IMAGE) 2>/dev/null || true
	$(CONTAINER_CLI) build --platform linux/amd64 --manifest $(DEV_SANDBOX_IMAGE) sandbox/
	$(CONTAINER_CLI) build --platform linux/arm64 --manifest $(DEV_SANDBOX_IMAGE) sandbox/
	$(CONTAINER_CLI) manifest push $(DEV_SANDBOX_IMAGE)
	$(CONTAINER_CLI) build --platform $(PLATFORM) -t $(DEV_RUNNER_IMAGE) build/runner/
	$(CONTAINER_CLI) push $(DEV_RUNNER_IMAGE)
	@echo "Pushed: $(DEV_SANDBOX_IMAGE) (multi-arch) $(DEV_RUNNER_IMAGE) (amd64)"

## ── Convenience targets ───────────────────────────────────────────────

## Clean built binaries
clean:
	rm -f harness build/runner/harness build/runner/openshell
	@echo "Cleaned binaries"

## Show available targets
help:
	@grep -E '^## ' Makefile | sed 's/## //'
