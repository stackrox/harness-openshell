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
##   make launcher          # build + push launcher

REGISTRY      ?= ghcr.io/robbycochran/harness-openshell
DEV_REGISTRY  ?= quay.io/rcochran/openshell
DEV_TAG       := dev-$(shell git rev-parse --short HEAD)
PLATFORM      := linux/amd64

SANDBOX_IMAGE  := $(REGISTRY):sandbox
LAUNCHER_IMAGE := $(REGISTRY):launcher
DEV_SANDBOX_IMAGE  := $(DEV_REGISTRY):$(DEV_TAG)-sandbox
DEV_LAUNCHER_IMAGE := $(DEV_REGISTRY):$(DEV_TAG)-launcher

.PHONY: cli sandbox push-sandbox cli-launcher launcher push-launcher \
        vet lint ci ci-local ci-kind \
        dev-test-local dev-test-kind dev-test-remote dev-test-all \
        dev-sandbox dev-launcher clean help

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

## Cross-compile Go launcher binary for linux/amd64
cli-launcher:
	cd sandbox/launcher && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o launcher .
	@echo "Built: sandbox/launcher/launcher"

## Launcher image (Go binary + openshell CLI, scratch-based)
launcher: cli-launcher sandbox/launcher/Dockerfile sandbox/launcher/openshell
	docker build --platform $(PLATFORM) -t $(LAUNCHER_IMAGE) sandbox/launcher/
	@echo "Built: $(LAUNCHER_IMAGE)"

push-launcher: launcher
	docker push $(LAUNCHER_IMAGE)

## ── Lint targets ─────────────────────────────────────────────────────

## Run go vet
vet:
	go vet ./...
	cd sandbox/launcher && go vet ./...

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
	cd sandbox/launcher && go test ./...
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
## Builds dev sandbox image to quay.io (kind can't pull private ghcr.io).
## Use KEEP=1 to keep the cluster after tests (for debugging).
dev-test-kind: cli ci dev-sandbox
	@echo ""
	SANDBOX_IMAGE=$(DEV_SANDBOX_IMAGE) SANDBOX_PULL_SECRET=quay-pull ./test/kind-lifecycle.sh $(if $(KEEP),--keep)

## Remote (OCP): unit + bats + OCP full + OCP CI
## Requires: KUBECONFIG set, provider credentials.
## Builds dev images to quay.io (OCP can't pull private ghcr.io).
dev-test-remote: cli ci dev-sandbox dev-launcher
	@test -n "$${KUBECONFIG}" || { echo "ERROR: Set KUBECONFIG for OCP (e.g. export KUBECONFIG=infracluster/kubeconfig)"; exit 1; }
	@echo ""
	@echo "=== Integration: OCP (full) ==="
	SANDBOX_IMAGE=$(DEV_SANDBOX_IMAGE) LAUNCHER_IMAGE=$(DEV_LAUNCHER_IMAGE) ./test/test-flow.sh ocp --full
	@echo ""
	@echo "=== Integration: OCP (ci) ==="
	LAUNCHER_IMAGE=$(DEV_LAUNCHER_IMAGE) ./test/test-flow.sh ocp --ci

## All: local + kind + remote
dev-test-all: dev-test-local dev-test-kind dev-test-remote

## ── Dev image builds ─────────────────────────────────────────────────

## Build dev sandbox image to quay.io (multi-arch)
dev-sandbox:
	docker buildx build --platform linux/amd64,linux/arm64 -t $(DEV_SANDBOX_IMAGE) sandbox/ --push
	@echo "Built and pushed: $(DEV_SANDBOX_IMAGE)"

## Build dev launcher image to quay.io
dev-launcher: cli-launcher
	docker build --platform $(PLATFORM) -t $(DEV_LAUNCHER_IMAGE) sandbox/launcher/
	docker push $(DEV_LAUNCHER_IMAGE)
	@echo "Built and pushed: $(DEV_LAUNCHER_IMAGE)"

## ── Convenience targets ───────────────────────────────────────────────

## Clean built binaries
clean:
	rm -f harness sandbox/launcher/launcher
	@echo "Cleaned binaries"

## Show available targets
help:
	@grep -E '^## ' Makefile | sed 's/## //'
