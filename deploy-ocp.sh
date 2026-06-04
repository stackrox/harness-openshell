#!/usr/bin/env bash
# Deploy OpenShell to an OpenShift cluster using the official Helm chart.
#
# Uses the OCI Helm chart from ghcr.io — no local OpenShell repo clone needed.
#
# Usage:
#   ./deploy-ocp.sh                           # full deploy
#   ./deploy-ocp.sh --kubeconfig ./kubeconfig  # explicit kubeconfig
#
# Environment variables (all optional, sensible defaults provided):
#   OPENSHELL_CHART_VERSION — Helm chart version (default: from openshell.toml or 0.0.55)
#   SANDBOX_IMAGE           — sandbox image (default: quay.io/rcochran/openshell:sandbox)
#   PULL_SECRET             — imagePullSecrets name (default: none)
#   SANDBOX_PULL_SECRET     — sandbox imagePullSecrets name (default: none)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

# ── Dependency checks ──────────────────────────────────────────────────
for cmd in kubectl helm; do
  command -v "$cmd" &>/dev/null || { echo "ERROR: $cmd is required but not found."; exit 1; }
done

while [[ $# -gt 0 ]]; do
  case $1 in
    --kubeconfig) export KUBECONFIG="$2"; shift 2 ;;
    *) echo "Unknown argument: $1"; exit 1 ;;
  esac
done

# Read chart version from openshell.toml or env
CHART_VERSION="${OPENSHELL_CHART_VERSION:-}"
if [[ -z "$CHART_VERSION" && -f "$SCRIPT_DIR/openshell.toml" ]]; then
  CHART_VERSION=$(python3 -c "
import tomllib
with open('$SCRIPT_DIR/openshell.toml', 'rb') as f:
    print(tomllib.load(f).get('upstream', {}).get('chart-version', ''))
" 2>/dev/null || true)
fi
CHART_VERSION="${CHART_VERSION:-0.0.55}"
CHART="oci://ghcr.io/nvidia/openshell/helm-chart"

echo "OpenShell chart: $CHART_VERSION"
echo "KUBECONFIG: ${KUBECONFIG:-default}"
echo ""

# ── Step 1: Namespace ──────────────────────────────────────────────────
echo "=== Step 1: Creating namespace ==="
kubectl create ns openshell 2>/dev/null || true
kubectl label ns openshell \
  pod-security.kubernetes.io/enforce=privileged \
  pod-security.kubernetes.io/warn=privileged \
  --overwrite

# ── Step 2: Sandbox CRD + controller ──────────────────────────────────
echo "=== Step 2: Installing Sandbox CRD ==="
kubectl apply -f https://github.com/kubernetes-sigs/agent-sandbox/releases/latest/download/manifest.yaml

# ── Step 3: OpenShift SCCs ────────────────────────────────────────────
echo "=== Step 3: Granting OpenShift SCCs ==="
for sa in openshell openshell-sandbox default; do
  oc adm policy add-scc-to-user privileged -z "$sa" -n openshell 2>/dev/null || true
done
oc adm policy add-scc-to-user anyuid -z openshell -n openshell 2>/dev/null || true
kubectl create clusterrolebinding agent-sandbox-admin \
  --clusterrole=cluster-admin \
  --serviceaccount=agent-sandbox-system:agent-sandbox-controller 2>/dev/null || true

# ── Launcher ServiceAccount + RBAC ────────────────────────────────────
kubectl apply -n openshell -f - <<'RBAC'
apiVersion: v1
kind: ServiceAccount
metadata:
  name: openshell-launcher
  namespace: openshell
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: openshell-launcher
  namespace: openshell
rules:
  - apiGroups: [""]
    resources: ["configmaps", "secrets"]
    verbs: ["get", "list"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: openshell-launcher
  namespace: openshell
subjects:
  - kind: ServiceAccount
    name: openshell-launcher
    namespace: openshell
roleRef:
  kind: Role
  name: openshell-launcher
  apiGroup: rbac.authorization.k8s.io
RBAC

# ── Step 4: Helm install gateway ──────────────────────────────────────
echo "=== Step 4: Deploying gateway via Helm ==="

SANDBOX_IMAGE="${SANDBOX_IMAGE:-quay.io/rcochran/openshell:sandbox}"

APPS_DOMAIN=$(kubectl get ingresses.config.openshift.io cluster -o jsonpath='{.spec.domain}' 2>/dev/null)
if [[ -z "$APPS_DOMAIN" ]]; then
  echo "ERROR: Could not determine OpenShift apps domain."
  exit 1
fi
ROUTE_HOST="gateway-openshell.${APPS_DOMAIN}"

HELM_ARGS=(
  --values "$SCRIPT_DIR/values-ocp.yaml"
  --set server.sandboxImage="$SANDBOX_IMAGE"
  --set pkiInitJob.serverDnsNames[0]="$ROUTE_HOST"
)

[[ -n "${PULL_SECRET:-}" ]]         && HELM_ARGS+=(--set imagePullSecrets[0].name="$PULL_SECRET")
[[ -n "${SANDBOX_PULL_SECRET:-}" ]] && HELM_ARGS+=(--set server.sandboxImagePullSecrets[0].name="$SANDBOX_PULL_SECRET")

helm upgrade --install openshell "$CHART" --version "$CHART_VERSION" -n openshell \
  "${HELM_ARGS[@]}"

echo "=== Waiting for gateway ==="
kubectl rollout status statefulset/openshell -n openshell --timeout=300s

# ── Step 5: OpenShift Route (TLS passthrough) ─────────────────────────
echo "=== Step 5: Creating OpenShift route ==="
if ! kubectl get route gateway -n openshell &>/dev/null; then
  cat <<'ROUTE' | kubectl apply -f -
apiVersion: route.openshift.io/v1
kind: Route
metadata:
  name: gateway
  namespace: openshell
spec:
  tls:
    termination: passthrough
  to:
    kind: Service
    name: openshell
  port:
    targetPort: grpc
ROUTE
fi
echo "  Route: $ROUTE_HOST"

# ── Step 6: Configure CLI gateway ─────────────────────────────────────
echo "=== Step 6: Configuring CLI gateway ==="
GATEWAY_NAME="${GATEWAY_NAME:-openshell-remote-ocp}"
GATEWAY_URL="https://${ROUTE_HOST}:443"
CLI="${OPENSHELL_CLI:-openshell}"

if command -v "$CLI" &>/dev/null; then
  for gw in $("$CLI" gateway list 2>/dev/null | grep "$ROUTE_HOST" | awk '{gsub(/^\*/, ""); print $1}'); do
    "$CLI" gateway remove "$gw" 2>/dev/null || true
  done

  "$CLI" gateway add "$GATEWAY_URL" --name "$GATEWAY_NAME" --local 2>/dev/null || true

  MTLS_DIR="$HOME/.config/openshell/gateways/$GATEWAY_NAME/mtls"
  kubectl get secret openshell-client-tls -n openshell \
    -o jsonpath='{.data.ca\.crt}' | base64 -d > "$MTLS_DIR/ca.crt"
  kubectl get secret openshell-client-tls -n openshell \
    -o jsonpath='{.data.tls\.crt}' | base64 -d > "$MTLS_DIR/tls.crt"
  kubectl get secret openshell-client-tls -n openshell \
    -o jsonpath='{.data.tls\.key}' | base64 -d > "$MTLS_DIR/tls.key"

  "$CLI" gateway select "$GATEWAY_NAME" 2>/dev/null || true
  echo "  ✓ $GATEWAY_NAME registered (certs from cluster)"

  echo -n "  Waiting for gateway..."
  for i in $(seq 1 30); do
    if "$CLI" inference get &>/dev/null; then
      echo " ✓ reachable"
      break
    fi
    sleep 2
    echo -n "."
    [[ $i -eq 30 ]] && echo " ✗ timed out (try: openshell inference get)"
  done
fi

echo ""
echo "Done. Next: ./setup-creds.sh && ./setup-providers.sh && ./sandbox-ocp.sh"
