#!/usr/bin/env bash
# Store non-provider credentials as K8s Secrets in the openshell namespace.
#
# Run once after deploy-ocp.sh. Credentials are stored in the cluster and
# mounted into sandbox launcher pods — they never transit the kubectl client
# when launching sandboxes.
#
# Manages:
#   openshell-gws       — decrypted GWS OAuth credentials (gws CLI)
#   openshell-atlassian — Atlassian site URL and username (non-secrets)
#
# JIRA_API_TOKEN, GITHUB_TOKEN, and GCP ADC secrets are managed separately
# by the OpenShell provider system (setup-providers.sh) and are NOT stored
# in K8s secrets.
#
# Prerequisites:
#   - Namespace 'openshell' exists (./deploy-ocp.sh)
#   - gws auth login completed (for GWS)
#   - JIRA_URL, JIRA_USERNAME set in environment (for Atlassian)
#
# Usage:
#   ./setup-creds.sh           # create missing secrets
#   ./setup-creds.sh --force   # delete and recreate all secrets
set -euo pipefail

NAMESPACE="${OPENSHELL_NAMESPACE:-openshell}"
FORCE=false
[[ "${1:-}" == "--force" ]] && FORCE=true

command -v kubectl &>/dev/null || { echo "ERROR: kubectl is required."; exit 1; }

kubectl get ns "$NAMESPACE" &>/dev/null || {
  echo "ERROR: Namespace '$NAMESPACE' not found. Run ./deploy-ocp.sh first."
  exit 1
}

secret_exists() {
  kubectl get secret "$1" -n "$NAMESPACE" &>/dev/null
}

echo "=== Setting up cluster credentials ==="
echo "  Namespace: $NAMESPACE"
echo ""

# ── GWS credentials ────────────────────────────────────────────────────
echo "=== GWS ==="
if $FORCE && secret_exists openshell-gws; then
  kubectl delete secret openshell-gws -n "$NAMESPACE"
  echo "  Deleted existing secret."
fi

if secret_exists openshell-gws; then
  echo "  openshell-gws: exists (use --force to recreate)"
else
  command -v gws &>/dev/null || { echo "  gws CLI not found — skipping GWS"; }
  if command -v gws &>/dev/null; then
    if ! gws auth status &>/dev/null; then
      echo "  GWS not authenticated — run 'gws auth login' then re-run this script"
    else
      TMPDIR=$(mktemp -d)
      trap 'rm -rf "$TMPDIR"' EXIT
      gws auth export --unmasked > "$TMPDIR/credentials.json" 2>/dev/null || {
        echo "  ERROR: gws export failed"; exit 1
      }
      GWS_DIR="${GWS_CONFIG_DIR:-$HOME/.config/gws}"
      CLIENT_SECRET_ARG=""
      if [[ -f "$GWS_DIR/client_secret.json" ]]; then
        cp "$GWS_DIR/client_secret.json" "$TMPDIR/"
        CLIENT_SECRET_ARG="--from-file=client_secret.json=$TMPDIR/client_secret.json"
      fi
      kubectl create secret generic openshell-gws -n "$NAMESPACE" \
        --from-file=credentials.json="$TMPDIR/credentials.json" \
        ${CLIENT_SECRET_ARG:+"$CLIENT_SECRET_ARG"}
      echo "  openshell-gws: created"
    fi
  fi
fi

# ── Atlassian non-secret config ────────────────────────────────────────
echo ""
echo "=== Atlassian ==="
if $FORCE && secret_exists openshell-atlassian; then
  kubectl delete secret openshell-atlassian -n "$NAMESPACE"
  echo "  Deleted existing secret."
fi

if secret_exists openshell-atlassian; then
  echo "  openshell-atlassian: exists (use --force to recreate)"
elif [[ -n "${JIRA_URL:-}" ]]; then
  kubectl create secret generic openshell-atlassian -n "$NAMESPACE" \
    --from-literal=JIRA_URL="$JIRA_URL" \
    --from-literal=JIRA_USERNAME="${JIRA_USERNAME:-}"
  echo "  openshell-atlassian: created ($JIRA_URL)"
else
  echo "  openshell-atlassian: skipped (JIRA_URL not set)"
fi

echo ""
echo "=== Secrets in $NAMESPACE ==="
kubectl get secrets -n "$NAMESPACE" \
  --field-selector type=Opaque \
  -o custom-columns="NAME:.metadata.name,KEYS:.data" 2>/dev/null | \
  grep -E "openshell-gws|openshell-atlassian|NAME" || echo "  (none created yet)"

echo ""
echo "Done. Run ./sandbox-check.sh to verify all prerequisites."
