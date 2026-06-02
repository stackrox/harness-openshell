#!/usr/bin/env bash
# Pre-flight check for the OpenShell sandbox environment.
# Read-only — prints the status of all prerequisites, no mutations.
#
# Usage:
#   ./sandbox-check.sh
set -euo pipefail

NAMESPACE="${OPENSHELL_NAMESPACE:-openshell}"
export OPENSHELL_GATEWAY="${GATEWAY_NAME:-ocp}"
CLI="${OPENSHELL_CLI:-openshell}"

READY=true
fail() { echo "  ✗ $*"; READY=false; }
ok()   { echo "  ✓ $*"; }
skip() { echo "  - $*"; }

echo ""
echo "╔══════════════════════════════════════════╗"
echo "║      OpenShell Sandbox Pre-flight        ║"
echo "╚══════════════════════════════════════════╝"

# ── Gateway ────────────────────────────────────────────────────────────
echo ""
echo "=== Gateway ==="
if ! command -v "$CLI" &>/dev/null; then
  fail "openshell CLI not found"
else
  if "$CLI" inference get &>/dev/null 2>&1; then
    model=$("$CLI" inference get 2>/dev/null | grep Model: | awk '{print $2}')
    provider=$("$CLI" inference get 2>/dev/null | grep Provider: | awk '{print $2}')
    ok "Reachable (inference: $model via $provider)"
  else
    fail "Gateway unreachable — run ./deploy-ocp.sh"
  fi
fi

# ── Providers ──────────────────────────────────────────────────────────
echo ""
echo "=== Providers ==="
if command -v "$CLI" &>/dev/null; then
  for name in github vertex-local atlassian; do
    if "$CLI" provider get "$name" &>/dev/null 2>&1; then
      creds=$("$CLI" provider get "$name" 2>/dev/null | grep "Credential keys:" | sed 's/.*: //')
      config=$("$CLI" provider get "$name" 2>/dev/null | grep "Config keys:" | sed 's/.*: //')
      ok "$name: registered (credentials: $creds | config: $config)"
    else
      fail "$name: not registered — run ./setup-providers.sh"
    fi
  done
fi

# ── K8s Secrets ────────────────────────────────────────────────────────
echo ""
echo "=== K8s Secrets (namespace: $NAMESPACE) ==="
if ! command -v kubectl &>/dev/null; then
  skip "kubectl not found — skipping secret checks"
else
  for secret in openshell-gws openshell-atlassian; do
    if kubectl get secret "$secret" -n "$NAMESPACE" &>/dev/null 2>&1; then
      keys=$(kubectl get secret "$secret" -n "$NAMESPACE" \
        -o jsonpath='{.data}' 2>/dev/null | python3 -c \
        "import json,sys; print(', '.join(json.load(sys.stdin).keys()))" 2>/dev/null || echo "?")
      ok "$secret: present ($keys)"
    else
      skip "$secret: not found (run ./setup-creds.sh to create)"
    fi
  done
fi

# ── Launcher RBAC ──────────────────────────────────────────────────────
echo ""
echo "=== Launcher RBAC ==="
if command -v kubectl &>/dev/null; then
  if kubectl get serviceaccount openshell-launcher -n "$NAMESPACE" &>/dev/null 2>&1; then
    ok "ServiceAccount openshell-launcher: present"
  else
    fail "ServiceAccount openshell-launcher: missing — run ./deploy-ocp.sh"
  fi
  if kubectl get role openshell-launcher -n "$NAMESPACE" &>/dev/null 2>&1; then
    ok "Role openshell-launcher: present"
  else
    fail "Role openshell-launcher: missing — run ./deploy-ocp.sh"
  fi
fi

# ── Images ─────────────────────────────────────────────────────────────
echo ""
echo "=== Images ==="
for img in "quay.io/rcochran/openshell:sandbox" "quay.io/rcochran/openshell:launcher"; do
  if docker manifest inspect "$img" &>/dev/null 2>&1; then
    ok "$img"
  else
    skip "$img: not checked (docker not available or image not yet pushed)"
  fi
done

# ── K8s Cluster ────────────────────────────────────────────────────────
echo ""
echo "=== Cluster ==="
if command -v kubectl &>/dev/null; then
  if kubectl get ns "$NAMESPACE" &>/dev/null 2>&1; then
    pods=$(kubectl get pods -n "$NAMESPACE" --no-headers 2>/dev/null | wc -l | tr -d ' ')
    ok "Namespace '$NAMESPACE' exists ($pods pods)"
  else
    fail "Namespace '$NAMESPACE' not found — run ./deploy-ocp.sh"
  fi
fi

# ── Summary ────────────────────────────────────────────────────────────
echo ""
echo "══════════════════════════════════════════════"
if $READY; then
  echo "  ✓ Ready to launch: kubectl apply -f sandbox.yaml"
else
  echo "  ✗ Not ready — fix issues above before launching"
fi
echo "══════════════════════════════════════════════"
echo ""
