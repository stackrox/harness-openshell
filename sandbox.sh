#!/usr/bin/env bash
# Launch an OpenShell sandbox by applying a sandbox YAML file.
#
# Prerequisites (one-time):
#   ./deploy-ocp.sh
#   ./setup-creds.sh
#   ./setup-providers.sh
#
# Usage:
#   ./sandbox.sh                          # apply sandbox.yaml
#   ./sandbox.sh my-sandbox.yaml          # apply a custom sandbox file
#   ./sandbox.sh --rejoin agent           # reconnect to existing sandbox
#
# Edit sandbox.yaml to configure name, providers, skills, etc.
set -euo pipefail

export OPENSHELL_GATEWAY="${GATEWAY_NAME:-ocp}"
NAMESPACE="${OPENSHELL_NAMESPACE:-openshell}"

CLI="${OPENSHELL_CLI:-openshell}"
command -v "$CLI" &>/dev/null || { echo "ERROR: openshell CLI not found."; exit 1; }
command -v kubectl &>/dev/null || { echo "ERROR: kubectl is required."; exit 1; }

case "${1:-}" in
  --rejoin)
    echo "Reconnecting to sandbox: $2"
    exec "$CLI" sandbox connect "$2"
    ;;
  *.yaml|*.yml)
    SANDBOX_FILE="$1"
    ;;
  "")
    SANDBOX_FILE="sandbox.yaml"
    ;;
  *)
    echo "Usage: $0 [sandbox.yaml | --rejoin <name>]"
    exit 1
    ;;
esac

[[ -f "$SANDBOX_FILE" ]] || { echo "ERROR: $SANDBOX_FILE not found."; exit 1; }

echo "=== Applying $SANDBOX_FILE ==="
kubectl apply -n "$NAMESPACE" -f "$SANDBOX_FILE"

JOB_NAME=$(grep -A5 'kind: Job' "$SANDBOX_FILE" | grep 'name:' | head -1 | awk '{print $2}' | tr -d '"')
echo ""
echo "Waiting for launcher..."
kubectl wait --for=condition=ready pod -n "$NAMESPACE" \
  -l "job-name=${JOB_NAME}" --timeout=120s 2>/dev/null || true

echo ""
kubectl logs -n "$NAMESPACE" -f -l "job-name=${JOB_NAME}" 2>/dev/null || true
