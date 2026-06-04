#!/usr/bin/env bash
# Agent config parsing helpers.
#
# Source from any script:
#   source "$(dirname "$0")/lib/agent.sh"
#
# Usage:
#   parse_agent agents/default.toml
#   # Sets: SANDBOX_IMAGE, SANDBOX_COMMAND, SANDBOX_NAME,
#   #       SANDBOX_PROVIDERS, SANDBOX_ENV, SANDBOX_KEEP

parse_agent() {
  local agent_file="$1"
  [[ -f "$agent_file" ]] || { echo "ERROR: $agent_file not found."; exit 1; }

  eval "$(python3 -c "
import tomllib, sys, shlex
with open(sys.argv[1], 'rb') as f:
    c = tomllib.load(f)
print(f'SANDBOX_NAME={shlex.quote(c.get(\"name\", \"agent\"))}')
print(f'SANDBOX_IMAGE={shlex.quote(c.get(\"image\", \"\"))}')
print(f'SANDBOX_COMMAND={shlex.quote(c.get(\"command\", \"claude --bare\"))}')
print(f'SANDBOX_KEEP={shlex.quote(str(c.get(\"keep\", True)).lower())}')
providers = c.get('providers', [])
print(f'SANDBOX_PROVIDERS={shlex.quote(\" \".join(providers))}')
env = c.get('env', {})
lines = [f'export {k}={v}' for k, v in env.items()]
print(f'SANDBOX_ENV={shlex.quote(chr(10).join(lines) + chr(10))}')
" "$agent_file")"
}

# Build provider flags array from SANDBOX_PROVIDERS.
# Sets: PROVIDER_FLAGS array
build_provider_flags() {
  PROVIDER_FLAGS=()
  for name in $SANDBOX_PROVIDERS; do
    if "$CLI" provider get "$name" &>/dev/null; then
      PROVIDER_FLAGS+=(--provider "$name")
      echo "  $name: attached"
    else
      echo "  $name: not registered (skipping)"
    fi
  done
}

# Stage sandbox.env + GWS credentials to a directory for upload.
# The directory name must be "openshell" so upload lands at /sandbox/.config/openshell/.
#
# Usage:
#   stage_harness_dir /tmp/openshell
#   # Creates: $dir/sandbox.env, $dir/credentials.json, $dir/client_secret.json
stage_harness_dir() {
  local dir="$1"
  mkdir -p "$dir"

  if [[ -n "${SANDBOX_ENV:-}" ]]; then
    echo "$SANDBOX_ENV" > "$dir/sandbox.env"
  fi

  if command -v gws &>/dev/null && gws auth status &>/dev/null 2>&1; then
    gws auth export --unmasked > "$dir/credentials.json" 2>/dev/null
    cp ~/.config/gws/client_secret.json "$dir/client_secret.json" 2>/dev/null || true
    echo "  GWS: exported"
  else
    echo "  GWS: not configured (skipping)"
  fi
}

# Strip ANSI escape codes from stdin.
strip_ansi() {
  sed 's/\x1b\[[0-9;]*m//g'
}
