#!/usr/bin/env bash
# Validate the bounded Codex JSONL event stream used by the PR reviewer.
set -euo pipefail

review_dir="${1:?usage: validate-codex-output.sh REVIEW_DIR}"
while IFS= read -r line || [[ -n "$line" ]]; do
  trimmed="${line#"${line%%[![:space:]]*}"}"
  if [[ "$trimmed" == \{* || "$trimmed" == \[* ]]; then
    jq -e . >/dev/null <<<"$line" || {
      echo "malformed JSON event in Codex output" >&2
      exit 1
    }
  fi
done < "$review_dir/agent.ndjson"

jq -Rse 'split("\n") | map(fromjson?) | . as $events |
  any($events[]; .type == "turn.completed") and
  any($events[];
    .type == "item.completed" and
    .item.type == "agent_message" and
    (.item.text | type == "string" and test("\\S"))
  ) and
  all($events[]; .type != "error" and .type != "turn.failed")
  ' "$review_dir/agent.ndjson" >/dev/null
