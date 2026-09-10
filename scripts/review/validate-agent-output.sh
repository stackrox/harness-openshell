#!/usr/bin/env bash
# Validate the generic OpenCode JSON event contract for a bounded agent run.
set -euo pipefail

review_dir="${1:?usage: validate-agent-output.sh REVIEW_DIR}"
jq -Rse 'split("\n") | map(fromjson?) |
  any(.[]; .type == "text" and (.part.text | type == "string" and test("\\S"))) and
  any(.[]; .type == "step_finish" and .part.reason == "stop") and
  all(.[]; .type != "error" and
    (.type != "tool_use" or
      (.part.state.status == "completed" and
        ((.part.state.metadata.exit // -1) == 0 or
          ((.part.state.metadata.exit // -1) == 1 and
            ((.part.state.output // .part.state.error // "") | test("422|unprocessable entity|comment.*(position|line)"; "i")))))) and
    (.type != "step_finish" or .part.reason == "stop" or .part.reason == "tool-calls"))
  ' "$review_dir/agent.ndjson" >/dev/null
