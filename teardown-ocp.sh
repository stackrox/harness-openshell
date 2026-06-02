#!/usr/bin/env bash
# Tear down OpenShell from the OpenShift cluster.
# Safe to run multiple times — each step tolerates missing resources.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
OPENSHELL_REPO="${OPENSHELL_REPO:-$(cd "$SCRIPT_DIR/../../nvidia/OpenShell" 2>/dev/null && pwd || echo "$SCRIPT_DIR/../OpenShell")}"
GATEWAY_NAME="${GATEWAY_NAME:-ocp}"
export OPENSHELL_GATEWAY="$GATEWAY_NAME"
CLI="${OPENSHELL_CLI:-openshell}"

for cmd in kubectl helm; do
  command -v "$cmd" &>/dev/null || { echo "ERROR: $cmd is required but not found."; exit 1; }
done

echo "WARNING: This will delete all sandboxes, providers, and cluster resources."
read -r -p "Continue? [y/N] " confirm
[[ "$confirm" =~ ^[yY]$ ]] || { echo "Aborted."; exit 0; }

echo "=== Deleting sandboxes ==="
if command -v "$CLI" &>/dev/null && "$CLI" gateway info "$GATEWAY_NAME" &>/dev/null; then
  "$CLI" sandbox list 2>/dev/null | awk 'NR>1 {print $1}' | while read -r name; do
    echo "  Deleting sandbox: $name"
    "$CLI" sandbox delete "$name" 2>/dev/null || true
  done
fi

echo "=== Uninstalling Helm release ==="
helm uninstall openshell -n openshell 2>/dev/null || echo "  (not installed)"

echo "=== Removing Sandbox CRD ==="
if [[ -f "$OPENSHELL_REPO/deploy/kube/manifests/agent-sandbox.yaml" ]]; then
  kubectl delete -f "$OPENSHELL_REPO/deploy/kube/manifests/agent-sandbox.yaml" 2>/dev/null || true
else
  kubectl delete ns agent-sandbox-system 2>/dev/null || true
fi

echo "=== Removing cluster role bindings ==="
for crb in openshell-sa-anyuid openshell-sa-privileged openshell-default-privileged \
            openshell-sandbox-privileged agent-sandbox-admin; do
  kubectl delete clusterrolebinding "$crb" 2>/dev/null || true
done

# Launcher RBAC lives in the namespace so it's deleted with it,
# but remove explicitly for robustness.
kubectl delete rolebinding,role openshell-launcher -n openshell 2>/dev/null || true
kubectl delete serviceaccount openshell-launcher -n openshell 2>/dev/null || true

echo "=== Deleting namespace ==="
kubectl delete ns openshell 2>/dev/null || echo "  (not found)"

echo "=== Removing local gateway config ==="
if command -v "$CLI" &>/dev/null; then
  "$CLI" gateway remove "$GATEWAY_NAME" 2>/dev/null || true
fi

echo ""
echo "Teardown complete. Run ./deploy-ocp.sh to redeploy."
