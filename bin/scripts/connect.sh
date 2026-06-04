#!/usr/bin/env bash
# Connect to a running sandbox.
set -euo pipefail
CLI="${OPENSHELL_CLI:-openshell}"
exec "$CLI" sandbox connect "${1:-}"
