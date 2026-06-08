#!/usr/bin/env bash
# Self-contained kind cluster lifecycle for integration testing.
#
# Creates a kind cluster with an isolated kubeconfig (never touches your
# OCP/cloud kubectl context), runs the test flow, then tears down.
#
# CI mode auto-detects from the CI env var. Override with --ci.
#
# Usage:
#   ./test/kind-lifecycle.sh              # full test with credentials
#   ./test/kind-lifecycle.sh --keep       # don't delete cluster after tests
#
# Works alongside any existing KUBECONFIG. The cluster gets its own temp
# kubeconfig file that is cleaned up on exit.
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
CLUSTER_NAME="openshell-test"
KIND_KUBECONFIG=""
KEEP_CLUSTER=false
TEST_ARGS=("kind")

for arg in "$@"; do
  case "$arg" in
    --ci)    TEST_ARGS+=("--ci") ;;
    --keep)  KEEP_CLUSTER=true ;;
    *)       TEST_ARGS+=("$arg") ;;
  esac
done

cleanup() {
  local rc=$?
  # Deregister kind gateway from openshell CLI so it doesn't leak into OCP sessions
  openshell gateway remove openshell-kind 2>/dev/null || true
  if $KEEP_CLUSTER; then
    echo ""
    echo "Cluster kept: $CLUSTER_NAME"
    echo "  KUBECONFIG=$KIND_KUBECONFIG kubectl get nodes"
    echo "  kind delete cluster --name $CLUSTER_NAME"
  else
    echo ""
    echo "=== Deleting kind cluster ==="
    kind delete cluster --name "$CLUSTER_NAME" 2>/dev/null || true
    rm -f "$KIND_KUBECONFIG"
  fi
  exit $rc
}
trap cleanup EXIT

# ── Create cluster with isolated kubeconfig ─────────────────────────

KIND_KUBECONFIG=$(mktemp /tmp/kind-${CLUSTER_NAME}-XXXXXX.kubeconfig)
export KUBECONFIG="$KIND_KUBECONFIG"

echo "=== kind cluster lifecycle ==="
echo "  Cluster:    $CLUSTER_NAME"
echo "  KUBECONFIG: $KIND_KUBECONFIG"
echo ""

if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
  echo "=== Reusing existing cluster ==="
  kind export kubeconfig --name "$CLUSTER_NAME" --kubeconfig "$KIND_KUBECONFIG"
else
  echo "=== Creating kind cluster ==="
  kind create cluster --name "$CLUSTER_NAME" --kubeconfig "$KIND_KUBECONFIG"
fi
echo ""

# ── Pre-test setup ──────────────────────────────────────────────────

kubectl create namespace openshell --dry-run=client -o yaml | kubectl apply -f - 2>/dev/null || true

# Pre-load dev sandbox image into kind (avoids pull secrets and ImagePullBackOff).
# SANDBOX_IMAGE is set by the Makefile for dev builds; CI uses the public community image.
if [[ -n "${SANDBOX_IMAGE:-}" ]]; then
  echo "  Pre-loading image: $SANDBOX_IMAGE"
  if docker image inspect "$SANDBOX_IMAGE" &>/dev/null; then
    kind load docker-image "$SANDBOX_IMAGE" --name "$CLUSTER_NAME" 2>/dev/null || true
  else
    echo "  Image not in local docker — kind will pull at sandbox creation time"
    # Fall back to pull secret for private registries
    if [[ -f "$HOME/.docker/config.json" ]]; then
      QUAY_PASS=$(python3 -c "
import json, base64, pathlib, sys
try:
    d = json.loads(pathlib.Path('$HOME/.docker/config.json').read_text())
    print(base64.b64decode(d['auths']['quay.io']['auth']).decode().split(':',1)[1])
except Exception:
    sys.exit(1)
" 2>/dev/null) || true
      if [[ -n "${QUAY_PASS:-}" ]]; then
        kubectl create secret docker-registry quay-pull \
          --docker-server=quay.io --docker-username=rcochran \
          --docker-password="$QUAY_PASS" \
          -n openshell --dry-run=client -o yaml | kubectl apply -f - 2>/dev/null || true
      fi
    fi
  fi
fi

# ── Run tests ───────────────────────────────────────────────────────

echo "=== Running: test-flow.sh ${TEST_ARGS[*]} ==="
echo ""
"$SCRIPT_DIR/test/test-flow.sh" "${TEST_ARGS[@]}"
