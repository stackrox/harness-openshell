#!/usr/bin/env bash
# Codex PR-review profile, sourced by scripts/pr-review.sh.
# Keep provider and workspace ownership explicit: Codex uses a pre-provisioned
# OpenShell workspace and never creates or deletes its providers.
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

  workspace="$codex_workspace"
  sandbox_name="codex-$(openssl rand -hex 6)"
  configured_target=true
  github_provider=github-review
}

agent_require_credentials() {
  :
}

agent_setup() {
  :
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
