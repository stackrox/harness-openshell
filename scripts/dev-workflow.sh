#!/usr/bin/env bash
set -euo pipefail

# Start an interactive, retained sandbox containing the current working tree.
# The retained sandbox can be reconnected after the shell exits with the
# printed `openshell sandbox connect` command.
#
# Usage:
#   ./scripts/dev-workflow.sh
#   HARNESS_DEV_NAME=my-debug ./scripts/dev-workflow.sh --gateway openshell

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
NAME="${HARNESS_DEV_NAME:-harness-openshell-dev-$(date -u +%Y%m%d-%H%M%S)-$$}"

echo "Sandbox: $NAME" >&2
echo "Reconnect: openshell sandbox connect $NAME" >&2

exec "$REPO_ROOT/scripts/dev-harness.sh" \
  workflow apply "$REPO_ROOT/dev-workflow.yaml" \
  --name "$NAME" \
  --attach \
  "$@"
