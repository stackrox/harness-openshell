#!/usr/bin/env bash
# Store non-provider credentials as K8s Secrets in the openshell namespace.
#
# Run once after deploy-ocp.sh. Credentials are stored in the cluster and
# mounted into sandbox launcher pods — they never transit the kubectl client
# when launching sandboxes.
#
# For local mode, this script is NOT needed — sandbox-podman.sh handles
# credential staging at launch time.
#
# Usage:
#   ./setup-creds.sh           # create missing secrets
#   ./setup-creds.sh --force   # delete and recreate all secrets
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib/common.sh"

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
  TMPDIR=$(mktemp -d)
  trap 'rm -rf "$TMPDIR"' EXIT
  if export_gws_creds "$TMPDIR"; then
    ARGS=(--from-file=credentials.json="$TMPDIR/gws-config/credentials.json")
    [[ -f "$TMPDIR/gws-config/client_secret.json" ]] && \
      ARGS+=(--from-file=client_secret.json="$TMPDIR/gws-config/client_secret.json")
    kubectl create secret generic openshell-gws -n "$NAMESPACE" "${ARGS[@]}"
    echo "  openshell-gws: created"
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
  echo "  Atlassian: JIRA_URL not set (skipping)"
fi

echo ""
echo "Done. Run ./openshell-harness-preflight.sh to verify all prerequisites."
