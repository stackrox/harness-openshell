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
workspace="rev-$RANDOM-$$"
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
  if [[ -n "$apply_pid" ]]; then
    kill -TERM "$apply_pid" 2>/dev/null || true
    wait "$apply_pid" || true
  fi
  if $created_workspace; then
    timeout 30s ./harness delete --gateway "$gateway" --workspace "$workspace" --sandboxes || cleanup_status=1
    if $created_vertex_provider; then
      timeout 30s openshell provider delete --gateway "$gateway" --workspace "$workspace" vertex-review || cleanup_status=1
    fi
    if $created_github_provider; then
      timeout 30s openshell provider delete --gateway "$gateway" --workspace "$workspace" github-review || cleanup_status=1
    fi
    timeout 30s openshell workspace delete --gateway "$gateway" "$workspace" || cleanup_status=1
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
  if ! jq -e --arg head "$head" --arg base "$base" '
    .state == "open" and (.draft | not) and any(.labels[]?; .name == "ai-review")
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
    | head -c 204801 > "$REVIEW_DIR/pr.diff"
  [[ -s "$REVIEW_DIR/pr.diff" && $(wc -c < "$REVIEW_DIR/pr.diff") -le 204800 ]] || exit 1
  (cd "$REVIEW_DIR" && shasum -a 256 pr.diff > pr.diff.sha256)
  [[ -z "${GITHUB_OUTPUT:-}" ]] || printf 'eligible=true\n' >> "$GITHUB_OUTPUT"
  state=prepared
}

validate_agent_output() {
  jq -Rse 'split("\n") | map(fromjson?) |
    any(.[]; .type == "text" and (.part.text | type == "string" and test("\\S"))) and
    any(.[]; .type == "step_finish" and .part.reason == "stop") and
    all(.[]; .type != "error" and
      (.type != "tool_use" or
        (.part.state.status == "completed" and
          ((.part.state.metadata.exit // 0) == 0 or
            ((.part.state.metadata.exit // 0) == 1 and
              ((.part.state.output // .part.state.error // "") | test("422|unprocessable entity|comment.*(position|line)"; "i")))))) and
      (.type != "step_finish" or .part.reason == "stop" or .part.reason == "tool-calls"))
  ' "$REVIEW_DIR/agent.ndjson" >/dev/null
}

run_review() {
  head="$(jq -er '.head | select(test("^[0-9a-f]{40}$"))' "$REVIEW_DIR/input.json")"
  base="$(jq -er '.base | select(test("^[0-9a-f]{40}$"))' "$REVIEW_DIR/input.json")"
  (cd "$REVIEW_DIR" && shasum -a 256 -c pr.diff.sha256 >/dev/null)
  ensure_current
  : "${GOOGLE_VERTEX_AI_TOKEN:?set a short-lived Vertex token}" "${VERTEX_AI_PROJECT_ID:?set Vertex project}"
  : "${GITHUB_TOKEN:?set the workflow GitHub token for provider bootstrap}"

  timeout 60s openshell workspace create --gateway "$gateway" --name "$workspace"
  created_workspace=true
  timeout 60s openshell provider create --gateway "$gateway" --workspace "$workspace" \
    --name vertex-review --type google-vertex-ai --from-existing \
    --config "VERTEX_AI_PROJECT_ID=$VERTEX_AI_PROJECT_ID" --config "VERTEX_AI_REGION=${VERTEX_AI_REGION:-global}"
  created_vertex_provider=true
  timeout 60s openshell provider create --gateway "$gateway" --workspace "$workspace" \
    --name github-review --type github --credential GITHUB_TOKEN
  created_github_provider=true
  timeout 60s openshell inference set --gateway "$gateway" --workspace "$workspace" \
    --provider vertex-review --model gemini-2.5-pro --no-verify

  export REVIEW_DIFF="$REVIEW_DIR/pr.diff"
  export REVIEW_POLICY="$REVIEW_DIR/review-policy.yaml"
  policy_template="${REVIEW_POLICY_TEMPLATE:-examples/github-pr-reviewer/review-policy.yaml}"
  sed \
    -e "s|\${REVIEW_REPOSITORY}|$REVIEW_REPOSITORY|g" \
    -e "s|\${REVIEW_PR}|$REVIEW_PR|g" \
    "$policy_template" > "$REVIEW_POLICY"

  (
    ulimit -f 2048 # Bound raw diagnostic output as well as runtime.
    exec timeout -s TERM -k 35s 8m ./harness apply -f examples/github-pr-reviewer/opencode-harness.yaml \
      --gateway "$gateway" --workspace "$workspace" --result-file "$REVIEW_DIR/execution.json"
  ) > "$REVIEW_DIR/agent.ndjson" 2> "$REVIEW_DIR/agent.stderr" &
  apply_pid=$!
  wait "$apply_pid"
  apply_pid=""

  validate_agent_output
  ensure_current
  jq -Rr 'fromjson? | select(.type == "text") | .part.text' \
    "$REVIEW_DIR/agent.ndjson" > "$REVIEW_DIR/review.txt"
  state=completed
}

if [[ "$mode" == prepare ]]; then
  prepare_review
else
  run_review
fi
