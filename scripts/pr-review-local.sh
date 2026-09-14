#!/usr/bin/env bash
# Temporary local CI resources around the existing PR review script.
set -euo pipefail
umask 077
cd "$(dirname "$0")/.."
: "${GOOGLE_VERTEX_AI_TOKEN:?set a short-lived Vertex token}" "${VERTEX_AI_PROJECT_ID:?set Vertex project}"
: "${GITHUB_TOKEN:?set the repository-scoped GitHub App token for bootstrap}"
gateway="${OPENSHELL_GATEWAY:-openshell}"
workspace="review-$(openssl rand -hex 12)"
created_workspace=false
created_vertex=false
created_github=false
imported_profile=false
review_pid=""
cleanup() {
  local status=$?
  trap - EXIT INT TERM
  if [[ -n "$review_pid" ]]; then
    kill -TERM "$review_pid" 2>/dev/null || true
    wait "$review_pid" || true
  fi
  # Wait for the runner's sandbox cleanup before removing only our resources.
  if $created_workspace; then
    if $created_vertex; then
      timeout 30s openshell provider delete --gateway "$gateway" --workspace "$workspace" vertex-review || status=1
    fi
    if $created_github; then
      timeout 30s openshell provider delete --gateway "$gateway" --workspace "$workspace" github-review || status=1
    fi
    if $imported_profile; then
      timeout 30s openshell provider profile delete --gateway "$gateway" --workspace "$workspace" github-review || status=1
    fi
    timeout 30s openshell workspace delete --gateway "$gateway" "$workspace" || status=1
  fi
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

timeout 60s openshell workspace create --gateway "$gateway" --name "$workspace"
created_workspace=true
profiles="$(timeout 60s openshell provider list-profiles --gateway "$gateway" --workspace "$workspace" -o json)"
jq -e 'type == "array" and all(.[]; (.id | type) == "string")' <<< "$profiles" >/dev/null
if ! jq -e 'any(.[]; .id == "github-review")' <<< "$profiles" >/dev/null; then
  timeout 60s openshell provider profile import --gateway "$gateway" --workspace "$workspace" \
    --file tasks/github-pr-reviewer/openshell/providers/github-review.yaml
  imported_profile=true
fi
timeout 60s openshell provider create --gateway "$gateway" --workspace "$workspace" \
  --name vertex-review --type google-vertex-ai --from-existing \
  --config "VERTEX_AI_PROJECT_ID=$VERTEX_AI_PROJECT_ID" --config "VERTEX_AI_REGION=${VERTEX_AI_REGION:-global}"
created_vertex=true
timeout 60s openshell provider create --gateway "$gateway" --workspace "$workspace" \
  --name github-review --type github-review --credential GITHUB_TOKEN
created_github=true
timeout 60s openshell inference set --gateway "$gateway" --workspace "$workspace" \
  --provider vertex-review --model gemini-2.5-pro --no-verify
OPENSHELL_GATEWAY="$gateway" OPENSHELL_WORKSPACE="$workspace" bash scripts/pr-review.sh run &
review_pid=$!
set +e
wait "$review_pid"
status=$?
set -e
review_pid=""
exit "$status"
