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
WORKSPACE="vtx-$RANDOM"
PROVIDER="vertex-ci"
WORKFLOW="$ROOT/test/vertex-gemini-opencode-workflow.yaml"
created_workspace=false

cleanup() {
  if [[ "$created_workspace" == true ]]; then
    "$HARNESS" delete --gateway "$GATEWAY" --workspace "$WORKSPACE" --sandboxes >/dev/null 2>&1 || true
    "$CLI" provider delete --gateway "$GATEWAY" --workspace "$WORKSPACE" "$PROVIDER" >/dev/null 2>&1 || true
    "$CLI" workspace delete --gateway "$GATEWAY" "$WORKSPACE" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

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

"$CLI" inference set \
  --gateway "$GATEWAY" \
  --workspace "$WORKSPACE" \
  --provider "$PROVIDER" \
  --model gemini-3.8-flash

output="$("$HARNESS" apply -f "$WORKFLOW" --gateway "$GATEWAY" --workspace "$WORKSPACE")"
printf '%s\n' "$output"
grep -Fxq 'GEMINI_OPENCODE_OK' <<<"$output"
