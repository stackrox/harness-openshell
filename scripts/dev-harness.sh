#!/usr/bin/env bash
set -euo pipefail

# Build the CLI and run Harness with the NVIDIA community base image.
#
# Set HARNESS_OS_IMAGE when a workflow needs a custom image, such as the
# StackRox image built from profiles/stackrox/image/sandbox-default.
#
# Usage:
#   ./scripts/dev-harness.sh workflow apply harness.yaml
#   ./scripts/dev-harness.sh workflow apply harness.yaml --attach
#   ./scripts/dev-harness.sh workflow apply harness.yaml --entrypoint opencode
#
# Env overrides:
#   HARNESS_OS_IMAGE=...   use a specific image or digest

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

# Build the CLI
make -C "$REPO_ROOT" cli

if [[ -n "${HARNESS_OS_IMAGE:-}" ]]; then
    echo "Image: ${HARNESS_OS_IMAGE}" >&2
fi

exec "$REPO_ROOT/harness" "$@"
