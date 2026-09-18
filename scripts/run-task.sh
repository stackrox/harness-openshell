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

check_required_providers() {
  local required="${TASK_PROVIDERS:-[]}"
  mkdir -p "$output_dir"
  if ! jq -e 'type == "array" and all(.[]; type == "string" and test("^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$"))' <<< "$required" >/dev/null; then
    echo "TASK_PROVIDERS must be a JSON array of provider names" >&2
    exit 1
  fi

  local report="$output_dir/provider-check.json"
  local provider
  local provider_args=(--gateway "$gateway")
  [[ -n "$workspace" ]] && provider_args+=(--workspace "$workspace")
  if jq -e 'length == 0' <<< "$required" >/dev/null; then
    jq -n --arg gateway "$gateway" --arg workspace "$workspace" \
      '{status:"skipped", gateway:$gateway, workspace:$workspace, providers:[]}' > "$report"
    return
  fi
  while IFS= read -r provider; do
    if ! timeout 60s openshell provider get "${provider_args[@]}" "$provider" >/dev/null 2>&1; then
      jq -n --arg gateway "$gateway" --arg workspace "$workspace" --arg provider "$provider" \
        '{status:"failed", gateway:$gateway, workspace:$workspace, missing:[$provider]}' > "$report"
      echo "required OpenShell provider is unavailable: $provider (gateway=$gateway workspace=${workspace:-default})" >&2
      exit 1
    fi
  done < <(jq -r '.[]' <<< "$required")

  jq -n --arg gateway "$gateway" --arg workspace "$workspace" --argjson providers "$required" \
    '{status:"available", gateway:$gateway, workspace:$workspace, providers:$providers}' > "$report"
}

check_required_providers

args=(workflow apply "$workflow_file" --output-dir "$output_dir" --result-file "$result_file")
[[ -n "$gateway" ]] && args+=(--gateway "$gateway")
[[ -n "$workspace" ]] && args+=(--workspace "$workspace")

exec timeout -s TERM -k "$kill_after" "$task_timeout" ./harness "${args[@]}"
