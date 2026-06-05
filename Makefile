## OpenShell Harness — build, push, and test
##
## Usage:
##   make sandbox        # build + push sandbox image (multi-arch)
##   make launcher       # build + push launcher image
##   make test           # build images + run full tests on both platforms
##   make test-podman    # build + test podman only
##   make test-ocp       # build + test OCP only

REGISTRY      ?= quay.io/rcochran/openshell
PLATFORM      := linux/amd64

SANDBOX_IMAGE  := $(REGISTRY):sandbox
LAUNCHER_IMAGE := $(REGISTRY):launcher

.PHONY: cli sandbox push-sandbox cli-launcher launcher push-launcher \
        vet lint test-unit test test-podman test-ocp test-all validate clean help

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

## Podman only (full lifecycle)
test-podman: cli sandbox push-launcher
	./test/test-flow.sh podman --full

## OCP only (full lifecycle)
test-ocp: cli sandbox push-launcher
	./test/test-flow.sh ocp --full

## All combinations: podman + ocp
test-all: cli sandbox push-launcher
	./test/test-flow.sh all --full

## Full validation: unit tests + bats + integration (podman + OCP)
## Run this before every commit.
validate: cli sandbox push-launcher
	@echo "=== Unit tests ==="
	CGO_ENABLED=0 go test ./...
	cd sandbox/launcher && go test ./...
	@echo ""
	@echo "=== Bats ==="
	bats test/preflight.bats
	@echo ""
	@echo "=== Integration: podman ==="
	./test/test-flow.sh podman --full
	@echo ""
	@echo "=== Integration: OCP ==="
	./test/test-flow.sh ocp --full

## ── Convenience targets ───────────────────────────────────────────────

## Clean built binaries
clean:
	rm -f harness sandbox/launcher/launcher
	@echo "Cleaned binaries"

## Show available targets
help:
	@grep -E '^## ' Makefile | sed 's/## //'
