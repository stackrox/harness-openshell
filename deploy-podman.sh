#!/usr/bin/env bash
# Verify OpenShell is installed and the local gateway is running.
#
# For local development using Podman. The gateway is managed by the OS
# package manager (Homebrew on macOS, systemd on Linux) — this script
# checks it's healthy, selects it, and prints next steps.
#
# Install OpenShell:
#   curl -LsSf https://raw.githubusercontent.com/NVIDIA/OpenShell/main/install.sh | sh
#
# Usage:
#   ./deploy-podman.sh
set -euo pipefail

CLI="${OPENSHELL_CLI:-openshell}"
command -v "$CLI" &>/dev/null || {
  echo "ERROR: openshell CLI not found. Install it first:"
  echo "  curl -LsSf https://raw.githubusercontent.com/NVIDIA/OpenShell/main/install.sh | sh"
  exit 1
}

# Check container runtime
echo "=== Container Runtime ==="
if command -v podman &>/dev/null; then
  echo "  ✓ Podman: $(podman --version 2>/dev/null)"
else
  echo "  ✗ Podman not found"
  exit 1
fi

# Find and select the local gateway
echo ""
echo "=== Gateway ==="
LOCAL_GW=$("$CLI" gateway list 2>/dev/null | awk '/127\.0\.0\.1/ {gsub(/^\*/, ""); print $1; exit}')

if [[ -z "$LOCAL_GW" ]]; then
  echo "  ✗ No local gateway found"
  echo ""
  echo "  Install OpenShell (auto-registers the gateway):"
  echo "    curl -LsSf https://raw.githubusercontent.com/NVIDIA/OpenShell/main/install.sh | sh"
  exit 1
fi

"$CLI" gateway select "$LOCAL_GW" 2>/dev/null || true

if "$CLI" inference get &>/dev/null; then
  echo "  ✓ $LOCAL_GW (active, reachable)"
else
  echo "  ✗ $LOCAL_GW (not responding)"
  echo ""
  echo "  Start the gateway:"
  echo "    macOS:  brew services start openshell"
  echo "    Linux:  systemctl --user start openshell"
  exit 1
fi

echo ""
echo "Done. Next: ./setup-providers.sh && ./sandbox-podman.sh"
