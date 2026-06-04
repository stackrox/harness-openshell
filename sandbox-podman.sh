#!/usr/bin/env bash
# Launch a sandbox on the local Podman gateway.
#
# Prerequisites:
#   ./deploy-podman.sh        # verify gateway running
#   ./setup-providers.sh      # register providers
#
# Usage:
#   ./sandbox-podman.sh              # uses agents/default.toml
#   ./sandbox-podman.sh research     # uses agents/research.toml
#
# To reconnect: openshell sandbox connect <name>
# To delete:    openshell sandbox delete <name>
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib/common.sh"
require_cli
require_local_gateway

AGENT_NAME="${1:-default}"
AGENT_FILE="$SCRIPT_DIR/agents/${AGENT_NAME}.toml"
[[ -f "$AGENT_FILE" ]] || { echo "ERROR: $AGENT_FILE not found."; exit 1; }

# Parse agent config
eval "$(python3 -c "
import tomllib, sys, shlex
with open(sys.argv[1], 'rb') as f:
    c = tomllib.load(f)
print(f'SANDBOX_IMAGE={shlex.quote(c.get(\"image\", \"\"))}')
print(f'SANDBOX_COMMAND={shlex.quote(c.get(\"command\", \"claude --bare\"))}')
providers = c.get('providers', [])
print(f'SANDBOX_PROVIDERS={shlex.quote(\" \".join(providers))}')
env = c.get('env', {})
lines = [f'export {k}={v}' for k, v in env.items()]
print(f'SANDBOX_ENV={shlex.quote(chr(10).join(lines) + chr(10))}')
" "$AGENT_FILE")"

# Build provider flags
PROVIDER_FLAGS=()
echo "=== Providers ==="
for name in $SANDBOX_PROVIDERS; do
  if "$CLI" provider get "$name" &>/dev/null; then
    PROVIDER_FLAGS+=(--provider "$name")
    echo "  $name: attached"
  else
    echo "  $name: not registered (skipping)"
  fi
done

# Stage files for upload to /sandbox/.config/openshell/ (same as OCP launcher)
HARNESS_DIR="/tmp/openshell"
mkdir -p "$HARNESS_DIR"

# Sandbox env vars from agent config
if [[ -n "$SANDBOX_ENV" ]]; then
  echo "$SANDBOX_ENV" > "$HARNESS_DIR/sandbox.env"
  echo "  Env: $(wc -l < "$HARNESS_DIR/sandbox.env") vars"
fi

# GWS credentials
if command -v gws &>/dev/null && gws auth status &>/dev/null; then
  gws auth export --unmasked > "$HARNESS_DIR/credentials.json" 2>/dev/null
  cp ~/.config/gws/client_secret.json "$HARNESS_DIR/client_secret.json" 2>/dev/null || true
  echo "  GWS: exported"
else
  echo "  GWS: not configured (skipping)"
fi

# Image flag
FROM_FLAGS=()
if [[ -n "$SANDBOX_IMAGE" ]]; then
  FROM_FLAGS=(--from "$SANDBOX_IMAGE")
fi

echo ""
echo "=== Creating sandbox ==="
for attempt in 1 2 3; do
  "$CLI" sandbox create \
    --tty \
    ${FROM_FLAGS[@]+"${FROM_FLAGS[@]}"} \
    ${PROVIDER_FLAGS[@]+"${PROVIDER_FLAGS[@]}"} \
    --upload "$HARNESS_DIR:/sandbox/.config" --no-git-ignore \
    -- bash -c '. /sandbox/startup.sh && exec '"$SANDBOX_COMMAND" \
    && exit 0
  echo "  Attempt $attempt failed (supervisor race), retrying in 5s..."
  "$CLI" sandbox delete "$("$CLI" sandbox list 2>/dev/null | awk 'NR==2{print $1}')" 2>/dev/null || true
  sleep 5
done
echo "ERROR: Failed after 3 attempts."
exit 1
