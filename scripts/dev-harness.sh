#!/usr/bin/env bash
set -euo pipefail

# Build CLI and run harness with the latest CI-built sandbox image.
#
# The sandbox image is built by CI and pushed to GHCR. Local podman
# builds on macOS produce images that don't work in OpenShell's sandbox
# runtime, so we pull from the registry instead of building locally.
#
# Usage:
#   ./scripts/dev-harness.sh apply
#   ./scripts/dev-harness.sh apply --task "review this code"
#   ./scripts/dev-harness.sh apply --agent opencode
#
# Env overrides:
#   HARNESS_OS_IMAGE=...   use a specific image tag
#   CONTAINER_CLI=...      podman or docker (default: podman)

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

# Build the CLI
make -C "$REPO_ROOT" cli

# Use CI image: find the latest sandbox tag from GHCR
if [[ -z "${HARNESS_OS_IMAGE:-}" ]]; then
    LATEST_TAG=$(gh api user/packages/container/harness-openshell/versions \
        --jq '[.[].metadata.container.tags[] | select(startswith("sandbox-v"))] | first // empty' 2>/dev/null || true)

    if [[ -n "$LATEST_TAG" ]]; then
        export HARNESS_OS_IMAGE="quay.io/rcochran/openshell:${LATEST_TAG}"
    else
        echo "WARNING: could not find CI image, using version-based default" >&2
    fi
fi

if [[ -n "${HARNESS_OS_IMAGE:-}" ]]; then
    echo "Image: ${HARNESS_OS_IMAGE}" >&2
fi

exec "$REPO_ROOT/harness" "$@"
