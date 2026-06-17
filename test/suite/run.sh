#!/usr/bin/env bash
# Configuration test suite for harness-openshell.
#
# Tests config resolution, output formats, provider registration, and
# end-to-end agent functionality. Each test verifies one code path.
#
# Usage:
#   ./test/suite/run.sh                    # offline tests only
#   ./test/suite/run.sh --live             # include gateway + sandbox tests
#   ./test/suite/run.sh --filter parse     # run only tests matching "parse"
#   ./test/suite/run.sh --verbose          # show output on failure
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
HARNESS="$SCRIPT_DIR/harness"
CLI="${OPENSHELL_CLI:-openshell}"
CONFIGS="$SCRIPT_DIR/test/configs"
PROVIDERS="$SCRIPT_DIR/test/configs/providers"

if [[ ! -x "$HARNESS" ]]; then
  echo "ERROR: Binary not found at $HARNESS — run: make cli"
  exit 1
fi

LIVE=false
FILTER=""
VERBOSE=false
for arg in "$@"; do
  case "$arg" in
    --live)       LIVE=true ;;
    --filter=*)   FILTER="${arg#--filter=}" ;;
    --verbose)    VERBOSE=true ;;
    *) echo "Unknown: $arg"; exit 1 ;;
  esac
done

PASS=0
FAIL=0
SKIP=0
TOTAL_START=$(date +%s)
SANDBOXES_TO_CLEAN=()

cleanup() {
  for s in "${SANDBOXES_TO_CLEAN[@]}"; do
    "$HARNESS" delete "$s" >/dev/null 2>&1 || true
  done
}
trap cleanup EXIT

run_test() {
  local name="$1"; shift
  [[ -n "$FILTER" ]] && [[ "$name" != *"$FILTER"* ]] && return
  local start=$(date +%s)
  local tmpout
  tmpout=$(mktemp)
  if "$@" >"$tmpout" 2>&1; then
    printf "  ✓ %-50s (%ds)\n" "$name" "$(( $(date +%s) - start ))"
    ((PASS++))
  else
    printf "  ✗ %-50s (%ds)\n" "$name" "$(( $(date +%s) - start ))"
    $VERBOSE && cat "$tmpout"
    ((FAIL++))
  fi
  rm -f "$tmpout"
}

run_test_fail() {
  local name="$1"; shift
  [[ -n "$FILTER" ]] && [[ "$name" != *"$FILTER"* ]] && return
  if ! "$@" >/dev/null 2>&1; then
    printf "  ✓ %-50s (expected fail)\n" "$name"
    ((PASS++))
  else
    printf "  ✗ %-50s (should have failed)\n" "$name"
    ((FAIL++))
  fi
}

skip_test() {
  local name="$1" reason="$2"
  [[ -n "$FILTER" ]] && [[ "$name" != *"$FILTER"* ]] && return
  printf "  - %-50s (skip: %s)\n" "$name" "$reason"
  ((SKIP++))
}

wait_sandbox() {
  local name="$1"
  for i in $(seq 1 10); do
    "$HARNESS" describe "$name" >/dev/null 2>&1 && return 0
    sleep 0.5
  done
  return 1
}

# ── 1. Config Parsing (9 tests) ─────────────────────────────────

echo "=== Config parsing ==="

run_test "parse: minimal agent (no providers)" \
  "$HARNESS" apply --dry-run -f "$CONFIGS/agent-minimal.yaml"

run_test "parse: multi-provider agent" \
  "$HARNESS" apply --dry-run -f "$CONFIGS/agent-multi-provider.yaml"

run_test "parse: task agent" \
  "$HARNESS" apply --dry-run -f "$CONFIGS/agent-task.yaml"

run_test "parse: multi-doc harness yaml" \
  "$HARNESS" apply --dry-run -f "$CONFIGS/harness-multidoc.yaml"

run_test "parse: harness with policy" \
  "$HARNESS" apply --dry-run -f "$CONFIGS/harness-with-policy.yaml"

run_test "parse: default agent (no -f)" \
  "$HARNESS" apply --dry-run

run_test "parse: custom provider profile" \
  "$HARNESS" apply --dry-run -f "$CONFIGS/agent-groq.yaml"

run_test "parse: harness with payloads" \
  "$HARNESS" apply --dry-run -f "$CONFIGS/harness-with-payloads.yaml"

run_test "output: kind: payload in -o yaml" \
  bash -c '"$1" apply -o yaml -f "$2" | grep "kind: payload"' _ "$HARNESS" "$CONFIGS/harness-with-payloads.yaml"

run_test_fail "parse: nonexistent file rejects" \
  "$HARNESS" apply --dry-run -f "/nonexistent/agent.yaml"

run_test_fail "parse: invalid yaml rejects" \
  bash -c 'f=$(mktemp); echo "name: [broken" > "$f"; "$1" apply --dry-run -f "$f"; rc=$?; rm -f "$f"; exit $rc' _ "$HARNESS"

echo ""

# ── 2. Output Rendering (5 tests) ───────────────────────────────

echo "=== Output rendering ==="

run_test "output: -o yaml contains kind: agent" \
  bash -c '"$1" apply -o yaml -f "$2" | grep "kind: agent"' _ "$HARNESS" "$CONFIGS/agent-minimal.yaml"

run_test "output: -o json is valid" \
  bash -c '"$1" apply -o json -f "$2" | python3 -m json.tool >/dev/null' _ "$HARNESS" "$CONFIGS/agent-minimal.yaml"

run_test "output: multidoc -o yaml includes provider" \
  bash -c '"$1" apply -o yaml -f "$2" | grep "kind: provider"' _ "$HARNESS" "$CONFIGS/harness-multidoc.yaml"

run_test "output: static env in rendered yaml" \
  bash -c '"$1" apply -o yaml -f "$2" | grep "hello-world"' _ "$HARNESS" "$CONFIGS/agent-custom-env.yaml"

run_test "output: env template preserved (not expanded)" \
  bash -c '"$1" apply -o yaml -f "$2" | grep -F '"'"'${USER}'"'"'' _ "$HARNESS" "$CONFIGS/agent-custom-env.yaml"

echo ""

# ── 3. CLI Flags (4 tests) ──────────────────────────────────────

echo "=== CLI flags ==="

run_test "flags: --agent default" \
  "$HARNESS" apply --dry-run --agent default

run_test_fail "flags: --agent nonexistent rejects" \
  "$HARNESS" apply --dry-run --agent nonexistent

run_test_fail "flags: --gateway + --gateway-profile rejects" \
  "$HARNESS" apply --dry-run --gateway local --gateway-profile /dev/null

run_test_fail "flags: delete with no args rejects" \
  "$HARNESS" delete

echo ""

# ── 4. Get/Describe (3 tests, need gateway) ─────────────────────

echo "=== Get/describe ==="

if "$CLI" inference get >/dev/null 2>&1; then
  run_test "get: agents -o json (valid, empty)" \
    bash -c '"$1" get agents -o json | python3 -m json.tool >/dev/null' _ "$HARNESS"

  run_test_fail "get: -o invalid rejects" \
    "$HARNESS" get agents -o csv

  run_test_fail "describe: nonexistent rejects" \
    "$HARNESS" describe nonexistent
else
  skip_test "get: agents -o json (valid, empty)" "no gateway"
  skip_test "get: -o invalid rejects" "no gateway"
  skip_test "describe: nonexistent rejects" "no gateway"
fi

echo ""

# ── 5. Live Sandbox Lifecycle (5 tests) ─────────────────────────

if $LIVE && "$CLI" inference get >/dev/null 2>&1; then
  echo "=== Live sandbox lifecycle ==="

  # Use ci-agent.yaml for lifecycle tests -- it specifies the community base
  # image which is available on CI without a prior image build.
  CI_AGENT="$SCRIPT_DIR/test/ci-agent.yaml"
  SANDBOXES_TO_CLEAN+=(test-lifecycle test-env-check)

  run_test "live: create + describe" \
    bash -c '"$1" apply -f "$2" --name test-lifecycle && \
      for i in $(seq 1 10); do "$1" describe test-lifecycle >/dev/null 2>&1 && exit 0; sleep 0.5; done; exit 1' _ "$HARNESS" "$CI_AGENT"

  run_test "live: get agents shows sandbox" \
    bash -c '"$1" get agents | grep test-lifecycle' _ "$HARNESS"

  run_test "live: exec in sandbox" \
    "$CLI" sandbox exec --name test-lifecycle -- echo "alive"

  run_test "live: env injection + verification" \
    bash -c '"$1" apply -f "$2" --name test-env-check && \
      for i in $(seq 1 10); do "$1" describe test-env-check >/dev/null 2>&1 && break; sleep 0.5; done && \
      "$3" sandbox exec --name test-env-check -- bash -c "test \"\$STATIC_VAR\" = \"hello-world\""' _ "$HARNESS" "$CONFIGS/agent-env-ci.yaml" "$CLI"

  run_test "live: delete + verify gone" \
    bash -c '"$1" delete test-lifecycle && \
      sleep 1 && \
      ! "$1" get agents 2>/dev/null | grep -q test-lifecycle' _ "$HARNESS"

  "$HARNESS" delete --sandboxes >/dev/null 2>&1 || true

  echo ""
else
  echo "=== Live sandbox lifecycle (skipped: use --live) ==="
  echo ""
fi

# ── 6. Provider Registration (5 tests) ─────────────────────────

if $LIVE && "$CLI" inference get >/dev/null 2>&1; then
  echo "=== Provider registration ==="

  "$HARNESS" delete --sandboxes --providers >/dev/null 2>&1 || true

  if [[ -n "${GITHUB_TOKEN:-}" ]]; then
    run_test "provider: github (from-existing)" \
      bash -c '"$1" apply -f "$2" --name test-prov-gh && \
        "$3" provider list 2>/dev/null | grep -q github && \
        "$1" delete test-prov-gh >/dev/null 2>&1' _ "$HARNESS" "$CONFIGS/agent-github-only.yaml" "$CLI"
  else
    skip_test "provider: github (from-existing)" "GITHUB_TOKEN not set"
  fi

  if [[ -n "${JIRA_API_TOKEN:-}" ]]; then
    run_test "provider: atlassian (basic-auth)" \
      bash -c '"$1" apply -f "$2" --name test-prov-atl && \
        "$3" provider list 2>/dev/null | grep -q atlassian && \
        "$1" delete test-prov-atl >/dev/null 2>&1' _ "$HARNESS" "$CONFIGS/agent-atlassian.yaml" "$CLI"
  else
    skip_test "provider: atlassian (basic-auth)" "JIRA_API_TOKEN not set"
  fi

  if [[ -n "${ANTHROPIC_VERTEX_PROJECT_ID:-}" ]]; then
    run_test "provider: vertex (adc + inference route)" \
      bash -c '"$1" apply -f "$2" --name test-prov-vtx && \
        "$3" provider list 2>/dev/null | grep -q vertex && \
        "$3" inference get 2>/dev/null && \
        "$1" delete test-prov-vtx >/dev/null 2>&1' _ "$HARNESS" "$CONFIGS/agent-vertex.yaml" "$CLI"
  else
    skip_test "provider: vertex (adc + inference route)" "ANTHROPIC_VERTEX_PROJECT_ID not set"
  fi

  if command -v gws >/dev/null 2>&1 && gws auth export --unmasked >/dev/null 2>&1; then
    run_test "provider: gws (oauth-refresh)" \
      bash -c '"$1" apply -f "$2" --name test-prov-gws && \
        "$3" provider list 2>/dev/null | grep -q gws && \
        "$1" delete test-prov-gws >/dev/null 2>&1' _ "$HARNESS" "$CONFIGS/agent-gws.yaml" "$CLI"
  else
    skip_test "provider: gws (oauth-refresh)" "gws not authenticated"
  fi

  if [[ -n "${GITHUB_TOKEN:-}" ]] && [[ -n "${JIRA_API_TOKEN:-}" ]] && [[ -n "${ANTHROPIC_VERTEX_PROJECT_ID:-}" ]]; then
    "$HARNESS" delete --sandboxes --providers >/dev/null 2>&1 || true
    run_test "provider: all-providers composition" \
      bash -c '"$1" apply -f "$2" --name test-prov-all && \
        count=$("$1" get providers -o json 2>/dev/null | python3 -c "import sys,json; print(len(json.load(sys.stdin)))") && \
        test "$count" -ge 3 && \
        "$1" delete test-prov-all >/dev/null 2>&1' _ "$HARNESS" "$CONFIGS/agent-all-providers.yaml"
  else
    skip_test "provider: all-providers composition" "missing credentials"
  fi

  "$HARNESS" delete --sandboxes --providers >/dev/null 2>&1 || true

  echo ""
else
  echo "=== Provider registration (skipped: use --live) ==="
  echo ""
fi

# ── 7. Agent Integration (4 tests) ─────────────────────────────

if $LIVE && "$CLI" inference get >/dev/null 2>&1; then
  echo "=== Agent integration ==="

  # These tests use the default all-providers config for full policy.
  # They verify that claude/opencode can actually use providers to do work.

  if [[ -n "${ANTHROPIC_VERTEX_PROJECT_ID:-}" ]]; then
    SANDBOXES_TO_CLEAN+=(test-agent-int)

    "$HARNESS" delete --sandboxes --providers >/dev/null 2>&1 || true
    "$HARNESS" apply --name test-agent-int >/dev/null 2>&1

    if wait_sandbox test-agent-int; then
      run_test "agent: claude inference via vertex" \
        "$CLI" sandbox exec --name test-agent-int -- \
          bash -c 'result=$(echo "respond with ok" | claude --print 2>&1); test -n "$result"'

      # OpenCode uses ANTHROPIC_BASE_URL=inference.local/v1 (note /v1 suffix)
      SANDBOXES_TO_CLEAN+=(test-opencode-int)
      run_test "agent: opencode inference via vertex" \
        bash -c '"$1" apply -f "$2" --name test-opencode-int >/dev/null 2>&1 && \
          for i in $(seq 1 10); do "$1" describe test-opencode-int >/dev/null 2>&1 && break; sleep 0.5; done && \
          result=$("$3" sandbox exec --name test-opencode-int -- bash -c "opencode run \"respond with ok\" 2>&1") && \
          test -n "$result"' _ "$HARNESS" "$CONFIGS/agent-opencode-vertex.yaml" "$CLI"

      if [[ -n "${GITHUB_TOKEN:-}" ]]; then
        run_test "agent: github via gh cli" \
          "$CLI" sandbox exec --name test-agent-int -- \
            bash -c 'gh api user --jq .login 2>/dev/null | grep -q .'
      else
        skip_test "agent: github via gh cli" "GITHUB_TOKEN not set"
      fi

      if [[ -n "${JIRA_API_TOKEN:-}" ]]; then
        # LLM output is non-deterministic. We verify claude can invoke the
        # jira MCP tool (mcp-atlassian) and get a response, not specific content.
        run_test "agent: claude uses jira mcp" \
          "$CLI" sandbox exec --name test-agent-int -- \
            bash -c 'result=$(echo "use the jira_get_user_profile mcp tool and respond with the result" | claude --print 2>&1); test -n "$result"'
      else
        skip_test "agent: claude uses jira mcp" "JIRA credentials not set"
      fi

      if command -v gws >/dev/null 2>&1 && gws auth export --unmasked >/dev/null 2>&1; then
        run_test "agent: gws gmail via proxy token" \
          "$CLI" sandbox exec --name test-agent-int -- \
            bash -c 'for i in 1 2 3; do curl -sf https://gmail.googleapis.com/gmail/v1/users/me/profile -H "Authorization: Bearer $GOOGLE_WORKSPACE_CLI_TOKEN" -o /dev/null && exit 0; sleep 2; done; exit 1'
      else
        skip_test "agent: gws gmail via proxy token" "gws not authenticated"
      fi
    else
      skip_test "agent: claude inference via vertex" "sandbox not ready"
      skip_test "agent: github via gh cli" "sandbox not ready"
      skip_test "agent: claude uses jira mcp" "sandbox not ready"
      skip_test "agent: gws gmail via proxy token" "sandbox not ready"
    fi

    # Test the built-in opencode profile (--agent opencode) with all providers.
    # Verifies the shipped profile works end-to-end with Vertex via inference.local/v1.
    SANDBOXES_TO_CLEAN+=(test-oc-builtin)
    SANDBOXES_TO_CLEAN+=(test-oc-builtin)
    run_test "agent: opencode built-in profile" \
      bash -c '"$1" apply --agent opencode --name test-oc-builtin >/dev/null 2>&1 && \
        for i in $(seq 1 10); do "$1" describe test-oc-builtin >/dev/null 2>&1 && break; sleep 0.5; done && \
        result=$("$2" sandbox exec --name test-oc-builtin -- bash -c "opencode run \"respond with ok\" 2>&1") && \
        test -n "$result"' _ "$HARNESS" "$CLI"

    "$HARNESS" delete test-agent-int test-opencode-int test-oc-builtin >/dev/null 2>&1 || true
    "$HARNESS" delete --sandboxes --providers >/dev/null 2>&1 || true
  else
    skip_test "agent: claude inference via vertex" "ANTHROPIC_VERTEX_PROJECT_ID not set"
    skip_test "agent: github via gh cli" "ANTHROPIC_VERTEX_PROJECT_ID not set"
    skip_test "agent: claude uses jira mcp" "ANTHROPIC_VERTEX_PROJECT_ID not set"
    skip_test "agent: gws gmail via proxy token" "ANTHROPIC_VERTEX_PROJECT_ID not set"
  fi

  echo ""
else
  echo "=== Agent integration (skipped: use --live) ==="
  echo ""
fi

# ── Summary ─────────────────────────────────────────────────────

TOTAL=$(( PASS + FAIL ))
ELAPSED=$(( $(date +%s) - TOTAL_START ))
echo ""
if [[ $FAIL -eq 0 ]]; then
  echo "${PASS}/${TOTAL} passed, ${SKIP} skipped (${ELAPSED}s)"
else
  echo "${PASS}/${TOTAL} passed, ${FAIL} failed, ${SKIP} skipped (${ELAPSED}s)"
  exit 1
fi
