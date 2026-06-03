#!/usr/bin/env bash
# Verify OpenShell is installed and the local gateway is running.
#
# For local development using Podman or Docker. The gateway is managed
# by the OS package manager (Homebrew on macOS, systemd on Linux) —
# this script just checks it's healthy and prints next steps.
#
# Install OpenShell:
#   curl -LsSf https://raw.githubusercontent.com/NVIDIA/OpenShell/main/install.sh | sh
#
# Usage:
#   ./deploy-local.sh
set -euo pipefail

CLI="${OPENSHELL_CLI:-openshell}"
command -v "$CLI" &>/dev/null || {
  echo "ERROR: openshell CLI not found. Install it first:"
  echo "  curl -LsSf https://raw.githubusercontent.com/NVIDIA/OpenShell/main/install.sh | sh"
  exit 1
}

echo "=== Local Gateway ==="

# Check if a gateway is registered and reachable
if "$CLI" gateway list &>/dev/null 2>&1 && "$CLI" inference get &>/dev/null 2>&1; then
  echo "  Status: running"
  "$CLI" gateway list 2>/dev/null | head -5
elif "$CLI" gateway list &>/dev/null 2>&1; then
  echo "  Status: registered but not responding"
  echo ""
  echo "  Start the gateway:"
  echo "    macOS:  brew services start openshell-gateway"
  echo "    Linux:  systemctl --user start openshell-gateway"
  exit 1
else
  echo "  Status: no gateway registered"
  echo ""
  echo "  Install OpenShell (auto-starts the gateway):"
  echo "    curl -LsSf https://raw.githubusercontent.com/NVIDIA/OpenShell/main/install.sh | sh"
  exit 1
fi

# Check container runtime
echo ""
echo "=== Container Runtime ==="
if command -v podman &>/dev/null; then
  echo "  Podman: $(podman --version 2>/dev/null)"
  podman info --format '{{.Host.RemoteSocket.Path}}' 2>/dev/null && echo "" || echo "  WARNING: podman socket not accessible"
elif command -v docker &>/dev/null; then
  echo "  Docker: $(docker --version 2>/dev/null)"
else
  echo "  ERROR: No container runtime found (need podman or docker)"
  exit 1
fi

echo ""
echo "════════════════════════════════════════════════════"
echo "  Local gateway is ready!"
echo "════════════════════════════════════════════════════"
echo ""
echo "Next steps:"
echo ""
echo "  1. Register providers (one-time per gateway):"
echo "     ./setup-providers.sh"
echo ""
echo "  2. Launch a sandbox:"
echo "     ./sandbox-local.sh"
echo ""
