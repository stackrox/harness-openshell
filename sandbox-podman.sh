#!/usr/bin/env bash
# Launch a sandbox on the local Podman gateway.
#
# Prerequisites:
#   ./deploy-podman.sh        # verify gateway running
#   ./setup-providers.sh      # register providers
#
# Usage:
#   ./sandbox-podman.sh                        # interactive, uses agents/default.toml
#   ./sandbox-podman.sh research               # uses agents/research.toml
#   ./sandbox-podman.sh --name my-agent        # named sandbox
#   ./sandbox-podman.sh --no-tty --name test   # non-interactive (for testing)
#
# To reconnect: openshell sandbox connect <name>
# To delete:    openshell sandbox delete <name>
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib/common.sh"
require_cli
require_local_gateway

# Parse args
AGENT_NAME="default"
SANDBOX_NAME=""
TTY_FLAG="--tty"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --name) SANDBOX_NAME="$2"; shift 2 ;;
    --no-tty) TTY_FLAG="--no-tty"; shift ;;
    *) AGENT_NAME="$1"; shift ;;
  esac
done

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

# Stage files for upload to /sandbox/.config/openshell/
HARNESS_DIR="/tmp/openshell"
mkdir -p "$HARNESS_DIR"

if [[ -n "$SANDBOX_ENV" ]]; then
  echo "$SANDBOX_ENV" > "$HARNESS_DIR/sandbox.env"
  echo "  Env: $(wc -l < "$HARNESS_DIR/sandbox.env") vars"
fi

if command -v gws &>/dev/null && gws auth status &>/dev/null; then
  gws auth export --unmasked > "$HARNESS_DIR/credentials.json" 2>/dev/null
  cp ~/.config/gws/client_secret.json "$HARNESS_DIR/client_secret.json" 2>/dev/null || true
  echo "  GWS: exported"
else
  echo "  GWS: not configured (skipping)"
fi

# Build flags
FROM_FLAGS=()
[[ -n "$SANDBOX_IMAGE" ]] && FROM_FLAGS=(--from "$SANDBOX_IMAGE")

NAME_FLAGS=()
[[ -n "$SANDBOX_NAME" ]] && NAME_FLAGS=(--name "$SANDBOX_NAME")

# Command depends on tty mode
if [[ "$TTY_FLAG" == "--tty" ]]; then
  CMD=(-- bash -c ". /sandbox/startup.sh && exec $SANDBOX_COMMAND")
else
  CMD=(-- bash /sandbox/startup.sh)
fi

echo ""
echo "=== Creating sandbox ==="
for attempt in 1 2 3 4 5; do
  "$CLI" sandbox create \
    $TTY_FLAG \
    ${NAME_FLAGS[@]+"${NAME_FLAGS[@]}"} \
    ${FROM_FLAGS[@]+"${FROM_FLAGS[@]}"} \
    ${PROVIDER_FLAGS[@]+"${PROVIDER_FLAGS[@]}"} \
    --upload "$HARNESS_DIR:/sandbox/.config" --no-git-ignore \
    "${CMD[@]}" \
    && exit 0
  echo "  Attempt $attempt failed (supervisor race), retrying in 5s..."
  if [[ -n "$SANDBOX_NAME" ]]; then
    "$CLI" sandbox delete "$SANDBOX_NAME" 2>/dev/null || true
  else
    "$CLI" sandbox delete "$("$CLI" sandbox list 2>/dev/null | sed 's/\x1b\[[0-9;]*m//g' | awk 'NR==2{print $1}')" 2>/dev/null || true
  fi
  sleep 5
done
echo "ERROR: Failed after 5 attempts."
exit 1
