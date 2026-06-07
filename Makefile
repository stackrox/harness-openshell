## OpenShell Harness — build, push, and test
##
## Usage:
##   make sandbox        # build + push sandbox image (multi-arch)
##   make launcher       # build + push launcher image
##   make test           # build images + run full tests on both platforms
##   make test-podman    # build + test podman only
##   make test-ocp       # build + test OCP only

REGISTRY      ?= ghcr.io/robbycochran/harness-openshell
DEV_REGISTRY  ?= quay.io/rcochran/openshell
DEV_TAG       := dev-$(shell git rev-parse --short HEAD)
PLATFORM      := linux/amd64

SANDBOX_IMAGE  := $(REGISTRY):sandbox
LAUNCHER_IMAGE := $(REGISTRY):launcher
DEV_SANDBOX_IMAGE  := $(DEV_REGISTRY):$(DEV_TAG)-sandbox
DEV_LAUNCHER_IMAGE := $(DEV_REGISTRY):$(DEV_TAG)-launcher

.PHONY: cli sandbox push-sandbox cli-launcher launcher push-launcher \
        vet lint test-unit test test-local test-kind test-ocp test-all validate \
        validate-local validate-local-ci validate-kind validate-kind-ci \
        dev-sandbox dev-launcher validate-dev clean help

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

## ── Test targets ─────────────────────────────────────────────────────

## Unit tests only (no live gateway, fast)
test-unit:
	CGO_ENABLED=0 go test ./...
	cd sandbox/launcher && go test ./...
	bats test/preflight.bats

## Both platforms (full lifecycle, rebuilds images)
test: cli sandbox push-launcher
	./test/test-flow.sh all --full

## Local gateway (full lifecycle)
test-local: cli
	./test/test-flow.sh local --full

## kind cluster (full lifecycle — requires: kind create cluster --name openshell)
test-kind: cli
	./test/test-flow.sh kind --full

## OCP only (full lifecycle)
test-ocp: cli sandbox push-launcher
	./test/test-flow.sh ocp --full

## All combinations: local + kind + ocp
test-all: cli sandbox push-launcher
	./test/test-flow.sh all --full

## Full validation: unit tests + bats + integration (local + OCP)
## Run this before every commit.
validate: cli sandbox push-launcher
	@echo "=== Unit tests ==="
	CGO_ENABLED=0 go test ./...
	cd sandbox/launcher && go test ./...
	@echo ""
	@echo "=== Bats ==="
	bats test/preflight.bats
	@echo ""
	@echo "=== Integration: local ==="
	./test/test-flow.sh local --full
	@echo ""
	@echo "=== Integration: OCP ==="
	./test/test-flow.sh ocp --full

## Dev validation: unit tests + bats + build images + full integration matrix.
## Builds sandbox + launcher to DEV_REGISTRY (quay.io/rcochran/openshell), runs every flow.
## Requires: openshell gateway running locally, OCP cluster via KUBECONFIG.
dev-sandbox:
	docker buildx build --platform linux/amd64,linux/arm64 -t $(DEV_SANDBOX_IMAGE) sandbox/ --push
	@echo "Built and pushed: $(DEV_SANDBOX_IMAGE)"

dev-launcher: cli-launcher
	docker build --platform $(PLATFORM) -t $(DEV_LAUNCHER_IMAGE) sandbox/launcher/
	docker push $(DEV_LAUNCHER_IMAGE)
	@echo "Built and pushed: $(DEV_LAUNCHER_IMAGE)"

validate-dev: cli dev-sandbox dev-launcher
	@echo "=== Images ==="
	@echo "  SANDBOX_IMAGE:  $(DEV_SANDBOX_IMAGE)"
	@echo "  LAUNCHER_IMAGE: $(DEV_LAUNCHER_IMAGE)"
	@echo ""
	@echo "=== Unit tests ==="
	CGO_ENABLED=0 go test ./...
	cd sandbox/launcher && go test ./...
	@echo ""
	@echo "=== Bats ==="
	bats test/preflight.bats
	@echo ""
	@echo "=== Integration: local (quick) ==="
	SANDBOX_IMAGE=$(DEV_SANDBOX_IMAGE) ./test/test-flow.sh local
	@echo ""
	@echo "=== Integration: local (full) ==="
	SANDBOX_IMAGE=$(DEV_SANDBOX_IMAGE) ./test/test-flow.sh local --full
	@echo ""
	@echo "=== Integration: local CI profile (no providers) ==="
	./test/test-flow.sh local --full --no-providers --profile=ci
	@echo ""
	@echo "=== Integration: OCP (quick) ==="
	SANDBOX_IMAGE=$(DEV_SANDBOX_IMAGE) LAUNCHER_IMAGE=$(DEV_LAUNCHER_IMAGE) ./test/test-flow.sh ocp
	@echo ""
	@echo "=== Integration: OCP (full) ==="
	SANDBOX_IMAGE=$(DEV_SANDBOX_IMAGE) LAUNCHER_IMAGE=$(DEV_LAUNCHER_IMAGE) ./test/test-flow.sh ocp --full

## Default validation: unit tests + bats + full integration with user credentials.
## Requires: openshell gateway running locally (brew services start openshell),
## JIRA_API_TOKEN, gcloud ADC, gws auth login.
validate-local: cli
	@echo "=== Unit tests ==="
	CGO_ENABLED=0 go test ./...
	cd sandbox/launcher && go test ./...
	@echo ""
	@echo "=== Bats ==="
	bats test/preflight.bats
	@echo ""
	@echo "=== Integration: local gateway, default mode ==="
	./test/test-flow.sh local --full

## CI validation: unit tests + integration without credentials (local gateway).
## Requires: openshell gateway running locally only.
validate-local-ci: cli
	@echo "=== Unit tests ==="
	CGO_ENABLED=0 go test ./...
	cd sandbox/launcher && go test ./...
	@echo ""
	@echo "=== Integration: local gateway, ci mode ==="
	./test/test-flow.sh local --ci

## Default validation on kind: unit tests + full integration with user credentials.
## Requires: kind create cluster --name openshell
validate-kind: cli
	@echo "=== Unit tests ==="
	CGO_ENABLED=0 go test ./...
	cd sandbox/launcher && go test ./...
	@echo ""
	@echo "=== Integration: kind gateway, default mode ==="
	./test/test-flow.sh kind --full

## CI validation on kind: unit tests + integration without credentials.
## Requires: kind create cluster --name openshell
validate-kind-ci: cli
	@echo "=== Unit tests ==="
	CGO_ENABLED=0 go test ./...
	cd sandbox/launcher && go test ./...
	@echo ""
	@echo "=== Integration: kind gateway, ci mode ==="
	./test/test-flow.sh kind --ci

## ── Convenience targets ───────────────────────────────────────────────

## Clean built binaries
clean:
	rm -f harness sandbox/launcher/launcher
	@echo "Cleaned binaries"

## Show available targets
help:
	@grep -E '^## ' Makefile | sed 's/## //'
