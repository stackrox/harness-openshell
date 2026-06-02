#!/usr/bin/env bash
# Convenience wrapper: generate and apply a sandbox Job YAML.
#
# Equivalent to editing sandbox.yaml and running kubectl apply.
# All credentials come from K8s Secrets — no env vars needed at launch.
#
# Prerequisites (one-time):
#   ./deploy-ocp.sh          # deploys gateway + launcher RBAC
#   ./setup-creds.sh         # stores GWS + Atlassian config in cluster
#   ./setup-providers.sh     # registers API tokens with gateway
#
# Usage:
#   ./sandbox.sh                        # launch with defaults
#   ./sandbox.sh --name my-sandbox      # custom sandbox name
#   ./sandbox.sh --no-keep              # delete sandbox after exit
#   ./sandbox.sh --rejoin my-sandbox    # reconnect to existing sandbox
#   ./sandbox.sh --providers "github vertex-local"  # override providers
#
# Or skip this wrapper entirely:
#   kubectl apply -f sandbox.yaml
set -euo pipefail

NAMESPACE="${OPENSHELL_NAMESPACE:-openshell}"
export OPENSHELL_GATEWAY="${GATEWAY_NAME:-ocp}"

CLI="${OPENSHELL_CLI:-openshell}"
command -v "$CLI" &>/dev/null || { echo "ERROR: openshell CLI not found."; exit 1; }
command -v kubectl &>/dev/null || { echo "ERROR: kubectl is required."; exit 1; }

# ── Parse args ─────────────────────────────────────────────────────────
NAME="agent"
KEEP="true"
REJOIN=""
PROVIDERS="github vertex-local atlassian"
while [[ $# -gt 0 ]]; do
  case $1 in
    --rejoin)    REJOIN="$2"; shift 2 ;;
    --name)      NAME="$2"; shift 2 ;;
    --no-keep)   KEEP="false"; shift ;;
    --providers) PROVIDERS="$2"; shift 2 ;;
    *) echo "Unknown argument: $1"; exit 1 ;;
  esac
done

# ── Rejoin: connect directly without creating a new sandbox ───────────
if [[ -n "$REJOIN" ]]; then
  echo "Reconnecting to sandbox: $REJOIN"
  exec "$CLI" sandbox connect "$REJOIN"
fi

# ── Pre-flight ─────────────────────────────────────────────────────────
echo "=== Pre-flight checks ==="
echo -n "  Gateway ($OPENSHELL_GATEWAY): "
"$CLI" inference get &>/dev/null && echo "reachable" || { echo "UNREACHABLE — run ./deploy-ocp.sh"; exit 1; }

model=$("$CLI" inference get 2>/dev/null | grep Model: | awk '{print $2}')
[[ -n "$model" ]] && echo "  Inference route: $model" || { echo "  ERROR: inference not set — run ./setup-providers.sh"; exit 1; }

# Clean up any previous failed Job/sandbox with the same name
JOB_NAME="sandbox-${NAME}"
CM_NAME="sandbox-${NAME}"
if kubectl get job "$JOB_NAME" -n "$NAMESPACE" &>/dev/null 2>&1; then
  echo "  Deleting previous Job: $JOB_NAME"
  kubectl delete job "$JOB_NAME" -n "$NAMESPACE" 2>/dev/null || true
fi
if "$CLI" sandbox list 2>/dev/null | awk 'NR>1 {print $1}' | grep -qFx "$NAME"; then
  echo "  Deleting previous sandbox: $NAME"
  "$CLI" sandbox delete "$NAME" 2>/dev/null || true
fi

# ── Generate and apply YAML ────────────────────────────────────────────
echo ""
echo "=== Launching sandbox: $NAME ==="
kubectl apply -n "$NAMESPACE" -f - <<YAML
apiVersion: v1
kind: ConfigMap
metadata:
  name: ${CM_NAME}
  namespace: ${NAMESPACE}
data:
  SANDBOX_NAME: "${NAME}"
  SANDBOX_PROVIDERS: "${PROVIDERS}"
  SANDBOX_KEEP: "${KEEP}"
  SANDBOX_COMMAND: "claude --bare"
---
apiVersion: batch/v1
kind: Job
metadata:
  name: ${JOB_NAME}
  namespace: ${NAMESPACE}
  labels:
    app: openshell-sandbox
    sandbox-name: "${NAME}"
spec:
  backoffLimit: 0
  template:
    metadata:
      labels:
        app: openshell-sandbox
        sandbox-name: "${NAME}"
    spec:
      serviceAccountName: openshell-launcher
      restartPolicy: Never
      containers:
        - name: launcher
          image: quay.io/rcochran/openshell:launcher
          imagePullPolicy: Always
          envFrom:
            - configMapRef:
                name: ${CM_NAME}
          env:
            - name: GATEWAY_ENDPOINT
              value: "https://openshell.openshell.svc.cluster.local:8080"
            - name: OPENSHELL_GATEWAY_INSECURE
              value: "true"
          volumeMounts:
            - name: gws
              mountPath: /secrets/gws
              readOnly: true
            - name: atlassian
              mountPath: /secrets/atlassian
              readOnly: true
      volumes:
        - name: gws
          secret:
            secretName: openshell-gws
            optional: true
        - name: atlassian
          secret:
            secretName: openshell-atlassian
            optional: true
YAML

echo ""
echo "Job created. Streaming logs (Ctrl-C to detach, sandbox keeps running):"
kubectl wait --for=condition=ready pod -n "$NAMESPACE" \
  -l "sandbox-name=${NAME}" --timeout=120s 2>/dev/null || true
kubectl attach -n "$NAMESPACE" -it \
  "$(kubectl get pod -n "$NAMESPACE" -l "sandbox-name=${NAME}" -o name | head -1)" \
  -c launcher 2>/dev/null || \
kubectl logs -n "$NAMESPACE" -f \
  "$(kubectl get pod -n "$NAMESPACE" -l "sandbox-name=${NAME}" -o name | head -1)" \
  -c launcher 2>/dev/null || true
