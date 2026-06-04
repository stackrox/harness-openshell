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
source "$SCRIPT_DIR/lib/agent.sh"
require_cli
require_local_gateway

# Parse args
AGENT_NAME="default"
NAME_OVERRIDE=""
TTY_FLAG="--tty"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --name) NAME_OVERRIDE="$2"; shift 2 ;;
    --no-tty) TTY_FLAG="--no-tty"; shift ;;
    *) AGENT_NAME="$1"; shift ;;
  esac
done

parse_agent "$SCRIPT_DIR/agents/${AGENT_NAME}.toml"

echo "=== Providers ==="
build_provider_flags

# Stage files for upload to /sandbox/.config/openshell/
HARNESS_DIR="/tmp/openshell"
rm -rf "$HARNESS_DIR"
stage_harness_dir "$HARNESS_DIR"

# Build flags
FROM_FLAGS=()
[[ -n "$SANDBOX_IMAGE" ]] && FROM_FLAGS=(--from "$SANDBOX_IMAGE")

NAME_FLAGS=()
[[ -n "$NAME_OVERRIDE" ]] && NAME_FLAGS=(--name "$NAME_OVERRIDE")

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
  if [[ -n "$NAME_OVERRIDE" ]]; then
    "$CLI" sandbox delete "$NAME_OVERRIDE" 2>/dev/null || true
  else
    "$CLI" sandbox delete "$("$CLI" sandbox list 2>/dev/null | strip_ansi | awk 'NR==2{print $1}')" 2>/dev/null || true
  fi
  sleep 5
done
echo "ERROR: Failed after 5 attempts."
exit 1
