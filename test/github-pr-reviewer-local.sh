#!/usr/bin/env bash
# Run the deterministic PR reviewer fixture against a local OpenShell gateway.
#
# This is intentionally local-only: it consumes the preconfigured inference
# route and grants the sandbox no GitHub write capability.
set -uo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
HARNESS="$ROOT/harness"
WORKFLOW="$ROOT/examples/github-pr-reviewer/harness.yaml"
EXPECTED="PR_REVIEW_OK sha=fixture-pr-head-20260908"

if [[ "${CI:-}" == "true" ]]; then
  echo "SKIP: PR reviewer fixture requires a locally reachable inference gateway."
  exit 0
fi
[[ -x "$HARNESS" ]] || { echo "ERROR: run make cli first" >&2; exit 1; }

name="pr-$(date +%s)-$$"
output=""
cleanup() {
  "$HARNESS" delete "$name" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM

output=$("$HARNESS" apply --file "$WORKFLOW" --name "$name" 2>&1)
rc=$?
printf '%s\n' "$output"

if ((rc != 0)); then
  echo "RESULT: FAIL (harness apply exited $rc)" >&2
  exit 1
fi
last_line="$(printf '%s\n' "$output" | awk 'NF { line = $0 } END { print line }')"
if [[ "$last_line" != "$EXPECTED" ]]; then
  echo "RESULT: FAIL (review output did not contain the required marker)" >&2
  exit 1
fi
echo "RESULT: PASS ($EXPECTED; sandbox cleanup requested)"
