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

# Write invalid YAML to a temp file (not stdin -- -f reads file paths)
run_test_expect_fail "parse: invalid yaml" \
  bash -c 'echo "name: [broken" > /tmp/test-invalid.yaml && "$1" apply --dry-run -f /tmp/test-invalid.yaml; rc=$?; rm -f /tmp/test-invalid.yaml; exit $rc' _ "$HARNESS"

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

# Note: -o yaml outputs the raw template (${USER}), not the expanded value.
# Env expansion happens at sandbox creation time via BuildEnvMap(), not at render time.
# This test verifies the template reference is preserved in the rendered output.
run_test "env: host var template preserved" \
  bash -c '"$1" apply -o yaml -f "$2" | grep "FROM_HOST"' _ "$HARNESS" "$CONFIGS/agent-custom-env.yaml"

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

# ── Provider registration tests (requires creds + gateway) ───────

echo "=== Provider registration ==="

if $LIVE && "$CLI" inference get >/dev/null 2>&1; then
  # Clean slate
  "$HARNESS" delete --sandboxes --providers >/dev/null 2>&1 || true

  # Provider registration tests: verify each provider registers with the gateway.
  # Single-provider configs test registration and openshell cross-validation.
  # Functionality tests (curl, MCP, inference) use the all-providers config
  # which has the full sandbox policy.

  # GitHub (from-existing credential style)
  if [[ -n "${GITHUB_TOKEN:-}" ]]; then
    run_test "provider: github register + apply" \
      "$HARNESS" apply -f "$CONFIGS/agent-github-only.yaml" --name test-gh

    run_test "provider: github in openshell" \
      bash -c '"$1" provider list 2>/dev/null | grep -q github' _ "$CLI"

    "$HARNESS" delete test-gh >/dev/null 2>&1 || true
  else
    skip_test "provider: github register + apply" "GITHUB_TOKEN not set"
    skip_test "provider: github in openshell" "GITHUB_TOKEN not set"
  fi

  # Atlassian (basic auth credential style)
  if [[ -n "${JIRA_API_TOKEN:-}" ]]; then
    run_test "provider: atlassian register + apply" \
      "$HARNESS" apply -f "$CONFIGS/agent-atlassian.yaml" --name test-atl

    run_test "provider: atlassian in openshell" \
      bash -c '"$1" provider list 2>/dev/null | grep -q atlassian' _ "$CLI"

    "$HARNESS" delete test-atl >/dev/null 2>&1 || true
  else
    skip_test "provider: atlassian register + apply" "JIRA_API_TOKEN not set"
    skip_test "provider: atlassian in openshell" "JIRA_API_TOKEN not set"
  fi

  # Vertex AI (ADC credential style)
  if [[ -n "${ANTHROPIC_VERTEX_PROJECT_ID:-}" ]]; then
    run_test "provider: vertex register + apply" \
      "$HARNESS" apply -f "$CONFIGS/agent-vertex.yaml" --name test-vtx

    run_test "provider: vertex in openshell" \
      bash -c '"$1" provider list 2>/dev/null | grep -q vertex' _ "$CLI"

    run_test "provider: inference route set" \
      bash -c '"$1" inference get 2>/dev/null' _ "$CLI"

    "$HARNESS" delete test-vtx >/dev/null 2>&1 || true
  else
    skip_test "provider: vertex register + apply" "ANTHROPIC_VERTEX_PROJECT_ID not set"
    skip_test "provider: vertex in openshell" "ANTHROPIC_VERTEX_PROJECT_ID not set"
    skip_test "provider: inference route set" "ANTHROPIC_VERTEX_PROJECT_ID not set"
  fi

  # GWS (OAuth refresh credential style)
  if command -v gws >/dev/null 2>&1 && gws auth export --unmasked >/dev/null 2>&1; then
    run_test "provider: gws register + apply" \
      "$HARNESS" apply -f "$CONFIGS/agent-gws.yaml" --name test-gws

    run_test "provider: gws in openshell" \
      bash -c '"$1" provider list 2>/dev/null | grep -q gws' _ "$CLI"

    "$HARNESS" delete test-gws >/dev/null 2>&1 || true
  else
    skip_test "provider: gws register + apply" "gws not authenticated"
    skip_test "provider: gws in openshell" "gws not authenticated"
  fi

  # All providers: composition + functionality validation
  # Uses the full default agent config which has all provider endpoints
  # in the sandbox policy. This is where we test that APIs actually work.
  if [[ -n "${GITHUB_TOKEN:-}" ]] && [[ -n "${JIRA_API_TOKEN:-}" ]] && [[ -n "${ANTHROPIC_VERTEX_PROJECT_ID:-}" ]]; then
    "$HARNESS" delete --sandboxes --providers >/dev/null 2>&1 || true

    run_test "provider: all providers live apply" \
      "$HARNESS" apply -f "$CONFIGS/agent-all-providers.yaml" --name test-all

    run_test "provider: all providers cross-validate" \
      bash -c 'providers=$("$1" provider list 2>/dev/null) && \
        echo "$providers" | grep -q github && \
        echo "$providers" | grep -q vertex && \
        echo "$providers" | grep -q atlassian' _ "$CLI"

    run_test "provider: harness get matches openshell" \
      bash -c 'h=$("$1" get providers -o json 2>/dev/null) && \
        echo "$h" | python3 -c "import sys,json; d=json.load(sys.stdin); assert len(d) >= 3, f\"expected >= 3 providers, got {len(d)}\""' _ "$HARNESS"

    # Functionality: test real API calls from inside the all-providers sandbox
    run_test "func: github api from sandbox" \
      "$CLI" sandbox exec --name test-all -- \
        bash -c 'curl -sf -H "Authorization: token $GITHUB_TOKEN" https://api.github.com/user -o /dev/null'

    run_test "func: jira api from sandbox" \
      "$CLI" sandbox exec --name test-all -- \
        bash -c 'curl -sf -u "$JIRA_USERNAME:$JIRA_API_TOKEN" "$JIRA_URL/rest/api/2/myself" -o /dev/null'

    run_test "func: inference route from sandbox" \
      "$CLI" sandbox exec --name test-all -- \
        bash -c 'curl -sf https://inference.local/v1/models 2>/dev/null | head -1 | grep -q .'

    if command -v gws >/dev/null 2>&1 && gws auth export --unmasked >/dev/null 2>&1; then
      run_test "func: gmail api from sandbox" \
        "$CLI" sandbox exec --name test-all -- \
          bash -c 'for i in 1 2 3; do curl -sf https://gmail.googleapis.com/gmail/v1/users/me/profile -H "Authorization: Bearer $GOOGLE_WORKSPACE_CLI_TOKEN" -o /dev/null && exit 0; sleep 2; done; exit 1'
    else
      skip_test "func: gmail api from sandbox" "gws not authenticated"
    fi

    "$HARNESS" delete test-all >/dev/null 2>&1 || true
    "$HARNESS" delete --providers >/dev/null 2>&1 || true
  else
    skip_test "provider: all providers live apply" "missing credentials"
    skip_test "provider: all providers cross-validate" "missing credentials"
    skip_test "provider: harness get matches openshell" "missing credentials"
    skip_test "func: github api from sandbox" "missing credentials"
    skip_test "func: jira api from sandbox" "missing credentials"
    skip_test "func: inference route from sandbox" "missing credentials"
    skip_test "func: gmail api from sandbox" "missing credentials"
  fi
else
  if ! $LIVE; then
    echo "  (skipped: use --live)"
  else
    echo "  (skipped: no gateway)"
  fi
fi

echo ""

# ── Free API provider tests (requires keys) ──────────────────────

echo "=== Free API providers (via provider profiles) ==="

# These tests use proper provider profiles (test/configs/providers/*.yaml)
# that define endpoint policy and credential handling. The gateway manages
# API keys via the proxy -- sandboxes never see real keys.
#
# Provider profiles are imported from test/configs/providers/ before
# registration, so they need to be imported to the gateway first.
PROVIDER_DIR="$SCRIPT_DIR/test/configs/providers"

if [[ -n "${GROQ_API_KEY:-}" ]]; then
  run_test "free-api: groq dry-run" \
    "$HARNESS" apply --dry-run -f "$CONFIGS/agent-groq.yaml"

  if $LIVE && "$CLI" inference get >/dev/null 2>&1; then
    # Import the Groq provider profile and register
    "$CLI" provider profile import "$PROVIDER_DIR" >/dev/null 2>&1 || true

    run_test "free-api: groq register + apply" \
      "$HARNESS" apply -f "$CONFIGS/agent-groq.yaml" --name test-groq-live

    # The provider profile defines api.groq.com as an allowed endpoint.
    # The proxy resolves GROQ_API_KEY as a bearer token.
    run_test "free-api: groq completion via proxy" \
      "$CLI" sandbox exec --name test-groq-live -- \
        bash -c 'curl -sf https://api.groq.com/openai/v1/chat/completions \
          -H "Authorization: Bearer $GROQ_API_KEY" \
          -H "Content-Type: application/json" \
          -d "{\"model\":\"llama-3.3-70b-versatile\",\"messages\":[{\"role\":\"user\",\"content\":\"Reply with only the word yes\"}],\"max_tokens\":5}" \
          | grep -q "\"content\""'

    "$HARNESS" delete test-groq-live >/dev/null 2>&1 || true
  fi
else
  skip_test "free-api: groq dry-run" "GROQ_API_KEY not set"
  $LIVE && skip_test "free-api: groq register + apply" "GROQ_API_KEY not set"
  $LIVE && skip_test "free-api: groq completion via proxy" "GROQ_API_KEY not set"
fi

if [[ -n "${OPENROUTER_API_KEY:-}" ]]; then
  run_test "free-api: openrouter dry-run" \
    "$HARNESS" apply --dry-run -f "$CONFIGS/agent-openrouter.yaml"

  if $LIVE && "$CLI" inference get >/dev/null 2>&1; then
    "$CLI" provider profile import "$PROVIDER_DIR" >/dev/null 2>&1 || true

    run_test "free-api: openrouter register + apply" \
      "$HARNESS" apply -f "$CONFIGS/agent-openrouter.yaml" --name test-or-live

    run_test "free-api: openrouter completion via proxy" \
      "$CLI" sandbox exec --name test-or-live -- \
        bash -c 'curl -sf https://openrouter.ai/api/v1/chat/completions \
          -H "Authorization: Bearer $OPENROUTER_API_KEY" \
          -H "Content-Type: application/json" \
          -d "{\"model\":\"meta-llama/llama-3.3-70b-instruct:free\",\"messages\":[{\"role\":\"user\",\"content\":\"Reply with only the word yes\"}],\"max_tokens\":5}" \
          | grep -q "\"content\""'

    "$HARNESS" delete test-or-live >/dev/null 2>&1 || true
  fi
else
  skip_test "free-api: openrouter dry-run" "OPENROUTER_API_KEY not set"
  $LIVE && skip_test "free-api: openrouter register + apply" "OPENROUTER_API_KEY not set"
  $LIVE && skip_test "free-api: openrouter completion via proxy" "OPENROUTER_API_KEY not set"
fi

if [[ -n "${NVIDIA_API_KEY:-}" ]]; then
  run_test "free-api: nvidia nim dry-run" \
    "$HARNESS" apply --dry-run -f "$CONFIGS/agent-nvidia-nim.yaml"

  if $LIVE && "$CLI" inference get >/dev/null 2>&1; then
    "$CLI" provider profile import "$PROVIDER_DIR" >/dev/null 2>&1 || true

    run_test "free-api: nvidia nim register + apply" \
      "$HARNESS" apply -f "$CONFIGS/agent-nvidia-nim.yaml" --name test-nim-live

    run_test "free-api: nvidia nim completion via proxy" \
      "$CLI" sandbox exec --name test-nim-live -- \
        bash -c 'curl -sf https://integrate.api.nvidia.com/v1/chat/completions \
          -H "Authorization: Bearer $NVIDIA_API_KEY" \
          -H "Content-Type: application/json" \
          -d "{\"model\":\"meta/llama-3.3-70b-instruct\",\"messages\":[{\"role\":\"user\",\"content\":\"Reply with only the word yes\"}],\"max_tokens\":5}" \
          | grep -q "\"content\""'

    "$HARNESS" delete test-nim-live >/dev/null 2>&1 || true
  fi
else
  skip_test "free-api: nvidia nim dry-run" "NVIDIA_API_KEY not set"
  $LIVE && skip_test "free-api: nvidia nim register + apply" "NVIDIA_API_KEY not set"
  $LIVE && skip_test "free-api: nvidia nim completion via proxy" "NVIDIA_API_KEY not set"
fi

echo ""

# ── Agent integration tests (claude/opencode using real providers) ─

echo "=== Agent integration ==="

if $LIVE && "$CLI" inference get >/dev/null 2>&1; then

  # Vertex AI through OpenCode: can opencode reach inference.local?
  if [[ -n "${ANTHROPIC_VERTEX_PROJECT_ID:-}" ]]; then
    run_test "agent: opencode inference via vertex" \
      bash -c '"$1" apply -f "$2" --name test-oc-vtx 2>/dev/null && \
        "$3" sandbox exec --name test-oc-vtx -- \
          bash -c "echo \"respond with ok\" | opencode --print 2>&1 | head -5 | grep -qi ok" && \
        "$1" delete test-oc-vtx >/dev/null 2>&1' _ "$HARNESS" "$CONFIGS/agent-vertex.yaml" "$CLI"
  else
    skip_test "agent: opencode inference via vertex" "ANTHROPIC_VERTEX_PROJECT_ID not set"
  fi

  # Claude Code through Vertex AI: can claude reach inference.local?
  if [[ -n "${ANTHROPIC_VERTEX_PROJECT_ID:-}" ]]; then
    run_test "agent: claude inference via vertex" \
      bash -c '"$1" apply --name test-cc-vtx 2>/dev/null && \
        "$3" sandbox exec --name test-cc-vtx -- \
          bash -c "echo \"respond with ok\" | claude --print 2>&1 | head -5 | grep -qi ok" && \
        "$1" delete test-cc-vtx >/dev/null 2>&1' _ "$HARNESS" "$CLI"
  else
    skip_test "agent: claude inference via vertex" "ANTHROPIC_VERTEX_PROJECT_ID not set"
  fi

  # Atlassian MCP through Claude: can claude use jira via mcp-atlassian?
  if [[ -n "${JIRA_API_TOKEN:-}" ]] && [[ -n "${JIRA_URL:-}" ]]; then
    run_test "agent: claude atlassian mcp" \
      bash -c '"$1" apply -f "$2" --name test-cc-atl 2>/dev/null && \
        "$3" sandbox exec --name test-cc-atl -- \
          bash -c "echo \"use the jira mcp tool to get my user profile, respond with my email\" | claude --print 2>&1 | grep -qi @" && \
        "$1" delete test-cc-atl >/dev/null 2>&1' _ "$HARNESS" "$CONFIGS/agent-atlassian.yaml" "$CLI"
  else
    skip_test "agent: claude atlassian mcp" "JIRA credentials not set"
  fi

  # GWS CLI through Claude: can claude use gws to read calendar?
  if command -v gws >/dev/null 2>&1 && gws auth export --unmasked >/dev/null 2>&1; then
    run_test "agent: claude gws calendar" \
      bash -c '"$1" apply -f "$2" --name test-cc-gws 2>/dev/null && \
        "$3" sandbox exec --name test-cc-gws -- \
          bash -c "echo \"use gws to list my next calendar event, respond with the title\" | claude --print 2>&1 | head -10 | grep -q ." && \
        "$1" delete test-cc-gws >/dev/null 2>&1' _ "$HARNESS" "$CONFIGS/agent-gws.yaml" "$CLI"
  else
    skip_test "agent: claude gws calendar" "gws not authenticated"
  fi

  # Free API: Groq through Claude (tests API key inference provider)
  if [[ -n "${GROQ_API_KEY:-}" ]]; then
    run_test "agent: groq inference from sandbox" \
      bash -c '"$1" apply -f "$2" --name test-cc-groq 2>/dev/null && \
        "$3" sandbox exec --name test-cc-groq -- \
          bash -c "curl -sf https://api.groq.com/openai/v1/chat/completions \
            -H \"Authorization: Bearer \$GROQ_API_KEY\" \
            -H \"Content-Type: application/json\" \
            -d \"{\\\"model\\\":\\\"llama-3.3-70b-versatile\\\",\\\"messages\\\":[{\\\"role\\\":\\\"user\\\",\\\"content\\\":\\\"say ok\\\"}],\\\"max_tokens\\\":5}\" \
            | grep -q content" && \
        "$1" delete test-cc-groq >/dev/null 2>&1' _ "$HARNESS" "$CONFIGS/agent-groq.yaml" "$CLI"
  else
    skip_test "agent: groq inference from sandbox" "GROQ_API_KEY not set"
  fi

else
  if ! $LIVE; then
    echo "  (skipped: use --live)"
  else
    echo "  (skipped: no gateway)"
  fi
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
