## OpenShell Harness — build and push images
##
## Usage:
##   make              # build and push all images
##   make sandbox      # build + push sandbox image only
##   make launcher     # build + push launcher image only
##   make gateway      # build + push gateway + supervisor images
##   make images       # build all images (no push)
##   make push         # push all images

REGISTRY      ?= quay.io/rcochran/openshell
OPENSHELL_REPO ?= $(shell cd ../../nvidia/OpenShell 2>/dev/null && pwd || echo ../OpenShell)
PLATFORM      := linux/amd64

SANDBOX_IMAGE  := $(REGISTRY):sandbox
LAUNCHER_IMAGE := $(REGISTRY):launcher
GATEWAY_IMAGE  := $(REGISTRY):gateway
SUPERVISOR_IMAGE := $(REGISTRY):supervisor

.PHONY: all images push sandbox launcher gateway supervisor \
        cli cli-gateway cli-supervisor cli-launcher \
        clean help

all: images push

## ── Images ────────────────────────────────────────────────────────────

images: sandbox launcher gateway supervisor

push: push-sandbox push-launcher push-gateway push-supervisor

## Sandbox image (Claude Code + mcp-atlassian + gws)
sandbox: sandbox/Dockerfile sandbox/startup.sh \
         sandbox/policy.yaml sandbox/CLAUDE.md sandbox/settings.json
	docker buildx build --platform linux/amd64,linux/arm64 -t $(SANDBOX_IMAGE) sandbox/ --push
	@echo "Built and pushed: $(SANDBOX_IMAGE) (multi-arch)"

push-sandbox: sandbox
	@echo "Already pushed by buildx"

## Launcher image (openshell CLI — requires cli-launcher first)
launcher: sandbox/launcher/Dockerfile sandbox/launcher/entrypoint.sh \
          sandbox/launcher/openshell
	docker build --platform $(PLATFORM) -t $(LAUNCHER_IMAGE) sandbox/launcher/
	@echo "Built: $(LAUNCHER_IMAGE)"

push-launcher: launcher
	docker push $(LAUNCHER_IMAGE)

## Gateway image (requires cli-gateway first)
gateway: $(OPENSHELL_REPO)/deploy/docker/.build/prebuilt-binaries/amd64/openshell-gateway \
         $(OPENSHELL_REPO)/deploy/docker/Dockerfile.gateway
	docker build --platform $(PLATFORM) \
	  -t $(GATEWAY_IMAGE) \
	  -f $(OPENSHELL_REPO)/deploy/docker/Dockerfile.gateway \
	  $(OPENSHELL_REPO)
	@echo "Built: $(GATEWAY_IMAGE)"

push-gateway: gateway
	docker push $(GATEWAY_IMAGE)

## Supervisor image (requires cli-supervisor first)
supervisor: $(OPENSHELL_REPO)/deploy/docker/.build/prebuilt-binaries/amd64/openshell-sandbox \
            $(OPENSHELL_REPO)/deploy/docker/Dockerfile.supervisor
	docker build --platform $(PLATFORM) \
	  -t $(SUPERVISOR_IMAGE) \
	  -f $(OPENSHELL_REPO)/deploy/docker/Dockerfile.supervisor \
	  $(OPENSHELL_REPO)
	@echo "Built: $(SUPERVISOR_IMAGE)"

push-supervisor: supervisor
	docker push $(SUPERVISOR_IMAGE)

## ── CLI builds ────────────────────────────────────────────────────────

## Build the openshell CLI for the local machine (macOS arm64)
cli:
	cd $(OPENSHELL_REPO) && cargo build --release -p openshell-cli
	@echo "Built: $(OPENSHELL_REPO)/target/release/openshell"

## Cross-compile the openshell CLI for linux/amd64 (launcher image)
cli-launcher: $(OPENSHELL_REPO)/target/x86_64-unknown-linux-gnu/release/openshell
	cp $(OPENSHELL_REPO)/target/x86_64-unknown-linux-gnu/release/openshell \
	   sandbox/launcher/openshell
	@echo "Staged: sandbox/launcher/openshell"

$(OPENSHELL_REPO)/target/x86_64-unknown-linux-gnu/release/openshell:
	cd $(OPENSHELL_REPO) && cargo zigbuild --release \
	  --target x86_64-unknown-linux-gnu \
	  -p openshell-cli \
	  --features openshell-prover/bundled-z3
	@echo "Cross-compiled: openshell CLI (linux/amd64)"

## Cross-compile the gateway for linux/amd64
cli-gateway: $(OPENSHELL_REPO)/deploy/docker/.build/prebuilt-binaries/amd64/openshell-gateway

$(OPENSHELL_REPO)/deploy/docker/.build/prebuilt-binaries/amd64/openshell-gateway: \
    $(OPENSHELL_REPO)/target/x86_64-unknown-linux-gnu/release/openshell-gateway
	mkdir -p $(OPENSHELL_REPO)/deploy/docker/.build/prebuilt-binaries/amd64
	cp $(OPENSHELL_REPO)/target/x86_64-unknown-linux-gnu/release/openshell-gateway \
	   $(OPENSHELL_REPO)/deploy/docker/.build/prebuilt-binaries/amd64/openshell-gateway
	@echo "Staged: gateway binary"

$(OPENSHELL_REPO)/target/x86_64-unknown-linux-gnu/release/openshell-gateway:
	cd $(OPENSHELL_REPO) && cargo zigbuild --release \
	  --target x86_64-unknown-linux-gnu \
	  -p openshell-server \
	  --features bundled-z3
	@echo "Cross-compiled: openshell-gateway (linux/amd64)"

## Cross-compile the supervisor for linux/amd64 (musl for scratch image)
cli-supervisor: $(OPENSHELL_REPO)/deploy/docker/.build/prebuilt-binaries/amd64/openshell-sandbox

$(OPENSHELL_REPO)/deploy/docker/.build/prebuilt-binaries/amd64/openshell-sandbox: \
    $(OPENSHELL_REPO)/target/x86_64-unknown-linux-musl/release/openshell-sandbox
	mkdir -p $(OPENSHELL_REPO)/deploy/docker/.build/prebuilt-binaries/amd64
	cp $(OPENSHELL_REPO)/target/x86_64-unknown-linux-musl/release/openshell-sandbox \
	   $(OPENSHELL_REPO)/deploy/docker/.build/prebuilt-binaries/amd64/openshell-sandbox
	@echo "Staged: supervisor binary"

$(OPENSHELL_REPO)/target/x86_64-unknown-linux-musl/release/openshell-sandbox:
	cd $(OPENSHELL_REPO) && cargo zigbuild --release \
	  --target x86_64-unknown-linux-musl \
	  -p openshell-sandbox
	@echo "Cross-compiled: openshell-sandbox (linux/amd64 musl)"

## ── Convenience targets ───────────────────────────────────────────────

## Build everything from source (slow — cross-compiles all binaries)
build-all: cli cli-launcher cli-gateway cli-supervisor images

## Clean staged binaries (does not delete Docker images or Rust build cache)
clean:
	rm -f sandbox/launcher/openshell
	@echo "Cleaned staged binaries"

## Show available targets
help:
	@grep -E '^## ' Makefile | sed 's/## //'
