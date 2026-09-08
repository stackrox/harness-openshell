#!/usr/bin/env bash
# Exercise Gemini 3.8 Flash through OpenCode on a local OpenShell gateway.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
HARNESS="$ROOT/harness"
CLI="${OPENSHELL_CLI:-openshell}"
GATEWAY="${OPENSHELL_GATEWAY:-openshell}"
PROJECT="${VERTEX_AI_PROJECT_ID:?set VERTEX_AI_PROJECT_ID}"
REGION="${VERTEX_AI_REGION:-global}"
TOKEN="${GOOGLE_VERTEX_AI_TOKEN:?set GOOGLE_VERTEX_AI_TOKEN}"
WORKSPACE="vtx-$RANDOM-$$"
PROVIDER="vertex-ci"
WORKFLOW="$ROOT/test/vertex-gemini-opencode-workflow.yaml"
created_workspace=false
created_provider=false
apply_pid=""
output_file=""

cleanup() {
  local status=$?
  trap - EXIT INT TERM
  if [[ -n "$apply_pid" ]]; then
    kill -TERM "$apply_pid" 2>/dev/null || true
    wait "$apply_pid" 2>/dev/null || true
  fi
  if [[ "$created_workspace" == true ]]; then
    "$HARNESS" delete --gateway "$GATEWAY" --workspace "$WORKSPACE" --sandboxes || status=1
    if [[ "$created_provider" == true ]]; then
      "$CLI" provider delete --gateway "$GATEWAY" --workspace "$WORKSPACE" "$PROVIDER" || status=1
    fi
    "$CLI" workspace delete --gateway "$GATEWAY" "$WORKSPACE" || status=1
  fi
  if [[ -n "$output_file" ]]; then rm -f "$output_file"; fi
  if ((status == 0)); then
    echo "RESULT: PASS (GEMINI_OPENCODE_OK; cleanup completed)"
  else
    echo "RESULT: FAIL (exit $status; cleanup attempted)" >&2
  fi
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

[[ -x "$HARNESS" ]] || { echo "ERROR: harness binary not found; run make cli" >&2; exit 1; }

"$CLI" workspace create --gateway "$GATEWAY" --name "$WORKSPACE"
created_workspace=true

GOOGLE_VERTEX_AI_TOKEN="$TOKEN" \
  "$CLI" provider create \
    --gateway "$GATEWAY" \
    --workspace "$WORKSPACE" \
    --name "$PROVIDER" \
    --type google-vertex-ai \
    --from-existing \
    --config "VERTEX_AI_PROJECT_ID=$PROJECT" \
    --config "VERTEX_AI_REGION=$REGION"
created_provider=true

"$CLI" inference set \
  --gateway "$GATEWAY" \
  --workspace "$WORKSPACE" \
  --provider "$PROVIDER" \
  --model gemini-3.8-flash

output_file="$(mktemp)"
"$HARNESS" apply -f "$WORKFLOW" --gateway "$GATEWAY" --workspace "$WORKSPACE" >"$output_file" &
apply_pid=$!
status=0
wait "$apply_pid" || status=$?
apply_pid=""
output="$(<"$output_file")"
printf '%s\n' "$output"
((status == 0)) || exit "$status"
last_line="$(awk 'NF { line = $0 } END { print line }' <<<"$output")"
[[ "$last_line" == 'GEMINI_OPENCODE_OK' ]] || { echo "ERROR: invalid smoke response" >&2; exit 1; }
