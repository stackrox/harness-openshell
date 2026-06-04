#!/usr/bin/env bash
# Runtime setup for the sandbox. Runs once via launcher's sandbox exec.
set -euo pipefail

OPENSHELL_DIR="/sandbox/.config/openshell"

# ── Source env vars from agent config ─────────────────────────────────
if [[ -f "$OPENSHELL_DIR/sandbox.env" ]]; then
  . "$OPENSHELL_DIR/sandbox.env"
  cat "$OPENSHELL_DIR/sandbox.env" >> /sandbox/.bashrc
fi

# ── Claude Code config (MCP servers, onboarding) ─────────────────────
cat > /sandbox/.claude.json <<'CLAUDEJSON'
{
  "autoUpdates": false,
  "hasCompletedOnboarding": true,
  "projects": {
    "/sandbox": {
      "hasTrustDialogAccepted": true
    }
  },
  "mcpServers": {
    "atlassian": {
      "type": "stdio",
      "command": "/sandbox/.venv/bin/mcp-atlassian",
      "args": [],
      "env": {
        "READ_ONLY_MODE": "true"
      }
    }
  }
}
CLAUDEJSON

# ── Git auth ──────────────────────────────────────────────────────────
gh auth setup-git 2>/dev/null || true

echo "Setup complete."
