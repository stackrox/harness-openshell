#!/usr/bin/env bash
# Codex PR-review profile, sourced by scripts/pr-review.sh.
# Keep provider and workspace ownership explicit: Codex uses a pre-provisioned
# OpenShell workspace by default. CI may opt into an ephemeral local gateway
# bootstrap, which keeps the credentials in the gateway and out of the sandbox.
# shellcheck disable=SC2034,SC2154 # configuration and lifecycle state are shared with the wrapper.

agent_configure() {
  [[ "$codex_inference_provider" =~ ^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$ && "$codex_model" =~ ^[A-Za-z0-9_.@/-]+$ ]] || {
    echo "invalid Codex inference provider or model" >&2
    return 1
  }
  [[ "$codex_workspace" =~ ^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$ ]] || {
    echo "Codex requires a valid pre-provisioned workspace" >&2
    return 1
  }

  if [[ "${CODEX_BOOTSTRAP:-false}" == true ]]; then
    workspace="codex-$(openssl rand -hex 6)"
    configured_target=false
  else
    workspace="$codex_workspace"
    configured_target=true
  fi
  sandbox_name="codex-$(openssl rand -hex 6)"
  github_provider=github-review
}

agent_require_credentials() {
  if [[ "$configured_target" != true ]]; then
    : "${GITHUB_TOKEN:?set the workflow GitHub token for provider bootstrap}"
    : "${OPENSHELL_CODEX_API_KEY:?set the OpenAI API key for the local Codex provider}"
  fi
}

agent_setup() {
  [[ "$configured_target" == true ]] && return 0

  timeout 60s openshell workspace create --gateway "$gateway" --name "$workspace"
  created_workspace=true

  profiles="$(timeout 60s openshell provider list-profiles --gateway "$gateway" --workspace "$workspace" -o json)"
  jq -e 'type == "array" and all(.[]; (.id | type) == "string")' <<< "$profiles" >/dev/null
  if ! jq -e 'any(.[]; .id == "github-review")' <<< "$profiles" >/dev/null; then
    timeout 60s openshell provider profile import --gateway "$gateway" --workspace "$workspace" \
      --file tasks/github-pr-reviewer/openshell/providers/github-review.yaml
    created_github_profile=true
  fi
  timeout 60s openshell provider create --gateway "$gateway" --workspace "$workspace" \
    --name github-review --type github-review --credential GITHUB_TOKEN
  created_github_provider=true
  timeout 60s openshell provider create --gateway "$gateway" --workspace "$workspace" \
    --name "$codex_inference_provider" --type openai --credential OPENSHELL_CODEX_API_KEY
  created_codex_provider=true
  timeout 60s openshell inference set --gateway "$gateway" --workspace "$workspace" \
    --provider "$codex_inference_provider" --model "$codex_model" --no-verify
}

agent_workflow_file() {
  printf '%s\n' tasks/github-pr-reviewer/workflow/codex-harness.yaml
}

agent_validator() {
  printf '%s\n' scripts/review/validate-codex-output.sh
}

agent_extract_output() {
  if [[ -s "$REVIEW_DIR/codex-final.txt" ]]; then
    cp "$REVIEW_DIR/codex-final.txt" "$REVIEW_DIR/review.txt"
  else
    jq -Rr 'fromjson? | select(.type == "item.completed" and .item.type == "agent_message") | .item.text' \
      "$REVIEW_DIR/agent.ndjson" > "$REVIEW_DIR/review.txt"
  fi
}
