#!/usr/bin/env bash
# End-to-end validation for podman and OCP flows.
#
# Usage:
#   ./test-flow.sh podman          # quick: deploy + providers + teardown
#   ./test-flow.sh podman --full   # full: + sandbox + verify integrations
#   ./test-flow.sh ocp             # quick: deploy + creds + providers + teardown
#   ./test-flow.sh ocp --full      # full: + sandbox + verify integrations
#   ./test-flow.sh all             # quick for both
#   ./test-flow.sh all --full      # full for both
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
CLI="${OPENSHELL_CLI:-openshell}"

TARGET="${1:-}"
FULL=false
[[ "${2:-}" == "--full" || "${3:-}" == "--full" ]] && FULL=true

if [[ -z "$TARGET" ]]; then
  echo "Usage: $0 <podman|ocp|all> [--full]"
  exit 1
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
  # Check sandbox is ready
  local phase
  phase=$("$CLI" sandbox list 2>/dev/null | sed 's/\x1b\[[0-9;]*m//g' | awk -v n="$name" '$1==n {print $NF}')
  if [[ "$phase" != "Ready" ]]; then
    printf "  ✗ %-35s\n" "sandbox ready"
    ((FAIL++))
    return
  fi
  printf "  ✓ %-35s\n" "sandbox ready"
  ((PASS++))

  # Check env vars
  step "sandbox: env vars" "$CLI" sandbox exec --name "$name" -- bash -c 'test -n "$ANTHROPIC_BASE_URL"'

  # Check GWS credentials
  step "sandbox: gws creds" "$CLI" sandbox exec --name "$name" -- test -f /sandbox/.config/openshell/credentials.json

  # Check MCP config
  step "sandbox: mcp config" "$CLI" sandbox exec --name "$name" -- test -f /sandbox/.mcp.json

  # Check Claude responds
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

# ── Podman flow ──────────────────────────────────────────────────────

test_podman() {
  local mode="quick"
  $FULL && mode="full"
  echo "=== test-flow: podman ($mode) ==="

  step "teardown" "$SCRIPT_DIR/teardown.sh" --sandboxes --providers
  step "deploy" "$SCRIPT_DIR/deploy-podman.sh"
  step "setup providers" "$SCRIPT_DIR/setup-providers.sh"
  step "gateway reachable" "$CLI" inference get
  check_providers

  if $FULL; then
    local sandbox_name="test-agent"
    step_output "sandbox create" "$SCRIPT_DIR/sandbox-podman.sh" --name "$sandbox_name" --no-tty
    sandbox_verify "$sandbox_name"
    step "sandbox delete" "$CLI" sandbox delete "$sandbox_name"
  fi

  step "teardown (clean)" "$SCRIPT_DIR/teardown.sh" --sandboxes --providers
}

# ── OCP flow ─────────────────────────────────────────────────────────

test_ocp() {
  local mode="quick"
  $FULL && mode="full"
  echo "=== test-flow: ocp ($mode) ==="

  step "teardown" "$SCRIPT_DIR/teardown.sh"
  step "deploy" "$SCRIPT_DIR/deploy-ocp.sh"
  step "setup creds" "$SCRIPT_DIR/setup-creds.sh"
  step "setup providers" "$SCRIPT_DIR/setup-providers.sh"
  step "gateway reachable" "$CLI" inference get
  check_providers

  if $FULL; then
    step_output "sandbox create" "$SCRIPT_DIR/sandbox-ocp.sh"
    local sandbox_name="agent"

    # Wait for ready
    for i in $(seq 1 30); do
      local phase=$("$CLI" sandbox list 2>/dev/null | sed 's/\x1b\[[0-9;]*m//g' | awk -v n="$sandbox_name" '$1==n {print $NF}')
      [[ "$phase" == "Ready" ]] && break
      sleep 2
    done
    sandbox_verify "$sandbox_name"
    step "sandbox delete" "$CLI" sandbox delete "$sandbox_name"
  fi

  step "teardown (clean)" "$SCRIPT_DIR/teardown.sh"
}

# ── Main ─────────────────────────────────────────────────────────────

case "$TARGET" in
  podman)
    test_podman
    ;;
  ocp)
    test_ocp
    ;;
  all)
    test_podman
    echo ""
    test_ocp
    ;;
  *)
    echo "Unknown target: $TARGET"
    echo "Usage: $0 <podman|ocp|all> [--full]"
    exit 1
    ;;
esac

summary
exit $FAIL
