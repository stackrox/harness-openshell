#!/usr/bin/env bash
# Trusted host orchestration. PR content is data, never runner code.
set -euo pipefail
umask 077
cd "$(dirname "$0")/.."
: "${REVIEW_DIR:?set an absolute artifact directory}"
: "${REVIEW_REPOSITORY:?set owner/repository}" "${REVIEW_PR:?set PR number}"
[[ "$REVIEW_DIR" == /* && "$REVIEW_REPOSITORY" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ && "$REVIEW_PR" =~ ^[1-9][0-9]*$ ]] || exit 1
mode="${1:?usage: pr-review.sh prepare|run}"
[[ "$mode" == prepare || "$mode" == run ]] || exit 1
gateway="${OPENSHELL_GATEWAY:-openshell}"
allow_draft_reviews="${ALLOW_DRAFT_REVIEWS:-false}"
review_agent="${REVIEW_AGENT:-opencode}"
review_label="${REVIEW_LABEL:-stackrox-ai-review}"
codex_inference_provider="${CODEX_INFERENCE_PROVIDER:-openai-review}"
codex_model="${CODEX_MODEL:-gpt-5.6-luna}"
codex_workspace="${CODEX_WORKSPACE:-}"
case "$review_agent" in
  opencode|codex) ;;
  *) echo "unsupported review agent: $review_agent" >&2; exit 1 ;;
esac
[[ "$review_label" =~ ^[A-Za-z0-9][A-Za-z0-9._-]{0,49}$ ]] || {
  echo "invalid review label" >&2
  exit 1
}
if [[ "$review_agent" == codex ]]; then
  [[ "$codex_inference_provider" =~ ^[A-Za-z0-9_.-]+$ && "$codex_model" =~ ^[A-Za-z0-9_.@/-]+$ ]] || {
    echo "invalid Codex inference provider or model" >&2
    exit 1
  }
  [[ "$codex_workspace" =~ ^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$ ]] || {
    echo "Codex requires a valid pre-provisioned workspace" >&2
    exit 1
  }
fi
if [[ "$review_agent" == codex ]]; then
  workspace="$codex_workspace"
  sandbox_name="codex-review-$RANDOM-$$"
else
  workspace="rev-$RANDOM-$$"
  sandbox_name=ai-review
fi
github_provider="github-review-$RANDOM-$$"
max_diff_bytes=262144
created_workspace=false
created_vertex_provider=false
created_github_provider=false
apply_pid=""
head="${REVIEW_HEAD:-}"
base=""
state=failed
endpoint="repos/$REVIEW_REPOSITORY/pulls/$REVIEW_PR"

write_summary() {
  [[ -d "$REVIEW_DIR" ]] || return
  {
    printf '## AI review: %s\n\nPR #%s; head: %s\n\n' "$state" "$REVIEW_PR" "$head"
    printf 'Artifact-only model output; not an approval or a validated finding list.\n'
    if [[ -n "${GITHUB_RUN_ID:-}" ]]; then
      printf '\n[Review artifacts](%s/%s/actions/runs/%s#artifacts)\n' "${GITHUB_SERVER_URL:-https://github.com}" "$REVIEW_REPOSITORY" "$GITHUB_RUN_ID"
    fi
  } > "$REVIEW_DIR/summary.md"
  if [[ -n "${GITHUB_STEP_SUMMARY:-}" && "$state" != prepared ]]; then
    cat "$REVIEW_DIR/summary.md" >> "$GITHUB_STEP_SUMMARY"
  fi
}

cleanup_runtime() {
  trap - EXIT INT TERM
  local cleanup_status=0
  delete_sandbox() {
    local output
    if output=$(timeout 30s openshell sandbox delete --gateway "$gateway" --workspace "$workspace" "$sandbox_name" 2>&1); then
      return 0
    fi
    # A workflow with keep:false lets Harness delete the sandbox before this
    # wrapper's best-effort cleanup runs. That is already the desired state.
    if [[ "$output" == *"sandbox not found"* ]]; then
      return 0
    fi
    printf '%s\n' "$output" >&2
    return 1
  }
  if [[ -n "$apply_pid" ]]; then
    kill -TERM "$apply_pid" 2>/dev/null || true
    wait "$apply_pid" || true
  fi
  if $created_workspace || $created_vertex_provider || $created_github_provider; then
    delete_sandbox || cleanup_status=1
    if $created_vertex_provider; then
      timeout 30s openshell provider delete --gateway "$gateway" --workspace "$workspace" vertex-review || cleanup_status=1
    fi
    if $created_github_provider; then
      timeout 30s openshell provider delete --gateway "$gateway" --workspace "$workspace" "$github_provider" || cleanup_status=1
    fi
    if $created_workspace; then
      timeout 30s openshell workspace delete --gateway "$gateway" "$workspace" || cleanup_status=1
    fi
  fi
  return "$cleanup_status"
}

finish() {
  local status=$?
  cleanup_runtime || status=1
  ((status == 0)) || state="failed (exit $status)"
  write_summary
  printf 'AI review: %s\n' "$state"
  exit "$status"
}
trap finish EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

ensure_current() {
  current="$(timeout 60s gh api "$endpoint")"
  if ! jq -e --arg head "$head" --arg base "$base" --arg allow_drafts "$allow_draft_reviews" --arg review_label "$review_label" '
    .state == "open" and (($allow_drafts == "true") or (.draft | not)) and any(.labels[]?; .name == $review_label)
    and ($head == "" or .head.sha == $head) and ($base == "" or .base.sha == $base)
  ' <<< "$current" >/dev/null; then
    state="skipped or superseded"
    exit 0
  fi
}

prepare_review() {
  mkdir "$REVIEW_DIR" # Refuse existing directories and symlinks.
  ensure_current
  head="$(jq -er '.head.sha | select(test("^[0-9a-f]{40}$"))' <<< "$current")"
  base="$(jq -er '.base.sha | select(test("^[0-9a-f]{40}$"))' <<< "$current")"
  jq -n --arg repository "$REVIEW_REPOSITORY" --argjson pr "$REVIEW_PR" --arg head "$head" --arg base "$base" \
    '{repository:$repository, pr:$pr, head:$head, base:$base}' > "$REVIEW_DIR/input.json"
  # Read at most limit+1 bytes. Oversized or failed downloads never reach inference.
  timeout 60s gh api "repos/$REVIEW_REPOSITORY/compare/$base...$head" -H 'Accept: application/vnd.github.diff' \
    | head -c "$((max_diff_bytes + 1))" > "$REVIEW_DIR/pr.diff"
  [[ -s "$REVIEW_DIR/pr.diff" && $(wc -c < "$REVIEW_DIR/pr.diff") -le "$max_diff_bytes" ]] || exit 1
  (cd "$REVIEW_DIR" && shasum -a 256 pr.diff > pr.diff.sha256)
  [[ -z "${GITHUB_OUTPUT:-}" ]] || printf 'eligible=true\n' >> "$GITHUB_OUTPUT"
  state=prepared
}

run_review() {
  head="$(jq -er '.head | select(test("^[0-9a-f]{40}$"))' "$REVIEW_DIR/input.json")"
  base="$(jq -er '.base | select(test("^[0-9a-f]{40}$"))' "$REVIEW_DIR/input.json")"
  (cd "$REVIEW_DIR" && shasum -a 256 -c pr.diff.sha256 >/dev/null)
  ensure_current
  : "${GITHUB_TOKEN:?set the workflow GitHub token for provider bootstrap}"
  if [[ "$review_agent" == opencode ]]; then
    : "${GOOGLE_VERTEX_AI_TOKEN:?set a short-lived Vertex token}" "${VERTEX_AI_PROJECT_ID:?set Vertex project}"
  fi

  if [[ "$review_agent" == opencode ]]; then
    timeout 60s openshell workspace create --gateway "$gateway" --name "$workspace"
    created_workspace=true
  fi
  if [[ "$review_agent" == opencode ]]; then
    timeout 60s openshell provider create --gateway "$gateway" --workspace "$workspace" \
      --name vertex-review --type google-vertex-ai --from-existing \
      --config "VERTEX_AI_PROJECT_ID=$VERTEX_AI_PROJECT_ID" --config "VERTEX_AI_REGION=${VERTEX_AI_REGION:-global}"
    created_vertex_provider=true
  fi
  timeout 60s openshell provider create --gateway "$gateway" --workspace "$workspace" \
    --name "$github_provider" --type github --credential GITHUB_TOKEN
  created_github_provider=true
  if [[ "$review_agent" == opencode ]]; then
    timeout 60s openshell inference set --gateway "$gateway" --workspace "$workspace" \
      --provider vertex-review --model gemini-2.5-pro --no-verify
    workflow_file=workloads/github-pr-reviewer/workflow/opencode-harness.yaml
    validator=scripts/review/validate-agent-output.sh
  else
    workflow_file=workloads/github-pr-reviewer/workflow/codex-harness.yaml
    validator=scripts/review/validate-codex-output.sh
  fi

  export REVIEW_DIFF="$REVIEW_DIR/pr.diff"
  export REVIEW_POLICY="$REVIEW_DIR/review-policy.yaml"
  export REVIEW_SKILL="${REVIEW_SKILL:-skills/pr-review/SKILL.md}"
  export REVIEW_GITHUB_PROVIDER="$github_provider"
  export REVIEW_SANDBOX_NAME="$sandbox_name"
  policy_template="${REVIEW_POLICY_TEMPLATE:-workloads/github-pr-reviewer/openshell/policy.yaml}"
  sed \
    -e "s|\${REVIEW_REPOSITORY}|$REVIEW_REPOSITORY|g" \
    -e "s|\${REVIEW_PR}|$REVIEW_PR|g" \
    -e "s|\${REVIEW_GITHUB_PROVIDER}|$REVIEW_GITHUB_PROVIDER|g" \
    "$policy_template" > "$REVIEW_POLICY"

  (
    ulimit -f 2048 # Bound raw diagnostic output as well as runtime.
    exec timeout -s TERM -k 35s 8m ./harness workflow apply "$workflow_file" \
      --gateway "$gateway" --workspace "$workspace" --output-dir "$REVIEW_DIR" \
      --result-file "$REVIEW_DIR/execution.json"
  ) > "$REVIEW_DIR/agent.ndjson" 2> "$REVIEW_DIR/agent.stderr" &
  apply_pid=$!
  set +e
  wait "$apply_pid"
  apply_status=$?
  set -e
  apply_pid=""

  "$validator" "$REVIEW_DIR"
  if [[ -s "$REVIEW_DIR/execution.json" ]] && ! jq -e '.status == "succeeded" and .phase == "complete"' "$REVIEW_DIR/execution.json" >/dev/null; then
    ((apply_status != 0)) && return "$apply_status"
    return 1
  fi
  ensure_current
  if [[ "$review_agent" == opencode ]]; then
    jq -Rr 'fromjson? | select(.type == "text") | .part.text' \
      "$REVIEW_DIR/agent.ndjson" > "$REVIEW_DIR/review.txt"
  elif [[ -s "$REVIEW_DIR/codex-final.txt" ]]; then
    cp "$REVIEW_DIR/codex-final.txt" "$REVIEW_DIR/review.txt"
  else
    jq -Rr 'fromjson? | select(.type == "item.completed" and .item.type == "agent_message") | .item.text' \
      "$REVIEW_DIR/agent.ndjson" > "$REVIEW_DIR/review.txt"
  fi
  state=completed
}

if [[ "$mode" == prepare ]]; then
  prepare_review
else
  run_review
fi
