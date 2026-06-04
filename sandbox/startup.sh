#!/usr/bin/env bash
# Runtime setup for the sandbox. Runs once via launcher's sandbox exec.
set -euo pipefail

# ── Source env vars from agent config ─────────────────────────────────
OPENSHELL_DIR="/sandbox/.config/openshell"
if [[ -f "$OPENSHELL_DIR/sandbox.env" ]]; then
  . "$OPENSHELL_DIR/sandbox.env"
  cat "$OPENSHELL_DIR/sandbox.env" >> /sandbox/.bashrc
fi

# ── Git auth ──────────────────────────────────────────────────────────
gh auth setup-git 2>/dev/null || true

echo "Setup complete."
