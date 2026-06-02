#!/usr/bin/env bash
# Runtime environment setup for the sandbox.
#
# Runs once at sandbox creation (sourced, not subshell). Configures
# environment and MCP servers, then writes .openshell-env so reconnects
# pick up the same environment.
#
# ── What the provider system handles (no work needed here) ─────────────
#
#   GITHUB_TOKEN                → github provider, Bearer auth
#   GOOGLE_VERTEX_AI_TOKEN      → vertex-local provider, gateway-minted
#   ANTHROPIC_VERTEX_PROJECT_ID → vertex-local provider config
#   VERTEX_LOCATION             → vertex-local provider config
#   JIRA_URL                    → atlassian provider, Basic auth (proxy
#   JIRA_USERNAME                 decodes base64, resolves placeholders,
#   JIRA_API_TOKEN                re-encodes)
#
set -euo pipefail

# ── Environment file (persists across reconnects) ──────────────────────
cat > /sandbox/.openshell-env <<'ENVEOF'
export ANTHROPIC_BASE_URL=https://inference.local
export ANTHROPIC_API_KEY=sk-ant-openshell-proxy-managed
export CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS=1
export CLAUDE_CODE_SANDBOXED=1
export GOOGLE_WORKSPACE_CLI_CONFIG_DIR=/tmp/gws-config
export GOOGLE_WORKSPACE_CLI_CREDENTIALS_FILE=/tmp/gws-config/credentials.json
export CLAUDE_DANGEROUSLY_SKIP_PERMISSIONS=1
export PATH="/sandbox/.local/bin:$PATH"
ENVEOF

grep -q openshell-env /sandbox/.bashrc 2>/dev/null || {
  echo ". ~/.openshell-env 2>/dev/null" >> /sandbox/.bashrc
}
. /sandbox/.openshell-env

# ── Copy GWS credentials if uploaded ──────────────────────────────────
if [[ -d /sandbox/.harness/creds/gws-config ]]; then
  cp /sandbox/.harness/creds/gws-config/* /tmp/gws-config/ 2>/dev/null || true
  chmod 600 /tmp/gws-config/* 2>/dev/null || true
fi

# ── Configure MCP servers ─────────────────────────────────────────────
python3 /sandbox/configure-mcp.py

echo "Setup complete."
