#!/usr/bin/env bash
# Deploy OpenShell to an OpenShift cluster using the official Helm chart.
#
# Usage:
#   ./deploy-ocp.sh                           # full deploy
#   ./deploy-ocp.sh --kubeconfig ./kubeconfig  # explicit kubeconfig
#
# Environment variables (all optional, sensible defaults provided):
#   OPENSHELL_REPO          — path to NVIDIA/OpenShell checkout (default: ../OpenShell)
#   GATEWAY_IMAGE_REPO      — gateway image repo   (default: quay.io/rcochran/openshell)
#   GATEWAY_IMAGE_TAG       — gateway image tag     (default: gateway)
#   SUPERVISOR_IMAGE_REPO   — supervisor image repo (default: quay.io/rcochran/openshell)
#   SANDBOX_IMAGE           — sandbox image          (default: quay.io/rcochran/openshell:sandbox)
#   PULL_SECRET             — imagePullSecrets name  (default: none)
#   SANDBOX_PULL_SECRET     — sandbox imagePullSecrets name (default: none)
#   GATEWAY_NAME            — CLI gateway name       (default: ocp)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

# ── Dependency checks ──────────────────────────────────────────────────
for cmd in kubectl helm base64; do
  command -v "$cmd" &>/dev/null || { echo "ERROR: $cmd is required but not found."; exit 1; }
done

while [[ $# -gt 0 ]]; do
  case $1 in
    --kubeconfig) export KUBECONFIG="$2"; shift 2 ;;
    *) echo "Unknown argument: $1"; exit 1 ;;
  esac
done

OPENSHELL_REPO="${OPENSHELL_REPO:-$(cd "$SCRIPT_DIR/../../nvidia/OpenShell" 2>/dev/null && pwd || echo "$SCRIPT_DIR/../OpenShell")}"
if [[ ! -d "$OPENSHELL_REPO/deploy/helm/openshell" ]]; then
  echo "ERROR: OpenShell repo not found at $OPENSHELL_REPO"
  echo "Set OPENSHELL_REPO or clone NVIDIA/OpenShell alongside this repo"
  exit 1
fi

GATEWAY_NAME="${GATEWAY_NAME:-ocp}"

echo "Using OpenShell repo: $OPENSHELL_REPO"
echo "Using KUBECONFIG: ${KUBECONFIG:-default}"
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
kubectl apply -f "$OPENSHELL_REPO/deploy/kube/manifests/agent-sandbox.yaml"

# ── Step 3: OpenShift SCCs ────────────────────────────────────────────
echo "=== Step 3: Granting OpenShift SCCs ==="
kubectl create clusterrolebinding openshell-sa-anyuid \
  --clusterrole=system:openshift:scc:anyuid \
  --serviceaccount=openshell:openshell 2>/dev/null || true
kubectl create clusterrolebinding openshell-sa-privileged \
  --clusterrole=system:openshift:scc:privileged \
  --serviceaccount=openshell:openshell 2>/dev/null || true
kubectl create clusterrolebinding openshell-default-privileged \
  --clusterrole=system:openshift:scc:privileged \
  --serviceaccount=openshell:default 2>/dev/null || true
kubectl create clusterrolebinding agent-sandbox-admin \
  --clusterrole=cluster-admin \
  --serviceaccount=agent-sandbox-system:agent-sandbox-controller 2>/dev/null || true

# Also grant privileged to the sandbox service account created by Helm
kubectl create clusterrolebinding openshell-sandbox-privileged \
  --clusterrole=system:openshift:scc:privileged \
  --serviceaccount=openshell:openshell-sandbox 2>/dev/null || true

# ── Launcher ServiceAccount + RBAC ────────────────────────────────────
# The in-cluster sandbox launcher (sandbox.yaml Job) needs to read
# ConfigMaps and Secrets in the openshell namespace.
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

# Custom gateway and supervisor builds (required for google-vertex-ai
# provider and model discovery fix).
GATEWAY_IMAGE_REPO="${GATEWAY_IMAGE_REPO:-quay.io/rcochran/openshell}"
GATEWAY_IMAGE_TAG="${GATEWAY_IMAGE_TAG:-gateway}"
SUPERVISOR_IMAGE_REPO="${SUPERVISOR_IMAGE_REPO:-quay.io/rcochran/openshell}"
SUPERVISOR_IMAGE_TAG="${SUPERVISOR_IMAGE_TAG:-supervisor}"
echo "  Gateway image tag:    $GATEWAY_IMAGE_TAG"
echo "  Supervisor image tag: $SUPERVISOR_IMAGE_TAG"

SANDBOX_IMAGE="${SANDBOX_IMAGE:-quay.io/rcochran/openshell:sandbox}"

# Compute the route hostname for the server cert SAN
APPS_DOMAIN=$(kubectl get ingresses.config.openshift.io cluster -o jsonpath='{.spec.domain}' 2>/dev/null)
if [[ -z "$APPS_DOMAIN" ]]; then
  echo "ERROR: Could not determine OpenShift apps domain."
  echo "This script requires an OpenShift cluster with an ingress controller."
  exit 1
fi
ROUTE_HOST="gateway-openshell.${APPS_DOMAIN}"

HELM_ARGS=(
  --set server.sandboxImage="$SANDBOX_IMAGE"
  --set server.sandboxImagePullPolicy=Always
  --set server.dbUrl="sqlite:/var/openshell/openshell.db"
  --set pkiInitJob.enabled=true
  --set pkiInitJob.serverDnsNames[0]="$ROUTE_HOST"
  --set service.type=ClusterIP
  --set image.tag="$GATEWAY_IMAGE_TAG"
  --set image.pullPolicy=Always
  --set supervisor.image.tag="$SUPERVISOR_IMAGE_TAG"
  # allowUnauthenticatedUsers: the gateway is not open to the internet — it is
  # only accessible via the OpenShift route with mTLS passthrough. The client
  # cert (in ~/.config/openshell/gateways/ocp/mtls/) is the authentication gate.
  # To add OIDC on top of mTLS, configure --oidc-issuer in the gateway config.
  --set server.auth.allowUnauthenticatedUsers=true
)

HELM_ARGS+=(--set image.repository="$GATEWAY_IMAGE_REPO")
HELM_ARGS+=(--set supervisor.image.repository="$SUPERVISOR_IMAGE_REPO")
[[ -n "${PULL_SECRET:-}" ]]          && HELM_ARGS+=(--set imagePullSecrets[0].name="$PULL_SECRET")
[[ -n "${SANDBOX_PULL_SECRET:-}" ]]  && HELM_ARGS+=(--set server.sandboxImagePullSecrets[0].name="$SANDBOX_PULL_SECRET")

helm upgrade --install openshell "$OPENSHELL_REPO/deploy/helm/openshell" -n openshell \
  "${HELM_ARGS[@]}"

echo "=== Waiting for PKI init job ==="
kubectl wait job -n openshell -l app.kubernetes.io/component=certgen \
  --for=condition=complete --timeout=120s 2>/dev/null || true

echo "=== Waiting for gateway ==="
kubectl rollout status statefulset/openshell -n openshell --timeout=300s

# ── Step 5: OpenShift Route (TLS passthrough) ─────────────────────────
# mTLS is preserved end-to-end: the route forwards encrypted traffic
# without terminating TLS, so the gateway validates the client cert.
# The server cert includes the route hostname as a SAN (set in Step 4).
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

# ── Step 6: Configure local CLI gateway ───────────────────────────────
echo "=== Step 6: Configuring local CLI gateway ==="
GW_DIR="$HOME/.config/openshell/gateways/$GATEWAY_NAME"
MTLS_DIR="$GW_DIR/mtls"
mkdir -p "$MTLS_DIR"

kubectl get secret openshell-client-tls -n openshell \
  -o jsonpath='{.data.ca\.crt}' | base64 -d > "$MTLS_DIR/ca.crt"
kubectl get secret openshell-client-tls -n openshell \
  -o jsonpath='{.data.tls\.crt}' | base64 -d > "$MTLS_DIR/tls.crt"
kubectl get secret openshell-client-tls -n openshell \
  -o jsonpath='{.data.tls\.key}' | base64 -d > "$MTLS_DIR/tls.key"

CLI="${OPENSHELL_CLI:-openshell}"
if command -v "$CLI" &>/dev/null; then
  "$CLI" gateway remove "$GATEWAY_NAME" 2>/dev/null || true
  "$CLI" gateway add "https://${ROUTE_HOST}:443" --name "$GATEWAY_NAME" --local
fi

echo ""
echo "════════════════════════════════════════════════════"
echo "  OpenShell deployed successfully!"
echo "════════════════════════════════════════════════════"
echo ""
echo "  Gateway route: https://$ROUTE_HOST"
echo ""
echo "Next steps:"
echo ""
echo "  1. Register providers (one-time, or after teardown + redeploy):"
echo "     ./setup-providers.sh"
echo ""
echo "  2. Launch a sandbox:"
echo "     ./sandbox.sh --name my-agent"
echo ""
