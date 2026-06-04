#!/usr/bin/env bash
# Tear down sandboxes, providers, k8s resources, or all.
#
# Usage:
#   ./teardown.sh                # delete everything (sandboxes + providers + k8s if active gateway is remote)
#   ./teardown.sh --sandboxes    # delete sandboxes only
#   ./teardown.sh --providers    # delete providers only
#   ./teardown.sh --k8s          # delete k8s resources only (secrets, RBAC, route)
#
# Detects local vs k8s from the active gateway name.
# Safe to run multiple times — each step tolerates missing resources.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib/common.sh"

require_cli

NAMESPACE="${OPENSHELL_NAMESPACE:-openshell}"

# Detect active gateway
ACTIVE_GW=$("$CLI" gateway list 2>/dev/null | awk '/^\*/ {print $2}' || true)

# Detect k8s cluster — just check if the namespace exists
HAS_K8S=false
if command -v kubectl &>/dev/null && kubectl get ns "$NAMESPACE" &>/dev/null 2>&1; then
  HAS_K8S=true
fi

DO_SANDBOXES=false
DO_PROVIDERS=false
DO_K8S=false

if [[ $# -eq 0 ]]; then
  DO_SANDBOXES=true
  DO_PROVIDERS=true
  DO_K8S=true
fi

while [[ $# -gt 0 ]]; do
  case "$1" in
    --sandboxes) DO_SANDBOXES=true ;;
    --providers) DO_PROVIDERS=true ;;
    --k8s)       DO_K8S=true ;;
    *) echo "Usage: $0 [--sandboxes] [--providers] [--k8s]"; exit 1 ;;
  esac
  shift
done

if [[ -n "$ACTIVE_GW" ]]; then
  echo "Active gateway: $ACTIVE_GW"
else
  echo "Active gateway: none"
fi
echo ""

# ── Sandboxes ─────────────────────────────────────────────────────────
if $DO_SANDBOXES; then
  echo "=== Sandboxes ==="
  if [[ -z "$ACTIVE_GW" ]]; then
    echo "  No active gateway, skipping"
  else
    sandboxes=$("$CLI" sandbox list 2>/dev/null | awk 'NR>1 && NF {print $1}' || true)
    if [[ -z "${sandboxes// /}" ]]; then
      echo "  None running"
    else
      while read -r name; do
        echo "  Deleting $name"
        "$CLI" sandbox delete "$name" 2>/dev/null || true
      done <<< "$sandboxes"
    fi
  fi
  echo ""
fi

# ── Providers ─────────────────────────────────────────────────────────
if $DO_PROVIDERS; then
  echo "=== Providers ==="
  if [[ -z "$ACTIVE_GW" ]]; then
    echo "  No active gateway, skipping"
  else
    remaining=$("$CLI" sandbox list 2>/dev/null | awk 'NR>1 && NF {print $1}' || true)
    if [[ -n "${remaining// /}" ]]; then
      echo "ERROR: Cannot delete providers with running sandboxes."
      echo "  Run: ./teardown.sh --sandboxes"
      exit 1
    fi

    providers=$("$CLI" provider list 2>/dev/null | awk 'NR>1 && NF {print $1}' || true)
    if [[ -z "${providers// /}" ]]; then
      echo "  None registered"
    else
      while read -r name; do
        echo "  Deleting $name"
        "$CLI" provider delete "$name" 2>/dev/null || true
      done <<< "$providers"
    fi

    echo ""
    echo "=== Inference ==="
    "$CLI" inference remove 2>/dev/null && echo "  Cleared" || echo "  Already cleared"
  fi
  echo ""
fi

# ── K8s resources ─────────────────────────────────────────────────────
if $DO_K8S; then
  if ! $HAS_K8S; then
    echo "=== K8s ==="
    echo "  No openshell namespace found, skipping"
  else
    echo "=== Helm release ==="
    helm uninstall openshell -n "$NAMESPACE" 2>/dev/null && \
      echo "  Uninstalled" || echo "  Not installed"

    echo ""
    echo "=== Sandbox CRD ==="
    kubectl delete ns agent-sandbox-system 2>/dev/null && \
      echo "  Deleted agent-sandbox-system" || echo "  Not found"

    echo ""
    echo "=== OpenShift SCCs ==="
    for sa in openshell openshell-sandbox default; do
      oc adm policy remove-scc-from-user privileged -z "$sa" -n "$NAMESPACE" 2>/dev/null || true
    done
    oc adm policy remove-scc-from-user anyuid -z openshell -n "$NAMESPACE" 2>/dev/null || true
    kubectl delete clusterrolebinding agent-sandbox-admin 2>/dev/null || true
    echo "  Cleared"

    echo ""
    echo "=== K8s secrets ==="
    for secret in openshell-gws openshell-atlassian; do
      kubectl delete secret "$secret" -n "$NAMESPACE" 2>/dev/null && \
        echo "  Deleted $secret" || echo "  $secret: not found"
    done

    echo ""
    echo "=== Namespace ==="
    kubectl delete ns "$NAMESPACE" 2>/dev/null && \
      echo "  Deleted $NAMESPACE" || echo "  $NAMESPACE: not found"

    echo ""
    echo "=== Gateway config ==="
    # Remove all non-local gateways (covers legacy 'ocp' and current 'openshell-remote-ocp')
    for gw in $("$CLI" gateway list 2>/dev/null | awk 'NR>1 && !/127\.0\.0\.1/ {gsub(/^\*/, ""); print $1}'); do
      "$CLI" gateway remove "$gw" 2>/dev/null && \
        echo "  Removed gateway '$gw'" || true
    done
    # Select local gateway if available
    local_gw=$("$CLI" gateway list 2>/dev/null | awk '/127\.0\.0\.1/ {gsub(/^\*/, ""); print $1; exit}')
    [[ -n "$local_gw" ]] && "$CLI" gateway select "$local_gw" 2>/dev/null || true

    echo ""
    echo "=== Local files ==="
    rm -f "$SCRIPT_DIR/.gateway-env" && echo "  Removed .gateway-env" || true
    rm -rf "$SCRIPT_DIR/.certs" && echo "  Removed .certs/" || true
    echo ""
  fi
fi

echo "Done."
