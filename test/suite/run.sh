#!/usr/bin/env bash
# Configuration test suite for harness-openshell.
#
# Tests the harness CLI with different agent configs, provider combinations,
# and credential styles. Complements test-flow.sh (which tests the full
# deploy/sandbox lifecycle) by focusing on config resolution and validation.
#
# Usage:
#   ./test/suite/run.sh                    # all tests
#   ./test/suite/run.sh --live             # include live sandbox tests (needs gateway)
#   ./test/suite/run.sh --filter parse     # run only tests matching "parse"
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
HARNESS="$SCRIPT_DIR/harness"
CLI="${OPENSHELL_CLI:-openshell}"
CONFIGS="$SCRIPT_DIR/test/configs"

if [[ ! -x "$HARNESS" ]]; then
  echo "ERROR: Binary not found at $HARNESS — run: make cli"
  exit 1
fi

LIVE=false
FILTER=""
for arg in "$@"; do
  case "$arg" in
    --live)   LIVE=true ;;
    --filter=*) FILTER="${arg#--filter=}" ;;
    *) echo "Unknown: $arg"; exit 1 ;;
  esac
done

PASS=0
FAIL=0
SKIP=0
TOTAL_START=$(date +%s)

run_test() {
  local name="$1"; shift
  if [[ -n "$FILTER" ]] && [[ "$name" != *"$FILTER"* ]]; then
    return
  fi
  local start=$(date +%s)
  if "$@" >/dev/null 2>&1; then
    local elapsed=$(( $(date +%s) - start ))
    printf "  ✓ %-45s (%ds)\n" "$name" "$elapsed"
    ((PASS++))
  else
    local elapsed=$(( $(date +%s) - start ))
    printf "  ✗ %-45s (%ds)\n" "$name" "$elapsed"
    ((FAIL++))
  fi
}

run_test_expect_fail() {
  local name="$1"; shift
  if [[ -n "$FILTER" ]] && [[ "$name" != *"$FILTER"* ]]; then
    return
  fi
  local start=$(date +%s)
  if ! "$@" >/dev/null 2>&1; then
    local elapsed=$(( $(date +%s) - start ))
    printf "  ✓ %-45s (expected fail, %ds)\n" "$name" "$elapsed"
    ((PASS++))
  else
    local elapsed=$(( $(date +%s) - start ))
    printf "  ✗ %-45s (should have failed, %ds)\n" "$name" "$elapsed"
    ((FAIL++))
  fi
}

skip_test() {
  local name="$1"; shift
  local reason="$1"
  if [[ -n "$FILTER" ]] && [[ "$name" != *"$FILTER"* ]]; then
    return
  fi
  printf "  - %-45s (skip: %s)\n" "$name" "$reason"
  ((SKIP++))
}

# ── Config parsing tests ──────────────────────────────────────────

echo "=== Config parsing ==="

run_test "parse: minimal agent" \
  "$HARNESS" apply --dry-run -f "$CONFIGS/agent-minimal.yaml"

run_test "parse: github-only agent" \
  "$HARNESS" apply --dry-run -f "$CONFIGS/agent-github-only.yaml"

run_test "parse: multi-provider agent" \
  "$HARNESS" apply --dry-run -f "$CONFIGS/agent-multi-provider.yaml"

run_test "parse: custom env agent" \
  "$HARNESS" apply --dry-run -f "$CONFIGS/agent-custom-env.yaml"

run_test "parse: task agent" \
  "$HARNESS" apply --dry-run -f "$CONFIGS/agent-task.yaml"

run_test "parse: multi-doc harness yaml" \
  "$HARNESS" apply --dry-run -f "$CONFIGS/harness-multidoc.yaml"

run_test "parse: harness with policy" \
  "$HARNESS" apply --dry-run -f "$CONFIGS/harness-with-policy.yaml"

run_test "parse: default agent (no -f)" \
  "$HARNESS" apply --dry-run

run_test_expect_fail "parse: nonexistent file" \
  "$HARNESS" apply --dry-run -f "/nonexistent/agent.yaml"

run_test_expect_fail "parse: invalid yaml" \
  bash -c 'echo "name: [broken" | "$HARNESS" apply --dry-run -f /dev/stdin'

echo ""

# ── Output format tests ──────────────────────────────────────────

echo "=== Output formats ==="

run_test "output: apply -o yaml" \
  bash -c '"$1" apply -o yaml -f "$2" | grep "kind: agent"' _ "$HARNESS" "$CONFIGS/agent-minimal.yaml"

run_test "output: apply -o json" \
  bash -c '"$1" apply -o json -f "$2" | python3 -m json.tool >/dev/null' _ "$HARNESS" "$CONFIGS/agent-minimal.yaml"

run_test "output: multidoc -o yaml includes provider" \
  bash -c '"$1" apply -o yaml -f "$2" | grep "kind: provider"' _ "$HARNESS" "$CONFIGS/harness-multidoc.yaml"

# get commands (need gateway)
if "$CLI" inference get >/dev/null 2>&1; then
  run_test "output: get gateways -o json" \
    bash -c '"$1" get gateways -o json | python3 -m json.tool >/dev/null' _ "$HARNESS"

  run_test "output: get gateways -o yaml" \
    bash -c '"$1" get gateways -o yaml | grep "name:"' _ "$HARNESS"

  run_test "output: get agents -o json (empty)" \
    bash -c '"$1" get agents -o json | python3 -m json.tool >/dev/null' _ "$HARNESS"

  run_test "output: get providers -o json (empty)" \
    bash -c '"$1" get providers -o json | python3 -m json.tool >/dev/null' _ "$HARNESS"

  run_test_expect_fail "output: get agents -o invalid" \
    "$HARNESS" get agents -o csv
else
  skip_test "output: get gateways -o json" "no gateway"
  skip_test "output: get gateways -o yaml" "no gateway"
  skip_test "output: get agents -o json" "no gateway"
  skip_test "output: get providers -o json" "no gateway"
  skip_test "output: get agents -o invalid" "no gateway"
fi

echo ""

# ── Env var resolution tests ─────────────────────────────────────

echo "=== Env resolution ==="

run_test "env: static var in -o yaml" \
  bash -c '"$1" apply -o yaml -f "$2" | grep "hello-world"' _ "$HARNESS" "$CONFIGS/agent-custom-env.yaml"

run_test "env: host var expansion" \
  bash -c '"$1" apply -o yaml -f "$2" | grep "$USER"' _ "$HARNESS" "$CONFIGS/agent-custom-env.yaml"

run_test "env: provider env in multi-provider" \
  bash -c '"$1" apply -o yaml -f "$2" | grep "JIRA_URL"' _ "$HARNESS" "$CONFIGS/agent-multi-provider.yaml"

echo ""

# ── CLI flag tests ────────────────────────────────────────────────

echo "=== CLI flags ==="

run_test "flags: --agent default" \
  "$HARNESS" apply --dry-run --agent default

run_test_expect_fail "flags: --agent nonexistent" \
  "$HARNESS" apply --dry-run --agent nonexistent

run_test_expect_fail "flags: --gateway + --gateway-profile" \
  "$HARNESS" apply --dry-run --gateway local --gateway-profile /dev/null

run_test "flags: --name accepted" \
  "$HARNESS" apply --dry-run -f "$CONFIGS/agent-minimal.yaml" --name custom-name

echo ""

# ── Describe/delete tests ─────────────────────────────────────────

echo "=== Describe/delete ==="

run_test_expect_fail "describe: nonexistent sandbox" \
  "$HARNESS" describe nonexistent

run_test_expect_fail "delete: no args or flags" \
  "$HARNESS" delete

echo ""

# ── Live sandbox tests (--live only) ─────────────────────────────

if $LIVE && "$CLI" inference get >/dev/null 2>&1; then
  echo "=== Live sandbox tests ==="

  # Minimal sandbox create/delete
  run_test "live: create minimal sandbox" \
    "$HARNESS" apply -f "$CONFIGS/agent-minimal.yaml" --name test-suite-min

  sleep 2

  run_test "live: describe sandbox" \
    "$HARNESS" describe test-suite-min

  run_test "live: get agents shows sandbox" \
    bash -c '"$1" get agents | grep test-suite-min' _ "$HARNESS"

  run_test "live: exec in sandbox" \
    "$CLI" sandbox exec --name test-suite-min -- echo "alive"

  run_test "live: env var injected" \
    bash -c '"$1" apply -f "$2" --name test-suite-env 2>&1' _ "$HARNESS" "$CONFIGS/agent-custom-env.yaml"

  sleep 2

  run_test "live: static env in sandbox" \
    "$CLI" sandbox exec --name test-suite-env -- bash -c 'test "$STATIC_VAR" = "hello-world"'

  # Cleanup
  run_test "live: delete specific sandbox" \
    "$HARNESS" delete test-suite-min

  run_test "live: delete all sandboxes" \
    "$HARNESS" delete --sandboxes

  echo ""
else
  if ! $LIVE; then
    echo "=== Live sandbox tests (skipped: use --live) ==="
  else
    echo "=== Live sandbox tests (skipped: no gateway) ==="
  fi
  echo ""
fi

# ── Free API provider tests (requires keys) ──────────────────────

echo "=== Free API providers ==="

if [[ -n "${GROQ_API_KEY:-}" ]]; then
  run_test "provider: groq dry-run" \
    bash -c 'cat > /tmp/test-groq.yaml << EOF
name: test-groq
entrypoint: bash
providers: []
env:
  GROQ_API_KEY: \${GROQ_API_KEY}
  GROQ_BASE_URL: https://api.groq.com/openai/v1
EOF
"$1" apply --dry-run -f /tmp/test-groq.yaml' _ "$HARNESS"
else
  skip_test "provider: groq dry-run" "GROQ_API_KEY not set"
fi

if [[ -n "${OPENROUTER_API_KEY:-}" ]]; then
  run_test "provider: openrouter dry-run" \
    bash -c 'cat > /tmp/test-openrouter.yaml << EOF
name: test-openrouter
entrypoint: bash
providers: []
env:
  OPENROUTER_API_KEY: \${OPENROUTER_API_KEY}
  OPENROUTER_BASE_URL: https://openrouter.ai/api/v1
EOF
"$1" apply --dry-run -f /tmp/test-openrouter.yaml' _ "$HARNESS"
else
  skip_test "provider: openrouter dry-run" "OPENROUTER_API_KEY not set"
fi

if [[ -n "${NVIDIA_API_KEY:-}" ]]; then
  run_test "provider: nvidia nim dry-run" \
    bash -c 'cat > /tmp/test-nvidia.yaml << EOF
name: test-nvidia
entrypoint: bash
providers:
  - profile: nvidia
env:
  NVIDIA_API_KEY: \${NVIDIA_API_KEY}
EOF
"$1" apply --dry-run -f /tmp/test-nvidia.yaml' _ "$HARNESS"
else
  skip_test "provider: nvidia nim dry-run" "NVIDIA_API_KEY not set"
fi

echo ""

# ── Summary ──────────────────────────────────────────────────────

TOTAL=$(( PASS + FAIL ))
ELAPSED=$(( $(date +%s) - TOTAL_START ))
echo ""
if [[ $FAIL -eq 0 ]]; then
  echo "${PASS}/${TOTAL} passed, ${SKIP} skipped (${ELAPSED}s)"
else
  echo "${PASS}/${TOTAL} passed, ${FAIL} failed, ${SKIP} skipped (${ELAPSED}s)"
  exit 1
fi
