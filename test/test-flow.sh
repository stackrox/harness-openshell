#!/usr/bin/env bash
# End-to-end validation for podman and OCP flows.
#
# Usage:
#   ./test-flow.sh podman                # quick: deploy + providers + teardown
#   ./test-flow.sh podman --full         # full: + sandbox + verify integrations
#   ./test-flow.sh ocp [--full]                  # OCP variants
#   ./test-flow.sh ocp --full --reuse-gateway   # skip deploy/teardown-k8s (~50s vs ~130s)
#   ./test-flow.sh all [--full]                  # both platforms
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
FULL=false
REUSE_GATEWAY=false
NO_PROVIDERS=false
PROFILE="default"

for arg in "$@"; do
  case "$arg" in
    --full)           FULL=true ;;
    --reuse-gateway)  REUSE_GATEWAY=true ;;
    --no-providers)   NO_PROVIDERS=true ;;
    --profile=*)      PROFILE="${arg#--profile=}" ;;
    -*)               ;;
    *)                [[ -z "$TARGET" ]] && TARGET="$arg" ;;
  esac
done

if [[ -z "$TARGET" ]]; then
  echo "Usage: $0 <podman|ocp|all> [--full] [--reuse-gateway]"
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

sandbox_verify() {
  local name="$1"
  local phase
  phase=$("$CLI" sandbox list 2>/dev/null | strip_ansi | awk -v n="$name" '$1==n {print $NF}')
  if [[ "$phase" != "Ready" ]]; then
    printf "  ✗ %-35s\n" "sandbox ready"
    ((FAIL++))
    return
  fi
  printf "  ✓ %-35s\n" "sandbox ready"
  ((PASS++))

  # Basic exec works
  step "sandbox: exec" "$CLI" sandbox exec --name "$name" -- echo "hello"

  if $NO_PROVIDERS; then
    return
  fi

  # Provider-dependent checks (require credentials + inference)
  step "sandbox: env vars" "$CLI" sandbox exec --name "$name" -- bash -c 'test -n "$ANTHROPIC_BASE_URL"'
  step "sandbox: gws creds" "$CLI" sandbox exec --name "$name" -- test -f /sandbox/.config/openshell/credentials.json
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

  # Bad profile
  step_fail "nonexistent profile" "$HARNESS" new --local --profile nonexistent --no-tty

  # Teardown idempotency (skip k8s teardown when reusing gateway)
  if $REUSE_GATEWAY; then
    step "teardown (first)" "$HARNESS" teardown --sandboxes --providers
    step "teardown (second)" "$HARNESS" teardown --sandboxes --providers
  else
    step "teardown (first)" "$HARNESS" teardown --sandboxes --providers --k8s
    step "teardown (second)" "$HARNESS" teardown --sandboxes --providers --k8s
  fi

  echo ""
}

# ── Podman flow ──────────────────────────────────────────────────────

test_podman() {
  local mode="quick"
  $FULL && mode="full"
  $NO_PROVIDERS && mode="$mode, no-providers"
  echo "=== test-flow: podman ($mode) ==="

  step "teardown" "$HARNESS" teardown --sandboxes --providers
  step "deploy" "$HARNESS" deploy --local

  if ! $NO_PROVIDERS; then
    step "setup providers" "$HARNESS" providers
    step "gateway reachable" "$CLI" inference get
    check_providers
  else
    step "gateway reachable" "$HARNESS" deploy --local
  fi

  if $FULL; then
    local sandbox_name="test-agent"
    step_output "sandbox create" "$HARNESS" new --local --name "$sandbox_name" --profile "$PROFILE" --no-tty
    sandbox_verify "$sandbox_name"
    step "sandbox delete" "$CLI" sandbox delete "$sandbox_name"

    if ! $NO_PROVIDERS; then
      # Missing providers scenario
      echo ""
      echo "=== test: missing providers ==="
      step "teardown providers" "$HARNESS" teardown --providers
      step_output "new with no providers" "$HARNESS" new --local --name test-noprov --no-tty
      step "cleanup" "$HARNESS" teardown --sandboxes
    fi
  fi

  step "teardown (clean)" "$HARNESS" teardown --sandboxes --providers
}

# ── OCP flow ─────────────────────────────────────────────────────────

test_ocp() {
  local mode="quick"
  $FULL && mode="full"
  $REUSE_GATEWAY && mode="$mode, reuse-gateway"
  echo "=== test-flow: ocp ($mode) ==="

  if $REUSE_GATEWAY; then
    # Ensure OCP gateway is selected (error scenarios may have switched to local)
    OCP_GW=$("$CLI" gateway list 2>/dev/null | strip_ansi | awk '/-remote-/ {gsub(/^\*/, ""); print $1; exit}')
    [[ -n "$OCP_GW" ]] && "$CLI" gateway select "$OCP_GW" 2>/dev/null || true

    step "teardown sandboxes+providers" "$HARNESS" teardown --sandboxes --providers
    # Deploy only if gateway is not reachable
    if ! "$CLI" inference get &>/dev/null; then
      step "deploy" "$HARNESS" deploy --remote
    else
      step "gateway reachable" "$CLI" inference get
    fi
  else
    step "teardown" "$HARNESS" teardown
    step "deploy" "$HARNESS" deploy --remote
  fi

  step "setup providers" "$HARNESS" providers
  step "gateway reachable" "$CLI" inference get
  check_providers

  if $FULL; then
    step_output "sandbox create" "$HARNESS" new --remote
    local sandbox_name="agent"

    for i in $(seq 1 30); do
      local phase=$("$CLI" sandbox list 2>/dev/null | strip_ansi | awk -v n="$sandbox_name" '$1==n {print $NF}')
      [[ "$phase" == "Ready" ]] && break
      sleep 2
    done
    sandbox_verify "$sandbox_name"
    step "sandbox delete" "$CLI" sandbox delete "$sandbox_name"
  fi

  if $REUSE_GATEWAY; then
    step "teardown (sandboxes+providers)" "$HARNESS" teardown --sandboxes --providers
  else
    step "teardown (clean)" "$HARNESS" teardown
  fi
}

# ── Main ─────────────────────────────────────────────────────────────

test_errors

case "$TARGET" in
  podman) test_podman ;;
  ocp)    test_ocp ;;
  all)    test_podman; echo ""; test_ocp ;;
  *)
    echo "Unknown target: $TARGET"
    echo "Usage: $0 <podman|ocp|all> [--full]"
    exit 1
    ;;
esac

summary
exit $FAIL
