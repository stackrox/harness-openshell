#!/usr/bin/env bash
# Pre-flight check for the OpenShell sandbox environment.
# Read-only — prints the status of all prerequisites, no mutations.
#
# Checks: env vars, paths, gateway, registered providers, platform.
# All driven by providers.toml (definitions) + openshell.toml (config).
#
# Usage:
#   ./openshell-harness-preflight.sh             # best-effort report
#   ./openshell-harness-preflight.sh --strict    # exit 1 if required providers missing
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

echo ""
echo "╔══════════════════════════════════════════╗"
echo "║      OpenShell Sandbox Pre-flight        ║"
echo "╚══════════════════════════════════════════╝"
echo ""

python3 "$SCRIPT_DIR/lib/providers.py" check "$@"
