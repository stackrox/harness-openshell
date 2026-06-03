#!/usr/bin/env bash
# Launch a sandbox on the local Podman/Docker gateway.
#
# Uses the same providers as the OCP workflow (setup-providers.sh).
# GWS credentials are exported from the local gws CLI.
# Atlassian URL/username come from env vars (not K8s secrets).
#
# Prerequisites:
#   ./deploy-local.sh        # verify gateway running
#   ./setup-providers.sh     # register providers
#
# Usage:
#   ./sandbox-local.sh                        # interactive Claude session
#   ./sandbox-local.sh --name my-sandbox      # custom name
#   ./sandbox-local.sh --rejoin my-sandbox    # reconnect
#   ./sandbox-local.sh --no-keep              # delete after exit
set -euo pipefail

CLI="${OPENSHELL_CLI:-openshell}"
command -v "$CLI" &>/dev/null || { echo "ERROR: openshell CLI not found."; exit 1; }

# Validate we're targeting a local gateway, not the OCP one
GW="${OPENSHELL_GATEWAY:-}"
if [[ "$GW" == "ocp" ]]; then
  echo "ERROR: OPENSHELL_GATEWAY=ocp — this script is for local gateways."
  echo "  Use ./sandbox.sh for OpenShift, or: export OPENSHELL_GATEWAY=<local-gateway>"
  exit 1
fi

# ── Parse args ─────────────────────────────────────────────────────────
EXTRA=()
while [[ $# -gt 0 ]]; do
  case $1 in
    --rejoin)
      echo "Reconnecting to sandbox: $2"
      exec "$CLI" sandbox connect "$2"
      ;;
    --name|--editor) EXTRA+=("$1" "$2"); shift 2 ;;
    --no-keep) EXTRA+=("$1"); shift ;;
    *) echo "Unknown argument: $1"; exit 1 ;;
  esac
done

# ── Detect registered providers ────────────────────────────────────────
PROVIDERS=()
for name in github vertex-local atlassian; do
  if "$CLI" provider get "$name" &>/dev/null; then
    PROVIDERS+=(--provider "$name")
  fi
done

echo "=== Providers ==="
for name in github vertex-local atlassian; do
  if "$CLI" provider get "$name" &>/dev/null; then
    echo "  $name: attached"
  else
    echo "  $name: not registered (skipping)"
  fi
done

# ── Stage files for upload ─────────────────────────────────────────────
UPLOAD_ARGS=()
STAGE=""

# GWS: export decrypted credentials
echo ""
echo "=== Credentials ==="
if command -v gws &>/dev/null && gws auth status &>/dev/null; then
  STAGE=$(mktemp -d)
  mkdir -p "$STAGE/creds/gws-config"
  if gws auth export --unmasked > "$STAGE/creds/gws-config/credentials.json" 2>/dev/null; then
    GWS_DIR="${GWS_CONFIG_DIR:-$HOME/.config/gws}"
    [[ -f "$GWS_DIR/client_secret.json" ]] && cp "$GWS_DIR/client_secret.json" "$STAGE/creds/gws-config/"
    chmod 600 "$STAGE/creds/gws-config"/*
    UPLOAD_ARGS=(--upload "$STAGE/creds:/sandbox/.harness")
    echo "  GWS: exported"
  else
    echo "  GWS: export failed (skipping)"
    rm -rf "$STAGE/creds/gws-config"
  fi
else
  echo "  GWS: not authenticated (skipping)"
fi

# Atlassian non-secret config
if [[ -n "${JIRA_URL:-}" ]]; then
  STAGE="${STAGE:-$(mktemp -d)}"
  mkdir -p "$STAGE/creds"
  python3 -c "import json,sys; json.dump({'jira_url':sys.argv[1],'jira_username':sys.argv[2]},open(sys.argv[3],'w'))" \
    "$JIRA_URL" "${JIRA_USERNAME:-}" "$STAGE/creds/atlassian.json"
  UPLOAD_ARGS=(--upload "$STAGE/creds:/sandbox/.harness")
  echo "  Atlassian: $JIRA_URL"
else
  echo "  Atlassian: JIRA_URL not set (skipping)"
fi

# ── Create sandbox ─────────────────────────────────────────────────────
echo ""
echo "=== Creating sandbox ==="
exec "$CLI" sandbox create \
  --tty \
  ${PROVIDERS[@]+"${PROVIDERS[@]}"} \
  ${UPLOAD_ARGS[@]+"${UPLOAD_ARGS[@]}"} \
  ${EXTRA[@]+"${EXTRA[@]}"} \
  -- bash -c '. /sandbox/startup.sh && exec claude --bare'
