#!/usr/bin/env bash
# End-to-end validation for harness CLI.
#
# CI mode auto-detects from the CI env var (set by GitHub Actions).
# Override with --ci or --no-providers.
#
# Usage:
#   ./test-flow.sh local                 # full test with credentials
#   ./test-flow.sh kind                  # full test on kind cluster
#   ./test-flow.sh ocp                   # full test on OCP
#   ./test-flow.sh ocp --reuse-gateway   # skip deploy/teardown
#   ./test-flow.sh all                   # all gateways
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
HARNESS="$SCRIPT_DIR/harness"
CLI="${OPENSHELL_CLI:-openshell}"

if [[ ! -x "$HARNESS" ]]; then
  echo "ERROR: Go binary not found at $HARNESS"
  echo "  Build it first: make cli"
  exit 1
fi

# ── Parse args ──────────────────────────────────────────────────────
TARGET=""
REUSE_GATEWAY=false
NO_PROVIDERS=false
PROFILE="default"

# Auto-detect CI mode
if [[ "${CI:-}" == "true" ]]; then
  NO_PROVIDERS=true
  PROFILE="ci"
fi

for arg in "$@"; do
  case "$arg" in
    --ci)             NO_PROVIDERS=true; PROFILE="ci" ;;
    --reuse-gateway)  REUSE_GATEWAY=true ;;
    --no-providers)   NO_PROVIDERS=true ;;
    --agent=*)        PROFILE="${arg#--agent=}" ;;
    -*)               ;;
    *)                [[ -z "$TARGET" ]] && TARGET="$arg" ;;
  esac
done

if [[ -z "$TARGET" ]]; then
  echo "Usage: $0 <local|kind|ocp|all> [--ci] [--reuse-gateway]"
  exit 1
fi

# ── Helpers ──────────────────────────────────────────────────────────

strip_ansi() {
  sed 's/\x1b\[[0-9;]*m//g'
}

PASS=0
FAIL=0
TOTAL_START=$(date +%s)

step() {
  local label="$1"; shift
  local start=$(date +%s)
  if "$@" &>/dev/null; then
    local elapsed=$(( $(date +%s) - start ))
    printf "  ✓ %-35s (%ds)\n" "$label" "$elapsed"
    ((PASS++))
  else
    local elapsed=$(( $(date +%s) - start ))
    printf "  ✗ %-35s (%ds)\n" "$label" "$elapsed"
    ((FAIL++))
  fi
}

step_fail() {
  local label="$1"; shift
  local start=$(date +%s)
  if ! "$@" &>/dev/null; then
    local elapsed=$(( $(date +%s) - start ))
    printf "  ✓ %-35s (expected failure, %ds)\n" "$label" "$elapsed"
    ((PASS++))
  else
    local elapsed=$(( $(date +%s) - start ))
    printf "  ✗ %-35s (should have failed, %ds)\n" "$label" "$elapsed"
    ((FAIL++))
  fi
}

step_output() {
  local label="$1"; shift
  local start=$(date +%s)
  local out
  out=$("$@" 2>&1)
  local rc=$?
  local elapsed=$(( $(date +%s) - start ))
  if [[ $rc -eq 0 ]]; then
    printf "  ✓ %-35s (%ds)\n" "$label" "$elapsed"
    ((PASS++))
  else
    printf "  ✗ %-35s (%ds)\n" "$label" "$elapsed"
    echo "    $out" | head -3
    ((FAIL++))
  fi
}

check_providers() {
  local count
  count=$("$CLI" provider list 2>/dev/null | awk 'NR>1' | wc -l | tr -d ' ')
  if [[ "$count" -gt 0 ]]; then
    printf "  ✓ %-35s (%s)\n" "providers registered" "${count} providers"
    ((PASS++))
  else
    printf "  ✗ %-35s\n" "providers registered (0)"
    ((FAIL++))
  fi
}

sandbox_wait() {
  local name="$1"
  for i in $(seq 1 60); do
    local phase
    phase=$("$CLI" sandbox list 2>/dev/null | strip_ansi | awk -v n="$name" '$1==n {print $NF}')
    [[ "$phase" == "Ready" ]] && return 0

    if kubectl get pods -n openshell 2>/dev/null | grep -q "ImagePullBackOff\|ErrImagePull\|CrashLoopBackOff"; then
      local bad
      bad=$(kubectl get pods -n openshell 2>/dev/null | grep "ImagePullBackOff\|ErrImagePull\|CrashLoopBackOff" | awk '{print $1, $3}' | head -3)
      echo "  ERROR: k8s pod in bad state: $bad" >&2
      return 1
    fi
    sleep 2
  done
  return 1
}

sandbox_verify() {
  local name="$1"
  local phase
  if ! sandbox_wait "$name"; then
    phase=$("$CLI" sandbox list 2>/dev/null | strip_ansi | awk -v n="$name" '$1==n {print $NF}')
    printf "  ✗ %-35s %s\n" "sandbox ready" "(phase: ${phase:-not found})"
    ((FAIL++))
    return
  fi
  phase="Ready"
  if [[ "$phase" != "Ready" ]]; then
    printf "  ✗ %-35s\n" "sandbox ready"
    ((FAIL++))
    return
  fi
  printf "  ✓ %-35s\n" "sandbox ready"
  ((PASS++))

  sleep 2
  step "sandbox: exec" "$CLI" sandbox exec --name "$name" -- echo "hello"

  if $NO_PROVIDERS; then
    return
  fi

  step "sandbox: env vars" "$CLI" sandbox exec --name "$name" -- bash -c 'test -n "$ANTHROPIC_BASE_URL"'
  step "sandbox: gws token placeholder" "$CLI" sandbox exec --name "$name" -- bash -c 'echo "$GOOGLE_WORKSPACE_CLI_TOKEN" | grep -q "openshell:resolve:env"'
  step "sandbox: gws api call" "$CLI" sandbox exec --name "$name" -- bash -c 'for i in 1 2 3; do curl -sf https://gmail.googleapis.com/gmail/v1/users/me/profile -H "Authorization: Bearer $GOOGLE_WORKSPACE_CLI_TOKEN" -o /dev/null && exit 0; sleep 3; done; exit 1'
  step "sandbox: mcp config" "$CLI" sandbox exec --name "$name" -- test -f /sandbox/.mcp.json
  step_output "sandbox: claude responds" "$CLI" sandbox exec --name "$name" -- bash -c 'echo "respond with ok" | claude --bare --print 2>&1 | head -1'
}

summary() {
  local total=$(( PASS + FAIL ))
  local elapsed=$(( $(date +%s) - TOTAL_START ))
  echo ""
  if [[ $FAIL -eq 0 ]]; then
    echo "${PASS}/${total} passed (${elapsed}s)"
  else
    echo "${PASS}/${total} passed, ${FAIL} failed (${elapsed}s)"
  fi
}

# ── Error scenarios ─────────────────────────────────────────────────

test_errors() {
  echo "=== test: error scenarios ==="

  step_fail "nonexistent profile" "$HARNESS" up --local --agent nonexistent --no-tty

  if $REUSE_GATEWAY; then
    step "teardown (first)" "$HARNESS" teardown --sandboxes --providers
    step "teardown (second)" "$HARNESS" teardown --sandboxes --providers
  else
    step "teardown (first)" "$HARNESS" teardown --sandboxes --providers --k8s
    step "teardown (second)" "$HARNESS" teardown --sandboxes --providers --k8s
  fi

  echo ""
}

# ── Local flow ───────────────────────────────────────────────────────

test_local() {
  local mode="full"
  $NO_PROVIDERS && mode="$mode, no-providers"
  echo "=== test-flow: local ($mode) ==="

  step "teardown" "$HARNESS" teardown --sandboxes --providers
  step "deploy" "$HARNESS" deploy --local

  if ! $NO_PROVIDERS; then
    step "setup providers" "$HARNESS" providers
    step "gateway reachable" "$CLI" inference get
    check_providers
  else
    step "gateway reachable" "$HARNESS" deploy --local
  fi

  local sandbox_name="test-agent"
  step_output "sandbox create (up)" "$HARNESS" up --local --name "$sandbox_name" --agent "$PROFILE" --no-tty
  sandbox_verify "$sandbox_name"
  step "sandbox delete" "$CLI" sandbox delete "$sandbox_name"

  local create_name="test-create"
  step_output "sandbox create (create)" "$HARNESS" create --name "$create_name" --agent ci
  step "sandbox verify (create)" "$CLI" sandbox exec --name "$create_name" -- echo "hello"
  step "sandbox delete (create)" "$CLI" sandbox delete "$create_name"

  if ! $NO_PROVIDERS; then
    echo ""
    echo "=== test: missing providers ==="
    step "teardown providers" "$HARNESS" teardown --providers
    step_output "up with no providers" "$HARNESS" up --local --name test-noprov --no-tty
    step "cleanup" "$HARNESS" teardown --sandboxes
  fi

  step "teardown (clean)" "$HARNESS" teardown --sandboxes --providers
}

# ── GWS lifecycle test ───────────────────────────────────────────────

test_gws() {
  local sandbox_name="$1"
  echo "=== test: GWS token lifecycle ==="

  step "gws: token is placeholder" \
    "$CLI" sandbox exec --name "$sandbox_name" -- bash -c \
      'echo "$GOOGLE_WORKSPACE_CLI_TOKEN" | grep -q "openshell:resolve:env"'

  step "gws: Gmail API via proxy" \
    "$CLI" sandbox exec --name "$sandbox_name" -- bash -c 'curl -sf https://gmail.googleapis.com/gmail/v1/users/me/profile -H "Authorization: Bearer $GOOGLE_WORKSPACE_CLI_TOKEN" -o /dev/null'

  openshell provider refresh rotate gws \
    --credential-key GOOGLE_WORKSPACE_CLI_TOKEN &>/dev/null
  step "gws: works after rotation" \
    "$CLI" sandbox exec --name "$sandbox_name" -- bash -c 'curl -sf https://gmail.googleapis.com/gmail/v1/users/me/profile -H "Authorization: Bearer $GOOGLE_WORKSPACE_CLI_TOKEN" -o /dev/null'

  echo ""
}

# ── kind flow ────────────────────────────────────────────────────────

test_kind() {
  local mode="full"
  $NO_PROVIDERS && mode="$mode, no-providers"
  echo "=== test-flow: kind ($mode) ==="

  if ! kubectl get nodes &>/dev/null; then
    echo "  ERROR: no kind cluster — run: kind create cluster --name openshell"
    ((FAIL++))
    return
  fi

  step "teardown" "$HARNESS" teardown --sandboxes --providers --k8s
  step "deploy" "$HARNESS" deploy kind

  if ! $NO_PROVIDERS; then
    step "setup providers" "$HARNESS" providers
    step "gateway reachable" "$CLI" inference get
    check_providers
  fi

  local sandbox_name="test-kind"
  step_output "sandbox create" "$HARNESS" up --name "$sandbox_name" --agent "$PROFILE" --no-tty
  sandbox_verify "$sandbox_name"

  if ! $NO_PROVIDERS; then
    test_gws "$sandbox_name"
  fi

  step "sandbox delete" "$CLI" sandbox delete "$sandbox_name"

  step "teardown (clean)" "$HARNESS" teardown --sandboxes --providers --k8s
  echo ""
}

# ── OCP flow ─────────────────────────────────────────────────────────

test_ocp() {
  local mode="full"
  $REUSE_GATEWAY && mode="$mode, reuse-gateway"
  echo "=== test-flow: ocp ($mode) ==="

  if $REUSE_GATEWAY; then
    OCP_GW=$("$CLI" gateway list 2>/dev/null | strip_ansi | awk '/-remote-/ {gsub(/^\*/, ""); print $1; exit}')
    [[ -n "$OCP_GW" ]] && "$CLI" gateway select "$OCP_GW" 2>/dev/null || true

    step "teardown sandboxes+providers" "$HARNESS" teardown --sandboxes --providers
    if ! "$CLI" inference get &>/dev/null; then
      step "deploy" "$HARNESS" deploy --remote
    else
      step "gateway reachable" "$CLI" inference get
    fi
  else
    step "teardown" "$HARNESS" teardown --sandboxes --providers --k8s
    step "deploy" "$HARNESS" deploy --remote
  fi

  if ! $NO_PROVIDERS; then
    step "setup providers" "$HARNESS" providers
    step "gateway reachable" "$CLI" inference get
    check_providers
  fi

  local sandbox_name
  if $NO_PROVIDERS; then
    sandbox_name="test-ocp"
    step_output "sandbox create" "$HARNESS" create --agent=ci --name "$sandbox_name"
  else
    sandbox_name="agent"
    step_output "sandbox create (up)" "$HARNESS" up --remote --name "$sandbox_name" --no-tty
  fi

  sandbox_verify "$sandbox_name"
  step "sandbox delete" "$CLI" sandbox delete "$sandbox_name"

  if $REUSE_GATEWAY; then
    step "teardown (sandboxes+providers)" "$HARNESS" teardown --sandboxes --providers
  else
    step "teardown (clean)" "$HARNESS" teardown --sandboxes --providers --k8s
  fi
}

# ── Main ─────────────────────────────────────────────────────────────

test_errors

case "$TARGET" in
  local|podman) test_local ;;
  kind)   test_kind ;;
  ocp)    test_ocp ;;
  all)    test_local; echo ""; test_kind; echo ""; test_ocp ;;
  *)
    echo "Unknown target: $TARGET"
    echo "Usage: $0 <local|kind|ocp|all> [--ci] [--reuse-gateway]"
    exit 1
    ;;
esac

summary
exit $FAIL
