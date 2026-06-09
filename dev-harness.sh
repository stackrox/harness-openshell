#!/usr/bin/env bash
set -euo pipefail

# Build CLI + dev images, then run harness with all args.
# Pushes images only when --remote is passed (OCP pulls from registry).
# Image layers are cached by podman/docker so rebuilds are fast when
# only code changes. First run pulls base layers (~2min); subsequent
# runs finish in seconds.
#
# Usage:
#   ./dev-harness.sh up                  # local: build only, no push
#   ./dev-harness.sh up --remote         # remote: build + push to registry
#   ./dev-harness.sh providers --force
#
# Env overrides:
#   REGISTRY=...        image registry (default: ghcr.io/robbycochran/harness-openshell)
#   CONTAINER_CLI=...   podman or docker (default: podman)

REPO_ROOT="$(cd "$(dirname "$0")" && pwd)"
VERSION=$(git -C "$REPO_ROOT" describe --tags --always 2>/dev/null || echo dev)
REGISTRY=${REGISTRY:-ghcr.io/robbycochran/harness-openshell}
CONTAINER_CLI=${CONTAINER_CLI:-podman}
DEV_SANDBOX_IMAGE="${REGISTRY}:sandbox-${VERSION}"
DEV_RUNNER_IMAGE="${REGISTRY}:runner-${VERSION}"

# 1. Build CLI
if ! CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=${VERSION}" \
    -o "$REPO_ROOT/harness" "$REPO_ROOT" 2>/dev/null; then
    echo "ERROR: CLI build failed" >&2
    CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=${VERSION}" \
        -o "$REPO_ROOT/harness" "$REPO_ROOT"
    exit 1
fi

# 2. Build dev sandbox image (native arch, layer-cached)
if ! $CONTAINER_CLI build -t "$DEV_SANDBOX_IMAGE" "$REPO_ROOT/sandbox/" >/dev/null 2>&1; then
    echo "ERROR: sandbox image build failed:" >&2
    $CONTAINER_CLI build -t "$DEV_SANDBOX_IMAGE" "$REPO_ROOT/sandbox/"
    exit 1
fi

# 3. Cross-compile CLI for runner image + build runner
if ! CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-s -w -X main.version=${VERSION}" \
    -o "$REPO_ROOT/build/runner/harness" "$REPO_ROOT" 2>/dev/null; then
    echo "ERROR: runner CLI build failed" >&2
    exit 1
fi
if ! $CONTAINER_CLI build --platform linux/amd64 -t "$DEV_RUNNER_IMAGE" \
    "$REPO_ROOT/build/runner/" >/dev/null 2>&1; then
    echo "ERROR: runner image build failed:" >&2
    $CONTAINER_CLI build --platform linux/amd64 -t "$DEV_RUNNER_IMAGE" "$REPO_ROOT/build/runner/"
    exit 1
fi

# 4. Push images (only when --remote is passed -- OCP pulls from registry)
NEEDS_PUSH=0
for arg in "$@"; do
    [ "$arg" = "--remote" ] && NEEDS_PUSH=1 && break
done
if [ "$NEEDS_PUSH" = "1" ]; then
    $CONTAINER_CLI push "$DEV_SANDBOX_IMAGE" >/dev/null 2>&1 || {
        echo "ERROR: sandbox image push failed" >&2
        $CONTAINER_CLI push "$DEV_SANDBOX_IMAGE"
        exit 1
    }
    $CONTAINER_CLI push "$DEV_RUNNER_IMAGE" >/dev/null 2>&1 || {
        echo "ERROR: runner image push failed" >&2
        $CONTAINER_CLI push "$DEV_RUNNER_IMAGE"
        exit 1
    }
fi

# 5. Run harness with dev images
export SANDBOX_IMAGE="$DEV_SANDBOX_IMAGE"
export RUNNER_IMAGE="$DEV_RUNNER_IMAGE"
exec "$REPO_ROOT/harness" "$@"
