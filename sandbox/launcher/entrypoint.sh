#!/usr/bin/env bash
# In-cluster sandbox launcher.
#
# Runs as a Kubernetes Job. Reads sandbox configuration from environment
# variables (sourced from the ConfigMap via envFrom) and credentials from
# volume-mounted Secrets. Calls openshell sandbox create in-cluster.
#
# Environment (from ConfigMap):
#   SANDBOX_NAME      — sandbox name
#   SANDBOX_KEEP      — "true" to keep sandbox after exit (default: "true")
#   SANDBOX_PROVIDERS — space-separated provider names (default: "github vertex-local atlassian")
#   SANDBOX_COMMAND   — command to run (default: "claude --bare")
#
# Secrets mounted at:
#   /secrets/gws/credentials.json    — decrypted GWS OAuth credentials
#   /secrets/gws/client_secret.json  — GWS OAuth client config
#   /secrets/atlassian/JIRA_URL      — Atlassian site URL
#   /secrets/atlassian/JIRA_USERNAME — Atlassian username
#
# Provider credentials (GITHUB_TOKEN, JIRA_API_TOKEN, etc.) are managed
# by the OpenShell gateway provider system — they never appear here.
set -euo pipefail

GATEWAY_ENDPOINT="${GATEWAY_ENDPOINT:-https://openshell.openshell.svc.cluster.local:8080}"
CLI="${OPENSHELL_CLI:-openshell}"
export OPENSHELL_GATEWAY_ENDPOINT="$GATEWAY_ENDPOINT"
export OPENSHELL_GATEWAY_INSECURE="${OPENSHELL_GATEWAY_INSECURE:-true}"

SANDBOX_NAME="${SANDBOX_NAME:-agent}"
SANDBOX_KEEP="${SANDBOX_KEEP:-true}"
SANDBOX_PROVIDERS="${SANDBOX_PROVIDERS:-github vertex-local atlassian}"
SANDBOX_COMMAND="${SANDBOX_COMMAND:-claude --bare}"

echo "=== Sandbox Launcher ==="
echo "  Name:      $SANDBOX_NAME"
echo "  Providers: $SANDBOX_PROVIDERS"
echo "  Gateway:   $GATEWAY_ENDPOINT"
echo ""

# ── Build provider flags ───────────────────────────────────────────────
PROVIDER_FLAGS=()
for name in $SANDBOX_PROVIDERS; do
  if "$CLI" provider get "$name" &>/dev/null; then
    PROVIDER_FLAGS+=(--provider "$name")
    echo "  Provider $name: attached"
  else
    echo "  Provider $name: not registered (skipping)"
  fi
done

# ── Stage credentials from mounted secrets ─────────────────────────────
STAGE=$(mktemp -d)
CREDS="$STAGE/creds"
mkdir -p "$CREDS"
HAS_UPLOADS=false

# GWS credentials (mounted from openshell-gws secret)
if [[ -f /secrets/gws/credentials.json ]]; then
  mkdir -p "$CREDS/gws-config"
  cp /secrets/gws/credentials.json "$CREDS/gws-config/"
  [[ -f /secrets/gws/client_secret.json ]] && cp /secrets/gws/client_secret.json "$CREDS/gws-config/"
  echo "  GWS credentials: mounted"
  HAS_UPLOADS=true
else
  echo "  GWS: not mounted (skipping)"
fi

# Atlassian non-secret config (mounted from openshell-atlassian secret)
if [[ -f /secrets/atlassian/JIRA_URL ]]; then
  JIRA_URL=$(cat /secrets/atlassian/JIRA_URL)
  JIRA_USERNAME=$(cat /secrets/atlassian/JIRA_USERNAME 2>/dev/null || echo "")
  python3 -c "
import json, sys
with open(sys.argv[1], 'w') as f:
    json.dump({'jira_url': sys.argv[2], 'jira_username': sys.argv[3]}, f)
" "$CREDS/atlassian.json" "$JIRA_URL" "$JIRA_USERNAME"
  echo "  Atlassian config: $JIRA_URL"
  HAS_UPLOADS=true
else
  echo "  Atlassian: not mounted (skipping)"
fi

UPLOAD_ARGS=()
$HAS_UPLOADS && UPLOAD_ARGS=(--upload "$CREDS:/sandbox/.harness")

# ── Keep flag ──────────────────────────────────────────────────────────
KEEP_ARGS=()
[[ "$SANDBOX_KEEP" != "true" ]] && KEEP_ARGS=(--no-keep)

# ── Create sandbox ─────────────────────────────────────────────────────
echo ""
echo "=== Creating sandbox ==="
exec "$CLI" sandbox create \
  --name "$SANDBOX_NAME" \
  --tty \
  ${PROVIDER_FLAGS[@]+"${PROVIDER_FLAGS[@]}"} \
  ${UPLOAD_ARGS[@]+"${UPLOAD_ARGS[@]}"} \
  ${KEEP_ARGS[@]+"${KEEP_ARGS[@]}"} \
  -- bash -c ". /sandbox/startup.sh && exec $SANDBOX_COMMAND"
