#!/usr/bin/env bash
set -euo pipefail

# Build CLI + dev images via make, then run harness with all args.
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

if [[ " $* " == *" --remote "* ]]; then
    make -C "$REPO_ROOT" cli dev-push
else
    make -C "$REPO_ROOT" cli dev-sandbox dev-runner
fi

export SANDBOX_IMAGE="${REGISTRY}:sandbox-${VERSION}"
export RUNNER_IMAGE="${REGISTRY}:runner-${VERSION}"
exec "$REPO_ROOT/harness" "$@"
