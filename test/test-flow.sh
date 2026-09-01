#!/usr/bin/env bash
# End-to-end validation of the canonical SDK sandbox lifecycle.
set -uo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
HARNESS="$ROOT/harness"
CLI="${OPENSHELL_CLI:-openshell}"
WORKFLOW="$ROOT/test/lifecycle-workflow.yaml"
AUTO_WORKFLOW="$ROOT/test/ci-workflow.yaml"
TARGET=""
REUSE_GATEWAY=false

for arg in "$@"; do
  case "$arg" in
    --ci|--no-providers) ;;
    --reuse-gateway) REUSE_GATEWAY=true ;;
    --workflow=*) WORKFLOW="${arg#--workflow=}" ;;
    -*) echo "Unknown flag: $arg"; exit 1 ;;
    *) [[ -z "$TARGET" ]] && TARGET="$arg" ;;
  esac
done

if [[ -z "$TARGET" || ! -x "$HARNESS" ]]; then
  echo "Usage: $0 <local-container|helm|openshift|all> [--ci] [--reuse-gateway]"
  exit 1
fi

harness() { "$HARNESS" "$@"; }
strip_ansi() { sed 's/\x1b\[[0-9;]*m//g'; }
source "$ROOT/test/lib/provision.sh"

PASS=0
FAIL=0
SKIP=0
START=$(date +%s)

step() {
  local label="$1"; shift
  local started=$SECONDS
  if "$@"; then
    printf "  ✓ %-35s (%ds)\n" "$label" "$((SECONDS-started))"
    ((PASS++))
  else
    printf "  ✗ %-35s (%ds)\n" "$label" "$((SECONDS-started))"
    ((FAIL++))
  fi
}

step_fail() {
  local label="$1"; shift
  if ! "$@" >/dev/null 2>&1; then
    printf "  ✓ %-35s (expected failure)\n" "$label"
    ((PASS++))
  else
    printf "  ✗ %-35s (should have failed)\n" "$label"
    ((FAIL++))
  fi
}

active_gateway() {
  "$CLI" gateway list 2>/dev/null | strip_ansi | awk '{for (i = 1; i <= NF; i++) if ($i == "*") {print $(i + 1); exit}}'
}

cleanup_gateway() {
  local gateway="$1"
  harness delete --gateway "$gateway" --sandboxes >/dev/null 2>&1 || true
}

provider_exists() {
  "$CLI" provider list --names 2>/dev/null | grep -Fxq "$1"
}

exercise_provider() {
  local gateway="$1" provider="$2" check="$3"
  if ! provider_exists "$provider"; then
    printf "  - %-35s (skip: not provisioned)\n" "provider: $provider"
    ((SKIP++))
    return
  fi
  local workflow sandbox image
  workflow=$(mktemp)
  sandbox="test-${provider//[^a-zA-Z0-9]/-}"
  image="${HARNESS_OS_IMAGE:-ghcr.io/nvidia/openshell-community/sandboxes/base:latest}"
  printf '%s\n' \
    'apiVersion: harness.openshell.dev/v1alpha1' \
    'kind: Harness' \
    'metadata:' "  name: $sandbox" \
    'spec:' '  sandbox:' "    image: $image" '    keep: true' \
    '    providers:' "      - $provider" \
    '  agent:' '    type: sh' '    args: [-c, "true"]' >"$workflow"
  step "provider: $provider attach" harness apply -f "$workflow" --gateway "$gateway"
  step "provider: $provider capability" "$CLI" sandbox exec --name "$sandbox" -- bash -c "$check"
  harness delete --gateway "$gateway" "$sandbox" >/dev/null 2>&1 || true
  rm -f "$workflow"
}

exercise_providers() {
  local gateway="$1"
  exercise_provider "$gateway" github 'curl -sf https://api.github.com/user -H "Authorization: Bearer $GITHUB_TOKEN" >/dev/null'
  exercise_provider "$gateway" gws 'curl -sf https://gmail.googleapis.com/gmail/v1/users/me/profile -H "Authorization: Bearer $GOOGLE_WORKSPACE_CLI_TOKEN" >/dev/null'
  exercise_provider "$gateway" google-vertex-ai 'test -n "$GOOGLE_VERTEX_AI_TOKEN"'
}

exercise_lifecycle() {
  local gateway="$1" sandbox="$2"
  cleanup_gateway "$gateway"
  step "canonical apply" harness apply -f "$WORKFLOW" --gateway "$gateway" --name "$sandbox"
  step "sandbox describe" harness describe --gateway "$gateway" "$sandbox"
  step "sandbox listed" bash -c '"$1" get agents --gateway "$2" | grep -q "$3"' _ "$HARNESS" "$gateway" "$sandbox"
  step "sandbox exec" "$CLI" sandbox exec --name "$sandbox" -- bash -c 'test "$STATIC_VAR" = hello-world'
  step "sandbox delete" harness delete --gateway "$gateway" "$sandbox"
  local auto_sandbox="${sandbox}-auto"
  step "automatic cleanup apply" harness apply -f "$AUTO_WORKFLOW" --gateway "$gateway" --name "$auto_sandbox"
  step_fail "automatic cleanup verified" harness describe --gateway "$gateway" "$auto_sandbox"
}

test_errors() {
  echo "=== canonical errors ==="
  step_fail "missing workflow" harness apply
  step_fail "unversioned workflow" bash -c 'f=$(mktemp); printf "name: old\n" >"$f"; "$1" apply -f "$f"; rc=$?; rm -f "$f"; exit $rc' _ "$HARNESS"
  echo
}

test_local() {
  echo "=== local container ==="
  step "provision" provision_local
  local gateway
  gateway=$(active_gateway)
  if [[ -z "$gateway" ]]; then echo "  ERROR: active gateway not found"; ((FAIL++)); return; fi
  exercise_lifecycle "$gateway" test-local-sdk
  exercise_providers "$gateway"
  cleanup_gateway "$gateway"
}

test_kind() {
  echo "=== kind ==="
  if ! kubectl get nodes >/dev/null 2>&1; then echo "  ERROR: no kind cluster"; ((FAIL++)); return; fi
  teardown_cluster openshell-kind
  step "provision" provision_kind
  exercise_lifecycle openshell-kind test-kind-sdk
  exercise_providers openshell-kind
  cleanup_gateway openshell-kind
  step "cluster teardown" teardown_cluster openshell-kind
}

test_ocp() {
  echo "=== OpenShift ==="
  local gateway=openshell-remote-ocp
  if $REUSE_GATEWAY; then
    "$CLI" gateway select "$gateway" >/dev/null 2>&1 || step "provision" provision_ocp
  else
    teardown_cluster "$gateway"
    step "provision" provision_ocp
  fi
  exercise_lifecycle "$gateway" test-ocp-sdk
  exercise_providers "$gateway"
  cleanup_gateway "$gateway"
  $REUSE_GATEWAY || step "cluster teardown" teardown_cluster "$gateway"
}

test_errors
case "$TARGET" in
  local-container|local|podman) test_local ;;
  helm|kind) test_kind ;;
  openshift|ocp) test_ocp ;;
  all) test_local; test_kind; test_ocp ;;
  *) echo "Unknown target: $TARGET"; exit 1 ;;
esac

TOTAL=$((PASS + FAIL))
ELAPSED=$(( $(date +%s) - START ))
echo
echo "$PASS/$TOTAL passed, $FAIL failed, $SKIP skipped (${ELAPSED}s)"
exit "$FAIL"
