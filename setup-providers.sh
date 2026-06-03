#!/usr/bin/env bash
# Register credential providers with the OpenShell gateway.
#
# Skips providers that already exist. Use --force to delete and recreate
# all providers (requires no running sandboxes — run ./delete-sandboxes.sh first).
#
# Run once after deploy-ocp.sh. Providers are stored in the gateway database
# and survive redeployments (same PVC).
#
# Prerequisites:
#   - Gateway deployed and reachable (./deploy-ocp.sh)
#   - GITHUB_TOKEN set in environment
#   - gcloud auth application-default login completed
#   - JIRA_URL, JIRA_USERNAME, JIRA_API_TOKEN set in environment
#
# Usage:
#   ./setup-providers.sh           # create missing providers
#   ./setup-providers.sh --force   # delete and recreate all providers
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
GATEWAY_NAME="${GATEWAY_NAME:-ocp}"
export OPENSHELL_GATEWAY="$GATEWAY_NAME"

CLI="${OPENSHELL_CLI:-openshell}"
command -v "$CLI" &>/dev/null || { echo "ERROR: openshell CLI not found."; exit 1; }
command -v jq &>/dev/null || { echo "ERROR: jq is required for ADC parsing."; exit 1; }

FORCE=false
[[ "${1:-}" == "--force" ]] && FORCE=true

VERTEX_PROJECT="${VERTEX_PROJECT:-}"
VERTEX_REGION="${VERTEX_REGION:-us-east5}"
MODEL="${OPENSHELL_MODEL:-claude-sonnet-4-6}"

provider_exists() { "$CLI" provider get "$1" &>/dev/null; }

# ── Force mode: require no running sandboxes ───────────────────────────
if $FORCE; then
  sandboxes=$("$CLI" sandbox list 2>/dev/null | awk 'NR>1 {print $1}')
  if [[ -n "$sandboxes" ]]; then
    echo "ERROR: Cannot --force with running sandboxes. Run ./delete-sandboxes.sh first."
    exit 1
  fi
  for name in github vertex-local atlassian; do
    "$CLI" provider delete "$name" 2>/dev/null || true
  done
  echo "Deleted existing providers."
fi

echo "=== Enabling providers v2 ==="
"$CLI" settings set --global --key providers_v2_enabled --value true --yes

echo ""
echo "=== Importing custom profiles ==="
if $FORCE; then
  for f in "$SCRIPT_DIR"/sandbox/profiles/*.yaml; do
    [[ -f "$f" ]] || continue
    id=$(grep '^id:' "$f" | awk '{print $2}')
    "$CLI" provider profile delete "$id" 2>/dev/null || true
  done
fi
"$CLI" provider profile import --from "$SCRIPT_DIR/sandbox/profiles/" 2>/dev/null || echo "  (already imported)"

echo ""
echo "=== Registering providers ==="

# ── GitHub ─────────────────────────────────────────────────────────────
# The built-in github profile supports read-only access (clone/fetch).
# For push access, use openshell policy set to add git-receive-pack rules.
# See: https://github.com/NVIDIA/OpenShell/blob/main/docs/get-started/tutorials/github-sandbox.mdx
if [[ -n "${GITHUB_TOKEN:-}" ]]; then
  if ! provider_exists github; then
    "$CLI" provider create --name github --type github \
      --credential "GITHUB_TOKEN=${GITHUB_TOKEN}"
    echo "  github — registered"
  else
    echo "  github — exists (use --force to recreate)"
  fi
else
  echo "  github — skipped (GITHUB_TOKEN not set)"
fi

# ── Vertex AI ──────────────────────────────────────────────────────────
ADC="${GOOGLE_APPLICATION_CREDENTIALS:-$HOME/.config/gcloud/application_default_credentials.json}"
if [[ -f "$ADC" ]]; then
  [[ -z "$VERTEX_PROJECT" ]] && VERTEX_PROJECT=$(jq -r '.quota_project_id // empty' "$ADC")
  if [[ -n "$VERTEX_PROJECT" ]]; then
    if ! provider_exists vertex-local; then
      "$CLI" provider create --name vertex-local --type google-vertex-ai \
        --from-gcloud-adc \
        --config "VERTEX_AI_PROJECT_ID=${VERTEX_PROJECT}" \
        --config "VERTEX_AI_REGION=${VERTEX_REGION}"
      echo "  vertex-local — registered (project: $VERTEX_PROJECT)"
    else
      echo "  vertex-local — exists (use --force to recreate)"
    fi
    # Always ensure inference route is set to the configured model
    "$CLI" inference set --provider vertex-local --model "$MODEL" --no-verify
    echo "  inference — model: $MODEL"
  else
    echo "  vertex-local — skipped (VERTEX_PROJECT not set and not in ADC)"
  fi
else
  echo "  vertex-local — skipped (no ADC file at $ADC)"
fi

# ── Atlassian ──────────────────────────────────────────────────────────
# Only JIRA_API_TOKEN is a provider credential (proxy-resolved in Basic auth).
# JIRA_URL and JIRA_USERNAME are non-secret config uploaded by sandbox.sh.
if [[ -n "${JIRA_API_TOKEN:-}" ]]; then
  if ! provider_exists atlassian; then
    "$CLI" provider create --name atlassian --type atlassian \
      --credential "JIRA_API_TOKEN=${JIRA_API_TOKEN}"
    echo "  atlassian — registered"
  else
    echo "  atlassian — exists (use --force to recreate)"
  fi
else
  echo "  atlassian — skipped (JIRA_API_TOKEN not set)"
fi

echo ""
echo "=== Providers ==="
"$CLI" provider list

echo ""
echo "=== Inference ==="
"$CLI" inference get

echo ""
echo "Done. Launch a sandbox with: ./sandbox.sh --name my-agent"
