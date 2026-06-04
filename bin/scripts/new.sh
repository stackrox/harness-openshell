#!/usr/bin/env bash
# Create a new sandbox.
#
# Ensures the gateway is deployed and providers are registered before creating.
#
# Usage:
#   new.sh                                  # active gateway, default profile
#   new.sh my-sandbox                       # named sandbox
#   new.sh --local                          # ensure local gateway
#   new.sh --remote                         # ensure OCP gateway
#   new.sh --profile coder                  # use profiles/coder.toml
#   new.sh --no-tty --name test-agent       # non-interactive (for testing)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
HARNESS_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
source "$SCRIPT_DIR/lib/common.sh"
source "$SCRIPT_DIR/lib/profile.sh"

# ── Parse args ────────────────────────────────────────────────────────
PLATFORM=""
PROFILE="default"
SANDBOX_NAME=""
TTY_FLAG="--tty"
NO_TTY=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --local)   PLATFORM="local"; shift ;;
    --remote)  PLATFORM="remote"; shift ;;
    --profile) PROFILE="$2"; shift 2 ;;
    --name)    SANDBOX_NAME="$2"; shift 2 ;;
    --no-tty)  TTY_FLAG="--no-tty"; NO_TTY=true; shift ;;
    *)         SANDBOX_NAME="$1"; shift ;;
  esac
done

# ── 1. Ensure gateway ────────────────────────────────────────────────
if [[ -n "$PLATFORM" ]]; then
  "$SCRIPT_DIR/deploy.sh" "--$PLATFORM"
else
  if ! "$CLI" inference get &>/dev/null 2>&1; then
    echo "ERROR: No active gateway. Use --local or --remote."
    exit 1
  fi
fi

# ── 2. Ensure providers ──────────────────────────────────────────────
provider_count=$("$CLI" provider list 2>/dev/null | awk 'NR>1' | wc -l | tr -d ' ')
if [[ "$provider_count" -eq 0 ]]; then
  "$SCRIPT_DIR/providers.sh"
fi

# ── 3. If remote, ensure creds ───────────────────────────────────────
if [[ "$PLATFORM" == "remote" ]]; then
  NAMESPACE="${OPENSHELL_NAMESPACE:-openshell}"
  if ! kubectl get secret openshell-gws -n "$NAMESPACE" &>/dev/null 2>&1; then
    "$SCRIPT_DIR/creds.sh"
  fi
fi

# ── 4. Parse profile ─────────────────────────────────────────────────
PROFILE_FILE="$HARNESS_DIR/profiles/${PROFILE}.toml"
NAME_OVERRIDE="$SANDBOX_NAME"
parse_profile "$PROFILE_FILE"
[[ -n "$NAME_OVERRIDE" ]] && SANDBOX_NAME="$NAME_OVERRIDE"

echo ""
echo "=== Sandbox ==="
echo "  Profile: $PROFILE"
echo "  Image:   $SANDBOX_IMAGE"

# ── 5. Build common flags ────────────────────────────────────────────
echo ""
echo "=== Providers ==="
build_provider_flags

FROM_FLAGS=()
[[ -n "$SANDBOX_IMAGE" ]] && FROM_FLAGS=(--from "$SANDBOX_IMAGE")

NAME_FLAGS=()
[[ -n "$SANDBOX_NAME" ]] && NAME_FLAGS=(--name "$SANDBOX_NAME")

# ── 6. Create sandbox ────────────────────────────────────────────────
create_local() {
  HARNESS_UPLOAD_DIR="/tmp/openshell"
  rm -rf "$HARNESS_UPLOAD_DIR"
  stage_harness_dir "$HARNESS_UPLOAD_DIR"

  if $NO_TTY; then
    CMD=(-- bash /sandbox/startup.sh)
  else
    CMD=(-- bash -c ". /sandbox/startup.sh && exec $SANDBOX_COMMAND")
  fi

  echo ""
  echo "=== Creating sandbox ==="
  for attempt in 1 2 3 4 5; do
    "$CLI" sandbox create \
      $TTY_FLAG \
      ${NAME_FLAGS[@]+"${NAME_FLAGS[@]}"} \
      ${FROM_FLAGS[@]+"${FROM_FLAGS[@]}"} \
      ${PROVIDER_FLAGS[@]+"${PROVIDER_FLAGS[@]}"} \
      --upload "$HARNESS_UPLOAD_DIR:/sandbox/.config" --no-git-ignore \
      "${CMD[@]}" \
      && exit 0
    echo "  Attempt $attempt failed (supervisor race), retrying in 5s..."
    if [[ -n "$SANDBOX_NAME" ]]; then
      "$CLI" sandbox delete "$SANDBOX_NAME" 2>/dev/null || true
    else
      "$CLI" sandbox delete "$("$CLI" sandbox list 2>/dev/null | strip_ansi | awk 'NR==2{print $1}')" 2>/dev/null || true
    fi
    sleep 5
  done
  echo "ERROR: Failed after 5 attempts."
  exit 1
}

create_remote() {
  NAMESPACE="${OPENSHELL_NAMESPACE:-openshell}"

  kubectl create configmap "sandbox-${SANDBOX_NAME}" -n "$NAMESPACE" \
    --from-file=config.toml="$PROFILE_FILE" \
    --dry-run=client -o yaml | kubectl apply -f -

  if [[ -n "$SANDBOX_ENV" ]]; then
    kubectl create configmap "sandbox-${SANDBOX_NAME}-env" -n "$NAMESPACE" \
      --from-literal=sandbox.env="$SANDBOX_ENV" \
      --dry-run=client -o yaml | kubectl apply -f -
  fi

  local job_name="sandbox-${SANDBOX_NAME}"
  kubectl delete job "$job_name" -n "$NAMESPACE" --force --grace-period=0 2>/dev/null || true
  kubectl delete pod -n "$NAMESPACE" -l "job-name=${job_name}" --force --grace-period=0 2>/dev/null || true

  cat <<JOBYAML | kubectl apply -n "$NAMESPACE" -f -
apiVersion: batch/v1
kind: Job
metadata:
  name: ${job_name}
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
    -l "job-name=${job_name}" --timeout=120s 2>/dev/null || true

  kubectl logs -n "$NAMESPACE" -f -l "job-name=${job_name}" 2>/dev/null &
  local log_pid=$!

  while true; do
    local status
    status=$(kubectl get job "$job_name" -n "$NAMESPACE" -o jsonpath='{.status.conditions[0].type}' 2>/dev/null)
    [[ "$status" == "Complete" || "$status" == "Failed" || "$status" == "SuccessCriteriaMet" ]] && break
    sleep 2
  done

  kill $log_pid 2>/dev/null || true
  wait $log_pid 2>/dev/null || true

  echo ""
  if [[ "$status" == "Complete" || "$status" == "SuccessCriteriaMet" ]]; then
    echo "Sandbox ready. Connect with: harness connect ${SANDBOX_NAME}"
  else
    echo "Launcher failed."
  fi
}

# ── Main ──────────────────────────────────────────────────────────────
if [[ "$PLATFORM" == "remote" ]]; then
  create_remote
else
  create_local
fi
