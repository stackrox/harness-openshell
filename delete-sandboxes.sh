#!/usr/bin/env bash
# Delete all running sandboxes.
set -euo pipefail

export OPENSHELL_GATEWAY="${GATEWAY_NAME:-ocp}"
CLI="${OPENSHELL_CLI:-openshell}"
command -v "$CLI" &>/dev/null || { echo "ERROR: openshell CLI not found."; exit 1; }

names=$("$CLI" sandbox list 2>/dev/null | awk 'NR>1 {print $1}')
if [[ -z "$names" ]]; then
  echo "No sandboxes running."
  exit 0
fi

for name in $names; do
  echo "Deleting $name..."
  "$CLI" sandbox delete "$name" 2>/dev/null || true
done

echo "Done."
