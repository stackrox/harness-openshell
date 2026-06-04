#!/usr/bin/env bash
# Shared helpers for harness scripts.
#
# Source from any script:
#   source "$(dirname "$0")/lib/common.sh"

# ── CLI detection ──────────────────────────────────────────────────────

CLI="${OPENSHELL_CLI:-openshell}"

require_cli() {
  command -v "$CLI" &>/dev/null || { echo "ERROR: openshell CLI not found."; exit 1; }
}

require_kubectl() {
  command -v kubectl &>/dev/null || { echo "ERROR: kubectl is required."; exit 1; }
}

# ── Gateway validation ────────────────────────────────────────────────

require_local_gateway() {
  local gw
  gw=$("$CLI" gateway list 2>/dev/null | awk '/127\.0\.0\.1/ {print $1; exit}')
  if [[ -z "$gw" ]]; then
    echo "ERROR: No local gateway registered."
    echo "  Install OpenShell or run: ./deploy-podman.sh"
    exit 1
  fi
}

# ── Environment pre-flight ─────────────────────────────────────────────

check_env() {
  local script_dir
  script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
  python3 "$script_dir/lib/providers.py" check "$@"
}

# ── GWS credential export ─────────────────────────────────────────────
# Used by: setup-creds.sh (K8s secret creation)

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
