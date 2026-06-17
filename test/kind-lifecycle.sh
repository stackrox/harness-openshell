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
TEST_ARGS=("helm")

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

# Pre-load dev sandbox image into kind.
# In CI (CI=true), images.yml has already pushed the image to the registry so
# Pre-load the sandbox image into kind from the local container daemon
# so tests work without a registry push.
CONTAINER_CLI=${CONTAINER_CLI:-podman}
if [[ -n "${HARNESS_OS_IMAGE:-}" ]]; then
  if "$CONTAINER_CLI" image inspect "$HARNESS_OS_IMAGE" &>/dev/null; then
    echo "  Pre-loading image: $HARNESS_OS_IMAGE"
    if [[ "$CONTAINER_CLI" == "docker" ]]; then
      kind load docker-image "$HARNESS_OS_IMAGE" --name "$CLUSTER_NAME"
    else
      IMAGE_ARCHIVE=$(mktemp /tmp/kind-sandbox-image-XXXXXX.tar)
      "$CONTAINER_CLI" save "$HARNESS_OS_IMAGE" -o "$IMAGE_ARCHIVE"
      kind load image-archive "$IMAGE_ARCHIVE" --name "$CLUSTER_NAME"
      rm -f "$IMAGE_ARCHIVE"
    fi
  else
    echo "  Image not found locally — kind will pull from registry at sandbox create time"
  fi
fi

# ── Run tests ───────────────────────────────────────────────────────

echo "=== Running: test-flow.sh ${TEST_ARGS[*]} ==="
echo ""
"$SCRIPT_DIR/test/test-flow.sh" "${TEST_ARGS[@]}"
