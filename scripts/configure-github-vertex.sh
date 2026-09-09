#!/usr/bin/env bash
set -euo pipefail

# Configure the repository's Vertex smoke-test credentials from a service-account key.
# The key is streamed directly to gh; its contents are never printed.

repo="${GH_REPO:-$(gh repo view --json nameWithOwner --jq '.nameWithOwner')}"
key_file="${1:-acs-ai-svc-accnt.json}"
project="${VERTEX_AI_PROJECT_ID:-acs-ai-677887}"
region="${VERTEX_AI_REGION:-us-central1}"

[[ -f "$key_file" ]] || { echo "error: key file not found: $key_file" >&2; exit 1; }
command -v gh >/dev/null || { echo 'error: gh is required' >&2; exit 1; }

python3 - "$key_file" "$project" <<'PY'
import json
import sys

path, expected_project = sys.argv[1:]
with open(path, encoding="utf-8") as f:
    data = json.load(f)

required = {"type", "project_id", "client_email", "private_key"}
missing = required - data.keys()
if missing:
    raise SystemExit(f"error: service-account JSON is missing: {', '.join(sorted(missing))}")
if data["type"] != "service_account":
    raise SystemExit("error: key is not a service-account key")
if data["project_id"] != expected_project:
    raise SystemExit(
        f"error: key project {data['project_id']!r} does not match {expected_project!r}"
    )
print(f"Using service account: {data['client_email']}")
PY

gh variable set VERTEX_AI_PROJECT_ID --repo "$repo" --body "$project"
gh variable set VERTEX_AI_REGION --repo "$repo" --body "$region"
gh secret set VERTEX_AI_SERVICE_ACCOUNT_KEY --repo "$repo" < "$key_file"

echo "Configured Vertex GitHub Actions settings for $repo"
echo "  VERTEX_AI_PROJECT_ID=$project"
echo "  VERTEX_AI_REGION=$region"
echo "  VERTEX_AI_SERVICE_ACCOUNT_KEY=<uploaded>"
