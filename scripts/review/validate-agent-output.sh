#!/usr/bin/env bash
# Compatibility validator for existing callers; parsing lives in the Go adapter.
set -euo pipefail
root="$(cd "$(dirname "$0")/../.." && pwd)"
exec "$root/harness" github review validate-output --dir "${1:?usage: validate-agent-output.sh REVIEW_DIR}"
