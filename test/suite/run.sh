#!/usr/bin/env bash
# Canonical configuration and CLI contract suite.
set -uo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
HARNESS="$ROOT/harness"
CLI="${OPENSHELL_CLI:-openshell}"
CONFIG="$ROOT/test/configs/harness-v1alpha1.yaml"
LIFECYCLE="$ROOT/test/lifecycle-workflow.yaml"
LIVE=false
FILTER=""
VERBOSE=false

for arg in "$@"; do
  case "$arg" in
    --live) LIVE=true ;;
    --filter=*) FILTER="${arg#--filter=}" ;;
    --verbose) VERBOSE=true ;;
    *) echo "Unknown: $arg"; exit 1 ;;
  esac
done

if [[ ! -x "$HARNESS" ]]; then
  echo "ERROR: Binary not found at $HARNESS — run: make cli"
  exit 1
fi

PASS=0
FAIL=0
SKIP=0
START=$(date +%s)

run_test() {
  local name="$1"; shift
  [[ -n "$FILTER" && "$name" != *"$FILTER"* ]] && return
  local output
  output=$(mktemp)
  if "$@" >"$output" 2>&1; then
    printf "  ✓ %-52s\n" "$name"
    ((PASS++))
  else
    printf "  ✗ %-52s\n" "$name"
    $VERBOSE && cat "$output"
    ((FAIL++))
  fi
  rm -f "$output"
}

run_test_fail() {
  local name="$1"; shift
  [[ -n "$FILTER" && "$name" != *"$FILTER"* ]] && return
  if ! "$@" >/dev/null 2>&1; then
    printf "  ✓ %-52s (expected fail)\n" "$name"
    ((PASS++))
  else
    printf "  ✗ %-52s (should have failed)\n" "$name"
    ((FAIL++))
  fi
}

echo "=== Canonical configuration ==="
run_test "apply: resolved YAML" bash -c '"$1" apply -f "$2" -o yaml | grep -q "apiVersion: harness.openshell.dev/v1alpha1"' _ "$HARNESS" "$CONFIG"
run_test "apply: resolved JSON" bash -c '"$1" apply -f "$2" -o json | python3 -m json.tool >/dev/null' _ "$HARNESS" "$CONFIG"
run_test "apply: name override" bash -c '"$1" apply -f "$2" --name overridden -o yaml | grep -q "name: overridden"' _ "$HARNESS" "$CONFIG"
run_test "apply: entrypoint override" bash -c '"$1" apply -f "$2" --entrypoint opencode -o yaml | grep -q "type: opencode"' _ "$HARNESS" "$CONFIG"
run_test "apply: attach override" bash -c '"$1" apply -f "$2" --attach -o yaml | grep -q "tty: true"' _ "$HARNESS" "$CONFIG"
run_test_fail "apply: file is required" "$HARNESS" apply -o yaml
run_test_fail "apply: unversioned config rejected" bash -c 'f=$(mktemp); printf "name: old\\nentrypoint: claude\\n" >"$f"; "$1" apply -f "$f" -o yaml; rc=$?; rm -f "$f"; exit $rc' _ "$HARNESS"

echo "=== Plan and resource commands ==="
run_test "plan: table has all sections" bash -c 'out=$("$1" plan -f "$2"); for section in TARGET PROVIDERS INFERENCE RUN; do grep -q "$section" <<<"$out" || exit 1; done' _ "$HARNESS" "$CONFIG"
run_test "plan: JSON" bash -c '"$1" plan -f "$2" -o json | python3 -m json.tool >/dev/null' _ "$HARNESS" "$CONFIG"
run_test "plan: YAML" bash -c '"$1" plan -f "$2" -o yaml | grep -q "section: providers"' _ "$HARNESS" "$CONFIG"
run_test_fail "delete: arguments required" "$HARNESS" delete

echo "=== Init and doctor ==="
run_test "init: canonical scaffold" bash -c 'd=$(mktemp -d); "$1" init --non-interactive -o "$d/harness.yaml" >/dev/null && grep -q "apiVersion: harness.openshell.dev/v1alpha1" "$d/harness.yaml"; rc=$?; rm -rf "$d"; exit $rc' _ "$HARNESS"
run_test "doctor: canonical file accepted" "$HARNESS" doctor -f "$ROOT/test/ci-workflow.yaml" -o json

if $LIVE && "$CLI" inference get >/dev/null 2>&1; then
  echo "=== Live SDK lifecycle ==="
  run_test "live: create and retain" "$HARNESS" apply -f "$LIFECYCLE" --name suite-sdk-live
  run_test "live: describe" "$HARNESS" describe suite-sdk-live
  run_test "live: get agents" bash -c '"$1" get agents | grep -q suite-sdk-live' _ "$HARNESS"
  run_test "live: exec" "$CLI" sandbox exec --name suite-sdk-live -- echo alive
  run_test "live: delete" "$HARNESS" delete suite-sdk-live
else
  echo "=== Live SDK lifecycle (skipped: use --live with a gateway) ==="
  ((SKIP++))
fi

TOTAL=$((PASS + FAIL))
ELAPSED=$(( $(date +%s) - START ))
echo
if [[ $FAIL -eq 0 ]]; then
  echo "$PASS/$TOTAL passed, $SKIP skipped (${ELAPSED}s)"
else
  echo "$PASS/$TOTAL passed, $FAIL failed, $SKIP skipped (${ELAPSED}s)"
  exit 1
fi
