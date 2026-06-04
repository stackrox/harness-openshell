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
        test test-podman test-ocp clean help

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

## ── Test targets ─────────────────────────────────────────────────────

## Build + push sandbox and launcher, then run full tests on both platforms
test: sandbox push-launcher
	./test/test-flow.sh all --full

## Build + push sandbox and launcher, then run full podman test
test-podman: sandbox push-launcher
	./test/test-flow.sh podman --full

## Build + push sandbox and launcher, then run full OCP test
test-ocp: sandbox push-launcher
	./test/test-flow.sh ocp --full

## ── Convenience targets ───────────────────────────────────────────────

## Clean built binaries
clean:
	rm -f harness sandbox/launcher/openshell sandbox/launcher/launcher
	@echo "Cleaned binaries"

## Show available targets
help:
	@grep -E '^## ' Makefile | sed 's/## //'
