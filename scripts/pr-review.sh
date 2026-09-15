#!/usr/bin/env bash
# Review using an already-configured OpenShell target. PR content is data.
set -euo pipefail
umask 077
cd "$(dirname "$0")/.."
: "${REVIEW_DIR:?set an absolute artifact directory}"
: "${REVIEW_REPOSITORY:?set owner/repository}" "${REVIEW_PR:?set PR number}"
[[ "$REVIEW_DIR" == /* && "$REVIEW_REPOSITORY" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ && "$REVIEW_PR" =~ ^[1-9][0-9]*$ ]] || exit 1
mode="${1:?usage: pr-review.sh prepare|run}"
[[ "$mode" == prepare || "$mode" == run ]] || exit 1
allow_draft_reviews="${ALLOW_DRAFT_REVIEWS:-false}"
max_diff_bytes=262144
apply_pid=""
head="${REVIEW_HEAD:-}"
base=""
state=failed
endpoint="repos/$REVIEW_REPOSITORY/pulls/$REVIEW_PR"

write_summary() {
  [[ -d "$REVIEW_DIR" ]] || return
  {
    printf '## AI review: %s\n\nPR #%s; head: %s\n\n' "$state" "$REVIEW_PR" "$head"
    printf 'Advisory review; the agent may have posted inline comments. Artifacts are not an approval or a validated finding list.\n'
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
  # The runner owns sandbox deletion, including on normal cancellation.
  if [[ -n "$apply_pid" ]]; then
    # timeout can exit before its command child handles a forwarded signal.
    # Signal the runner child first, then reap the timeout wrapper.
    runner_pids="$(pgrep -P "$apply_pid" 2>/dev/null || true)"
    for runner_pid in $runner_pids; do
      kill -TERM "$runner_pid" 2>/dev/null || true
    done
    kill -TERM "$apply_pid" 2>/dev/null || true
    wait "$apply_pid" || true
  fi
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
  if ! jq -e --arg head "$head" --arg base "$base" --arg allow_drafts "$allow_draft_reviews" '
    .state == "open" and (($allow_drafts == "true") or (.draft | not)) and any(.labels[]?; .name == "ai-review")
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
  export REVIEW_DIFF="$REVIEW_DIR/pr.diff"
  export REVIEW_POLICY="$REVIEW_DIR/review-policy.yaml"
  export REVIEW_SKILL="${REVIEW_SKILL:-$PWD/tasks/github-pr-reviewer/workflow/skills/pr-review/SKILL.md}"
  policy_template="${REVIEW_POLICY_TEMPLATE:-tasks/github-pr-reviewer/openshell/policy.yaml}"
  sed \
    -e "s|\${REVIEW_REPOSITORY}|$REVIEW_REPOSITORY|g" \
    -e "s|\${REVIEW_PR}|$REVIEW_PR|g" \
    "$policy_template" > "$REVIEW_POLICY"

  # OpenShell limits resource names to 19 characters.
  sandbox_name="review-$(openssl rand -hex 6)"
  (
    ulimit -f 2048 # Bound raw diagnostic output as well as runtime.
    exec timeout -s TERM -k 35s 8m ./harness workflow apply tasks/github-pr-reviewer/workflow/opencode-harness.yaml \
      --name "$sandbox_name" --result-file "$REVIEW_DIR/execution.json"
  ) > "$REVIEW_DIR/agent.ndjson" 2> "$REVIEW_DIR/agent.stderr" &
  apply_pid=$!
  set +e
  wait "$apply_pid"
  apply_status=$?
  set -e
  apply_pid=""

  ((apply_status == 0)) || return "$apply_status"
  scripts/review/validate-agent-output.sh "$REVIEW_DIR"
  jq -e '.status == "succeeded" and .phase == "complete"' "$REVIEW_DIR/execution.json" >/dev/null
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
