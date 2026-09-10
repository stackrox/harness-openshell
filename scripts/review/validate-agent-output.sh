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

jq -Rse 'split("\n") | map(fromjson?) |
  any(.[]; .type == "text" and (.part.text | type == "string" and test("\\S"))) and
  any(.[]; .type == "step_finish" and .part.reason == "stop") and
  all(.[]; .type != "error" and
    (.type != "tool_use" or
      (.part.state.status == "completed" and
        ((.part.state.metadata.exit // -1) == 0 or
          ((.part.state.metadata.exit // -1) == 1 and
            ((.part.state.output // .part.state.error // "") | test("comment.*(position|line)|(position|line).*comment"; "i")))))) and
    (.type != "step_finish" or .part.reason == "stop" or .part.reason == "tool-calls"))
  ' "$review_dir/agent.ndjson" >/dev/null
