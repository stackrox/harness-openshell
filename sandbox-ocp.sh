#!/usr/bin/env bash
# Launch a sandbox on OpenShift from an agent config.
#
# Prerequisites:
#   ./deploy-ocp.sh && ./setup-creds.sh && ./setup-providers.sh
#
# Usage:
#   ./sandbox-ocp.sh              # uses agents/default.toml
#   ./sandbox-ocp.sh research     # uses agents/research.toml
#
# To reconnect: openshell sandbox connect <name>
# To delete:    openshell sandbox delete <name>
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib/common.sh"
source "$SCRIPT_DIR/lib/agent.sh"
require_cli
require_kubectl

NAMESPACE="${OPENSHELL_NAMESPACE:-openshell}"
AGENT_NAME="${1:-default}"
AGENT_FILE="$SCRIPT_DIR/agents/${AGENT_NAME}.toml"
parse_agent "$AGENT_FILE"

echo "=== Agent: $AGENT_NAME ==="
echo "  Name:      $SANDBOX_NAME"
echo "  Image:     $SANDBOX_IMAGE"
echo "  Providers: $SANDBOX_PROVIDERS"

# Create ConfigMap with agent config (TOML, same format as the source)
kubectl create configmap "sandbox-${SANDBOX_NAME}" -n "$NAMESPACE" \
  --from-file=config.toml="$AGENT_FILE" \
  --dry-run=client -o yaml | kubectl apply -f -

# Create sandbox.env ConfigMap
if [[ -n "$SANDBOX_ENV" ]]; then
  kubectl create configmap "sandbox-${SANDBOX_NAME}-env" -n "$NAMESPACE" \
    --from-literal=sandbox.env="$SANDBOX_ENV" \
    --dry-run=client -o yaml | kubectl apply -f -
fi

# Generate and apply the Job
JOB_NAME="sandbox-${SANDBOX_NAME}"
echo ""
echo "=== Launching ==="

# Clean up previous run
kubectl delete job "$JOB_NAME" -n "$NAMESPACE" --force --grace-period=0 2>/dev/null || true
kubectl delete pod -n "$NAMESPACE" -l "job-name=${JOB_NAME}" --force --grace-period=0 2>/dev/null || true

cat <<JOBYAML | kubectl apply -n "$NAMESPACE" -f -
apiVersion: batch/v1
kind: Job
metadata:
  name: ${JOB_NAME}
  namespace: ${NAMESPACE}
spec:
  backoffLimit: 0
  template:
    spec:
      serviceAccountName: openshell-launcher
      restartPolicy: Never
      containers:
        - name: launcher
          image: quay.io/rcochran/openshell:launcher
          imagePullPolicy: Always
          env:
            - name: GATEWAY_ENDPOINT
              value: "https://openshell.openshell.svc.cluster.local:8080"
            - name: HOME
              value: "/tmp"
          volumeMounts:
            - name: config
              mountPath: /etc/openshell/sandbox
              readOnly: true
            - name: gws
              mountPath: /secrets/gws
              readOnly: true
            - name: gateway-mtls
              mountPath: /secrets/mtls
              readOnly: true
            - name: sandbox-env
              mountPath: /etc/openshell/env
              readOnly: true
      volumes:
        - name: config
          configMap:
            name: sandbox-${SANDBOX_NAME}
        - name: gws
          secret:
            secretName: openshell-gws
            optional: true
        - name: gateway-mtls
          secret:
            secretName: openshell-client-tls
        - name: sandbox-env
          configMap:
            name: sandbox-${SANDBOX_NAME}-env
            optional: true
JOBYAML

echo ""
echo "Waiting for launcher..."
kubectl wait --for=condition=ready pod -n "$NAMESPACE" \
  -l "job-name=${JOB_NAME}" --timeout=120s 2>/dev/null || true

# Stream logs until the job finishes
kubectl logs -n "$NAMESPACE" -f -l "job-name=${JOB_NAME}" 2>/dev/null &
LOG_PID=$!

while true; do
  status=$(kubectl get job "$JOB_NAME" -n "$NAMESPACE" -o jsonpath='{.status.conditions[0].type}' 2>/dev/null)
  [[ "$status" == "Complete" || "$status" == "Failed" || "$status" == "SuccessCriteriaMet" ]] && break
  sleep 2
done

kill $LOG_PID 2>/dev/null || true
wait $LOG_PID 2>/dev/null || true

echo ""
if [[ "$status" == "Complete" || "$status" == "SuccessCriteriaMet" ]]; then
  echo "Sandbox ready. Connect with: openshell sandbox connect $SANDBOX_NAME"
else
  echo "Launcher failed. Check logs: kubectl logs -n $NAMESPACE -l job-name=$JOB_NAME"
fi
