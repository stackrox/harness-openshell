#!/usr/bin/env bash
# End-to-end validation for podman and OCP flows.
#
# Supports both the bash (bin/harness) and Go (./harness) entry points.
# Use --go to test the Go binary. Use --full for sandbox lifecycle tests.
#
# Usage:
#   ./test-flow.sh podman                # quick bash: deploy + providers + teardown
#   ./test-flow.sh podman --full         # full bash: + sandbox + verify integrations
#   ./test-flow.sh podman --go           # quick Go: same flow via Go binary
#   ./test-flow.sh podman --full --go    # full Go
#   ./test-flow.sh ocp [--full] [--go]   # OCP variants
#   ./test-flow.sh all [--full] [--go]   # both platforms
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
source "$SCRIPT_DIR/bin/scripts/lib/profile.sh"
CLI="${OPENSHELL_CLI:-openshell}"

# ── Parse args ──────────────────────────────────────────────────────
TARGET=""
FULL=false
USE_GO=false

for arg in "$@"; do
  case "$arg" in
    --full) FULL=true ;;
    --go)   USE_GO=true ;;
    -*)     ;;
    *)      [[ -z "$TARGET" ]] && TARGET="$arg" ;;
  esac
done

if [[ -z "$TARGET" ]]; then
  echo "Usage: $0 <podman|ocp|all> [--full] [--go]"
  exit 1
fi

# ── Entry point: bash or Go ─────────────────────────────────────────
if $USE_GO; then
  HARNESS="$SCRIPT_DIR/harness"
  if [[ ! -x "$HARNESS" ]]; then
    echo "ERROR: Go binary not found at $HARNESS"
    echo "  Build it first: make cli"
    exit 1
  fi
  IMPL="go"
else
  HARNESS="$SCRIPT_DIR/bin/harness"
  IMPL="bash"
fi

# ── Helpers ──────────────────────────────────────────────────────────

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
    echo "${PASS}/${total} passed (${elapsed}s) [${IMPL}]"
  else
    echo "${PASS}/${total} passed, ${FAIL} failed (${elapsed}s) [${IMPL}]"
  fi
}

# ── Error scenarios (run on both paths) ──────────────────────────────

test_errors() {
  echo "=== test: error scenarios ($IMPL) ==="

  # Bad profile
  step_fail "nonexistent profile" "$HARNESS" new --local --profile nonexistent --no-tty

  # Teardown idempotency
  step "teardown (first)" "$HARNESS" teardown
  step "teardown (second)" "$HARNESS" teardown

  echo ""
}

# ── Podman flow ──────────────────────────────────────────────────────

test_podman() {
  local mode="quick"
  $FULL && mode="full"
  echo "=== test-flow: podman ($mode) [$IMPL] ==="

  step "teardown" "$HARNESS" teardown --sandboxes --providers
  step "deploy" "$HARNESS" deploy --local
  step "setup providers" "$HARNESS" providers
  step "gateway reachable" "$CLI" inference get
  check_providers

  if $FULL; then
    local sandbox_name="test-agent"
    step_output "sandbox create" "$HARNESS" new --local --name "$sandbox_name" --no-tty
    sandbox_verify "$sandbox_name"
    step "sandbox delete" "$CLI" sandbox delete "$sandbox_name"

    # Missing providers scenario
    echo ""
    echo "=== test: missing providers ($IMPL) ==="
    step "teardown providers" "$HARNESS" teardown --providers
    step_output "new with no providers" "$HARNESS" new --local --name test-noprov --no-tty
    step "cleanup" "$HARNESS" teardown --sandboxes
  fi

  step "teardown (clean)" "$HARNESS" teardown --sandboxes --providers
}

# ── OCP flow ─────────────────────────────────────────────────────────

test_ocp() {
  local mode="quick"
  $FULL && mode="full"
  echo "=== test-flow: ocp ($mode) [$IMPL] ==="

  step "teardown" "$HARNESS" teardown
  step "deploy" "$HARNESS" deploy --remote
  step "setup creds" "$SCRIPT_DIR/bin/scripts/creds.sh"
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

  step "teardown (clean)" "$HARNESS" teardown
}

# ── Main ─────────────────────────────────────────────────────────────

test_errors

case "$TARGET" in
  podman) test_podman ;;
  ocp)    test_ocp ;;
  all)    test_podman; echo ""; test_ocp ;;
  *)
    echo "Unknown target: $TARGET"
    echo "Usage: $0 <podman|ocp|all> [--full] [--go]"
    exit 1
    ;;
esac

summary
exit $FAIL
