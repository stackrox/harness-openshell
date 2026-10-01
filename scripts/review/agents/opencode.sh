#!/usr/bin/env bash
# OpenCode PR-review profile, sourced by scripts/pr-review.sh.
# This legacy profile owns the temporary Vertex workspace and provider setup.
# shellcheck disable=SC2034,SC2154 # configuration and lifecycle state are shared with the wrapper.

agent_configure() {
  workspace="${OPENSHELL_WORKSPACE:-rev-$RANDOM-$$}"
  sandbox_name="review-$(openssl rand -hex 6)"
  configured_target=false
  github_provider="github-review"
  if [[ -n "${OPENSHELL_WORKSPACE:-}" ]]; then
    configured_target=true
  else
    github_provider="github-review-$RANDOM-$$"
  fi
}

agent_require_credentials() {
  export VERTEX_AI_REGION="${VERTEX_AI_REGION:-global}"
  if [[ "$configured_target" != true ]]; then
    : "${GITHUB_TOKEN:?set the workflow GitHub token for provider bootstrap}"
    : "${GOOGLE_VERTEX_AI_TOKEN:?set a short-lived Vertex token}" \
      "${VERTEX_AI_PROJECT_ID:?set Vertex project}"
  fi
  if [[ -n "${VERTEX_AI_PROJECT_ID:-}" ]]; then
    local vertex_host="aiplatform.googleapis.com"
    [[ "$VERTEX_AI_REGION" == global ]] || vertex_host="${VERTEX_AI_REGION}-aiplatform.googleapis.com"
    export VERTEX_AI_BASE_URL="https://${vertex_host}/v1/projects/${VERTEX_AI_PROJECT_ID}/locations/${VERTEX_AI_REGION}/endpoints/openapi"
  else
    export VERTEX_AI_BASE_URL="${VERTEX_AI_BASE_URL:?set VERTEX_AI_BASE_URL for the pre-provisioned Vertex provider}"
  fi
}

agent_setup() {
  [[ "$configured_target" == true ]] && return 0

  timeout 60s openshell workspace create --gateway "$gateway" --name "$workspace"
  created_workspace=true
  timeout 60s openshell provider create --gateway "$gateway" --workspace "$workspace" \
    --name vertex-review --type google-vertex-ai --from-existing \
    --config "VERTEX_AI_PROJECT_ID=$VERTEX_AI_PROJECT_ID" \
    --config "VERTEX_AI_REGION=${VERTEX_AI_REGION:-global}"
  created_vertex_provider=true
  timeout 60s openshell provider create --gateway "$gateway" --workspace "$workspace" \
    --name "$github_provider" --type github --credential GITHUB_TOKEN
  created_github_provider=true
}

agent_workflow_file() {
  printf '%s\n' tasks/github-pr-reviewer/workflow/opencode-harness.yaml
}

agent_validator() {
  printf '%s\n' scripts/review/validate-agent-output.sh
}

agent_extract_output() {
  jq -Rr 'fromjson? | select(.type == "text") | .part.text' \
    "$REVIEW_DIR/agent.ndjson" > "$REVIEW_DIR/review.txt"
}
