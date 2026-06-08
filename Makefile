## OpenShell Harness — build, push, and test
##
## CI targets (no credentials, GHA-safe):
##   make ci               # unit + bats + lint (~2min)
##   make ci-local          # ci + local gateway integration
##   make ci-kind           # ci + kind (self-contained cluster)
##
## Developer targets (credentials available):
##   make dev-test-local    # pre-commit: unit + bats + local full + ci
##   make dev-test-kind     # kind: self-contained lifecycle
##   make dev-test-remote   # OCP: needs KUBECONFIG
##   make dev-test-all      # all of the above
##
## Images:
##   make sandbox           # build + push sandbox (multi-arch)
##   make runner            # build + push runner

REGISTRY      ?= ghcr.io/robbycochran/harness-openshell
DEV_REGISTRY  ?= quay.io/rcochran/openshell
DEV_TAG       := dev-$(shell git rev-parse --short HEAD)
PLATFORM      := linux/amd64

SANDBOX_IMAGE  := $(REGISTRY):sandbox
RUNNER_IMAGE   := $(REGISTRY):runner
DEV_SANDBOX_IMAGE  := $(DEV_REGISTRY):$(DEV_TAG)-sandbox
DEV_RUNNER_IMAGE   := $(DEV_REGISTRY):$(DEV_TAG)-runner

.PHONY: cli sandbox push-sandbox cli-runner runner push-runner \
        vet lint ci ci-local ci-kind \
        dev-test-local dev-test-kind dev-test-remote dev-test-all \
        dev-sandbox dev-runner clean help

## ── CLI ──────────────────────────────────────────────────────────────

## Build the harness CLI binary
cli:
	CGO_ENABLED=0 go build -o harness .
	@echo "Built: ./harness"

## ── Images ────────────────────────────────────────────────────────────

## Sandbox image (Claude Code + mcp-atlassian + gws, multi-arch)
sandbox: sandbox/Dockerfile sandbox/startup.sh \
         sandbox/policy.yaml sandbox/CLAUDE.md sandbox/settings.json
	docker buildx build --platform linux/amd64,linux/arm64 -t $(SANDBOX_IMAGE) sandbox/ --push
	@echo "Built and pushed: $(SANDBOX_IMAGE) (multi-arch)"

push-sandbox: sandbox
	@echo "Already pushed by buildx"

## Cross-compile harness binary for the runner image
cli-runner:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o build/runner/harness .
	rm -f build/runner/openshell && cp "$$(which openshell)" build/runner/openshell
	@echo "Built: build/runner/harness + openshell"

## Runner image (harness binary + openshell CLI)
runner: cli-runner build/runner/Dockerfile
	docker build --platform $(PLATFORM) -t $(RUNNER_IMAGE) build/runner/
	@echo "Built: $(RUNNER_IMAGE)"

push-runner: runner
	docker push $(RUNNER_IMAGE)

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

## ── CI targets (no credentials, GHA-safe) ────────────────────────────

## Unit tests + bats + lint (fast, ~2min, no gateway needed)
ci: vet
	CGO_ENABLED=0 go test ./...
	bats test/preflight.bats

## CI + local gateway integration (ci mode, no credentials)
ci-local: cli ci
	./test/test-flow.sh local --ci

## CI + kind gateway integration (self-contained, isolated kubeconfig)
ci-kind: cli ci
	./test/kind-lifecycle.sh --ci

## ── Developer targets (credentials available) ────────────────────────

## Pre-commit local: unit + bats + local full + local CI
## Requires: openshell gateway running locally, provider credentials.
dev-test-local: cli ci
	@echo ""
	@echo "=== Integration: local (full) ==="
	./test/test-flow.sh local --full
	@echo ""
	@echo "=== Integration: local (ci) ==="
	./test/test-flow.sh local --ci

## Kind: unit + bats + kind full (self-contained lifecycle)
## Creates/destroys its own kind cluster. Never touches your OCP kubectl context.
## Builds sandbox image locally and pre-loads into kind (no registry push needed).
## Use KEEP=1 to keep the cluster after tests (for debugging).
dev-test-kind: cli ci
	docker build -t $(DEV_SANDBOX_IMAGE) sandbox/
	@echo ""
	SANDBOX_IMAGE=$(DEV_SANDBOX_IMAGE) ./test/kind-lifecycle.sh $(if $(KEEP),--keep)

## Remote (OCP): unit + bats + OCP full + OCP CI
## Requires: KUBECONFIG set, provider credentials.
## Builds dev images to quay.io (OCP can't pull private ghcr.io).
dev-test-remote: cli ci dev-sandbox dev-runner
	@test -n "$${KUBECONFIG}" || { echo "ERROR: Set KUBECONFIG for OCP (e.g. export KUBECONFIG=infracluster/kubeconfig)"; exit 1; }
	@echo ""
	@echo "=== Integration: OCP (full) ==="
	SANDBOX_IMAGE=$(DEV_SANDBOX_IMAGE) RUNNER_IMAGE=$(DEV_RUNNER_IMAGE) ./test/test-flow.sh ocp --full
	@echo ""
	@echo "=== Integration: OCP (ci) ==="
	RUNNER_IMAGE=$(DEV_RUNNER_IMAGE) ./test/test-flow.sh ocp --ci

## All: local + kind + remote
dev-test-all: dev-test-local dev-test-kind dev-test-remote

## ── Dev image builds ─────────────────────────────────────────────────

## Build dev sandbox image to quay.io (multi-arch)
dev-sandbox:
	docker buildx build --platform linux/amd64,linux/arm64 -t $(DEV_SANDBOX_IMAGE) sandbox/ --push
	@echo "Built and pushed: $(DEV_SANDBOX_IMAGE)"

## Build dev runner image to quay.io
dev-runner: cli-runner
	docker build --platform $(PLATFORM) -t $(DEV_RUNNER_IMAGE) build/runner/
	docker push $(DEV_RUNNER_IMAGE)
	@echo "Built and pushed: $(DEV_RUNNER_IMAGE)"

## ── Convenience targets ───────────────────────────────────────────────

## Clean built binaries
clean:
	rm -f harness build/runner/harness build/runner/openshell
	@echo "Cleaned binaries"

## Show available targets
help:
	@grep -E '^## ' Makefile | sed 's/## //'
