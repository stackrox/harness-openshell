#!/usr/bin/env bash
# Execute one trusted task bundle through the harness workflow runner.
set -euo pipefail
umask 077
cd "$(dirname "$0")/.."

workflow_file="${1:?usage: run-task.sh WORKFLOW OUTPUT_DIR RESULT_FILE}"
output_dir="${2:?usage: run-task.sh WORKFLOW OUTPUT_DIR RESULT_FILE}"
result_file="${3:?usage: run-task.sh WORKFLOW OUTPUT_DIR RESULT_FILE}"

[[ "$output_dir" == /* && "$result_file" == /* ]] || {
  echo "task output and result paths must be absolute" >&2
  exit 1
}
[[ "$workflow_file" != /* && "$workflow_file" != *..* ]] || {
  echo "task workflow must be a trusted repository-relative path" >&2
  exit 1
}

gateway="${OPENSHELL_GATEWAY:-openshell}"
workspace="${OPENSHELL_WORKSPACE:-}"
task_timeout="${TASK_TIMEOUT:-8m}"
kill_after="${TASK_KILL_AFTER:-35s}"
[[ "$task_timeout" =~ ^[0-9]+[smh]$ && "$kill_after" =~ ^[0-9]+[smh]$ ]] || {
  echo "TASK_TIMEOUT and TASK_KILL_AFTER must be durations such as 8m or 35s" >&2
  exit 1
}

args=(workflow apply "$workflow_file" --output-dir "$output_dir" --result-file "$result_file")
[[ -n "$gateway" ]] && args+=(--gateway "$gateway")
[[ -n "$workspace" ]] && args+=(--workspace "$workspace")

exec timeout -s TERM -k "$kill_after" "$task_timeout" ./harness "${args[@]}"
