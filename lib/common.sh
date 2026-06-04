#!/usr/bin/env bash
# Shared helpers for harness scripts.
#
# Source from any script:
#   source "$(dirname "$0")/lib/common.sh"

# ── CLI detection ──────────────────────────────────────────────────────

# The openshell binary. Override with OPENSHELL_CLI for dev builds.
CLI="${OPENSHELL_CLI:-openshell}"

# Guard: exit if openshell CLI is not on PATH.
# Used by: sandbox-podman.sh, sandbox-ocp.sh, openshell-harness-preflight.sh, deploy-podman.sh
require_cli() {
  command -v "$CLI" &>/dev/null || { echo "ERROR: openshell CLI not found."; exit 1; }
}

# Guard: exit if kubectl is not on PATH.
# Used by: sandbox-ocp.sh, deploy-ocp.sh, setup-creds.sh, teardown-ocp.sh
require_kubectl() {
  command -v kubectl &>/dev/null || { echo "ERROR: kubectl is required."; exit 1; }
}

# ── Gateway validation ────────────────────────────────────────────────

# Get the active gateway name from the CLI.
active_gateway() {
  "$CLI" gateway list 2>/dev/null | awk '/^\*/ {print $2}'
}

# Guard: exit if active gateway is remote (wrong platform).
# Used by: sandbox-podman.sh, deploy-podman.sh
require_local_gateway() {
  local gw
  gw=$(active_gateway)
  if [[ "$gw" == *"-remote-"* ]]; then
    echo "ERROR: Active gateway '$gw' is remote."
    echo "  Switch to local: openshell gateway select openshell-local-podman"
    exit 1
  fi
}

# Guard: exit if active gateway is not remote.
# Used by: sandbox-ocp.sh
require_ocp_gateway() {
  local gw
  gw=$(active_gateway)
  if [[ "$gw" != *"-remote-"* ]]; then
    echo "ERROR: Active gateway '$gw' is not remote."
    echo "  Switch to remote: openshell gateway select openshell-remote-ocp"
    exit 1
  fi
}

# ── Provider detection ─────────────────────────────────────────────────

# Space-separated provider names to attach. Override with OPENSHELL_PROVIDERS.
PROVIDER_NAMES="${OPENSHELL_PROVIDERS:-github vertex-local atlassian}"

# Query the gateway for registered providers, build --provider flags.
# Sets PROVIDER_FLAGS array. Skips unregistered providers with a warning.
# Used by: sandbox-podman.sh, openshell-harness-preflight.sh
detect_providers() {
  PROVIDER_FLAGS=()
  echo "=== Providers ==="
  for name in $PROVIDER_NAMES; do
    if "$CLI" provider get "$name" &>/dev/null; then
      PROVIDER_FLAGS+=(--provider "$name")
      echo "  $name: attached"
    else
      echo "  $name: not registered (skipping)"
    fi
  done
}

# ── Environment pre-flight ─────────────────────────────────────────────

# Check which features are enabled based on harness.toml.
# Reads provider definitions and validates env vars, paths, and commands.
# Does not exit — informational only. Use --strict to fail on missing required.
# Used by: openshell-harness-preflight.sh, or standalone: source lib/common.sh && check_env
check_env() {
  local script_dir
  script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
  python3 "$script_dir/lib/providers.py" check "$@"
}

# Print space-separated names of providers whose inputs are all available.
# Used by: detect_providers (replaces hardcoded PROVIDER_NAMES when TOML exists)
available_providers() {
  local script_dir
  script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
  python3 "$script_dir/lib/providers.py" available
}

# ── Credential staging ─────────────────────────────────────────────────
# These functions stage credentials into a temp dir for --upload into
# sandboxes. Needed because OpenShell doesn't support file-based
# credential projection yet (#1268, #1423).

# Export decrypted GWS OAuth credentials into $outdir/gws-config/.
# The gws CLI encrypts credentials with a machine-specific key, so we
# must decrypt and re-export for use inside the sandbox container.
# Used by: sandbox-podman.sh (via stage_sandbox_creds), setup-creds.sh
export_gws_creds() {
  local outdir="$1"
  if ! command -v gws &>/dev/null; then
    echo "  GWS: not installed (skipping)"
    return 1
  fi
  if ! gws auth status &>/dev/null; then
    echo "  GWS: not authenticated (run 'gws auth login')"
    return 1
  fi
  mkdir -p "$outdir/gws-config"
  if gws auth export --unmasked > "$outdir/gws-config/credentials.json" 2>/dev/null; then
    local gws_dir="${GWS_CONFIG_DIR:-$HOME/.config/gws}"
    [[ -f "$gws_dir/client_secret.json" ]] && cp "$gws_dir/client_secret.json" "$outdir/gws-config/"
    chmod 600 "$outdir/gws-config"/*
    echo "  GWS: exported"
    return 0
  else
    echo "  GWS: export failed (skipping)"
    rm -rf "$outdir/gws-config"
    return 1
  fi
}

# Write a sourceable sandbox.env with non-secret env vars to inject into
# the sandbox. These are values that aren't provider-managed (not secrets)
# but need to be available inside the sandbox. The file is sourced by
# startup.sh. Add new env vars here as integrations grow.
# Used by: sandbox-podman.sh (via stage_sandbox_creds), setup-creds.sh
write_sandbox_env() {
  local outfile="$1"
  mkdir -p "$(dirname "$outfile")"
  local has_vars=false

  {
    # Atlassian non-secret config
    if [[ -n "${JIRA_URL:-}" ]]; then
      echo "export JIRA_URL=\"$JIRA_URL\""
      echo "export JIRA_USERNAME=\"${JIRA_USERNAME:-}\""
      echo "export CONFLUENCE_URL=\"${JIRA_URL%/}/wiki\""
      echo "export CONFLUENCE_USERNAME=\"${JIRA_USERNAME:-}\""
      echo "export CONFLUENCE_API_TOKEN=\"\${JIRA_API_TOKEN:-}\""
      has_vars=true
      echo "  Atlassian: $JIRA_URL" >&2
    fi

    # Add more env var blocks here as needed:
    # if [[ -n "${SOME_VAR:-}" ]]; then
    #   echo "export SOME_VAR=\"$SOME_VAR\""
    #   has_vars=true
    # fi
  } > "$outfile"

  $has_vars || { rm -f "$outfile"; echo "  sandbox.env: no vars to inject (skipping)" >&2; return 1; }
  return 0
}

# Stage all sandbox credentials into a temp dir for --upload.
# Sets UPLOAD_ARGS (array) and STAGE_DIR (path). The caller should not
# clean up STAGE_DIR if using exec (the upload happens during exec).
# Used by: sandbox-podman.sh
stage_sandbox_creds() {
  STAGE_DIR=$(mktemp -d)
  local creds="$STAGE_DIR/creds"
  mkdir -p "$creds"
  local has_uploads=false

  echo "=== Credentials ==="
  export_gws_creds "$creds" && has_uploads=true
  write_sandbox_env "$creds/sandbox.env" && has_uploads=true

  UPLOAD_ARGS=()
  $has_uploads && UPLOAD_ARGS=(--upload "$creds:/sandbox/.harness")
}
