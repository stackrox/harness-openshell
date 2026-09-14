#!/usr/bin/env bash
# Validate the bounded OpenCode event stream used by the PR reviewer.
set -euo pipefail

review_dir="${1:?usage: validate-agent-output.sh REVIEW_DIR}"
while IFS= read -r line || [[ -n "$line" ]]; do
  trimmed="${line#${line%%[![:space:]]*}}"
  if [[ "$trimmed" == \{* || "$trimmed" == \[* ]]; then
    jq -e . >/dev/null <<<"$line" || {
      echo "malformed JSON event in agent output" >&2
      exit 1
    }
  fi
done < "$review_dir/agent.ndjson"

# OpenCode may retry a malformed shell invocation. Only shell parser failures
# and the expected GitHub diff-location rejection are recoverable; every other
# nonzero tool result remains fatal.
jq -Rse 'split("\n") | map(fromjson?) | . as $events |
  def recoverable_comment_location_failure:
    (.part.state.metadata.exit // -1) == 1 and
    ((.part.state.output // .part.state.error // "") |
      test("comment[[:space:]]+(position|line)[[:space:]]+(is|was)[[:space:]]+(invalid|unresolvable|not[[:space:]]+part[[:space:]]+of[[:space:]]+the[[:space:]]+diff)"; "i") or
      (test("422|unprocessable[[:space:]]+entity"; "i") and
        test("comment|review|pull[[:space:]]+request"; "i") and
        test("position|line|side|diff[[:space:]]+hunk"; "i")))
    ;
  def recoverable_shell_parse_failure:
    (.part.state.metadata.exit // -1) > 0 and
    ((.part.state.output // .part.state.error // "") |
      test("unexpected EOF while looking for matching|syntax error near unexpected token"; "i"))
    ;
  any($events[]; .type == "text" and (.part.text | type == "string" and test("\\S"))) and
  any($events[]; .type == "step_finish" and .part.reason == "stop") and
  all($events[];
    (.type != "error") and
    (
      .type != "tool_use" or
      (
        .part.state.status == "completed" and
        (
          (.part.state.metadata.exit // -1) == 0 or
          recoverable_comment_location_failure or
          recoverable_shell_parse_failure
        )
      )
    ) and
    (.type != "step_finish" or .part.reason == "stop" or .part.reason == "tool-calls")
  )
  ' "$review_dir/agent.ndjson" >/dev/null
