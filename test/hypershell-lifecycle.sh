#!/usr/bin/env bash
# Canonical remote lifecycle against a managed HyperShell gateway, run LOCALLY.
#
# Drives the same path CI would: build the CLI, `harness apply` a throwaway
# sandbox via the OIDC service account, exec the agent, and assert the marker
# `canonical-sdk-ok`. The sandbox is keep:false, so the gateway deletes it when
# the run ends (create -> exec -> auto-delete).
#
# Why local, not GitHub Actions: the managed gateway is public, but the OIDC
# issuer (Keycloak) resolves to private RFC1918 IPs (the ROSA cluster apps
# ingress), so a public GitHub-hosted runner times out on issuer discovery
# during the client-credentials flow. You must run this from inside the Red Hat
# network (VPN). A pre-flight below checks issuer reachability and says so.
#
# Credentials come from a git-excluded SA env file (never committed). Point
# HYPERSHELL_SA_ENV at it. It must define (see hypershell-service-account-*.env):
#   HYPERSHELL_GATEWAY, OPENSHELL_OIDC_ISSUER, OPENSHELL_OIDC_AUDIENCE,
#   OPENSHELL_OIDC_CLIENT_ID, OPENSHELL_OIDC_CLIENT_SECRET
#
# Usage:
#   HYPERSHELL_SA_ENV=./hypershell-service-account-user.env ./test/hypershell-lifecycle.sh
#   make test-hypershell HYPERSHELL_SA_ENV=./hypershell-service-account-user.env
set -uo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
WORKFLOW_FILE="$ROOT_DIR/test/hypershell-workflow.yaml"

: "${HYPERSHELL_SA_ENV:?set HYPERSHELL_SA_ENV=path/to/sa.env (git-excluded SA credentials)}"
[[ -f "$HYPERSHELL_SA_ENV" ]] || { echo "ERROR: SA env file not found: $HYPERSHELL_SA_ENV" >&2; exit 1; }
[[ -f "$WORKFLOW_FILE" ]]     || { echo "ERROR: workflow file not found: $WORKFLOW_FILE" >&2; exit 1; }

# Load SA credentials (subshell-safe: only the vars we map are re-exported).
set -a; # shellcheck disable=SC1090
. "$HYPERSHELL_SA_ENV"; set +a

# Map the SA file's OPENSHELL_OIDC_* names onto the vars the config expands.
export HYPERSHELL_GATEWAY="${HYPERSHELL_GATEWAY:-}"
export HYPERSHELL_OIDC_ISSUER="${OPENSHELL_OIDC_ISSUER:-}"
export HYPERSHELL_OIDC_AUDIENCE="${OPENSHELL_OIDC_AUDIENCE:-}"
export HYPERSHELL_SANDBOX_SA_ID="${OPENSHELL_OIDC_CLIENT_ID:-}"
export OPENSHELL_OIDC_CLIENT_SECRET="${OPENSHELL_OIDC_CLIENT_SECRET:-}"

miss=()
[[ -n "$HYPERSHELL_GATEWAY"          ]] || miss+=(HYPERSHELL_GATEWAY)
[[ -n "$HYPERSHELL_OIDC_ISSUER"      ]] || miss+=(OPENSHELL_OIDC_ISSUER)
[[ -n "$HYPERSHELL_OIDC_AUDIENCE"    ]] || miss+=(OPENSHELL_OIDC_AUDIENCE)
[[ -n "$HYPERSHELL_SANDBOX_SA_ID"    ]] || miss+=(OPENSHELL_OIDC_CLIENT_ID)
[[ -n "$OPENSHELL_OIDC_CLIENT_SECRET" ]] || miss+=(OPENSHELL_OIDC_CLIENT_SECRET)
((${#miss[@]}==0)) || { echo "ERROR: $HYPERSHELL_SA_ENV is missing: ${miss[*]}" >&2; exit 1; }

secret_state() { [[ -n "${1:-}" ]] && printf 'set (%d chars)' "${#1}" || printf 'MISSING'; }
echo "=== HyperShell local lifecycle ==="
printf '  %-16s %s\n' gateway   "$HYPERSHELL_GATEWAY"
printf '  %-16s %s\n' issuer    "$HYPERSHELL_OIDC_ISSUER"
printf '  %-16s %s\n' audience  "$HYPERSHELL_OIDC_AUDIENCE"
printf '  %-16s %s\n' "SA client" "$HYPERSHELL_SANDBOX_SA_ID"
printf '  %-16s %s\n' "SA secret" "$(secret_state "$OPENSHELL_OIDC_CLIENT_SECRET")"

# Pre-flight: the OIDC issuer is VPN-only. Fail early with a clear message
# rather than after a 30s token-discovery timeout inside harness.
well_known="${HYPERSHELL_OIDC_ISSUER%/}/.well-known/openid-configuration"
if ! curl -fsS --max-time 8 -o /dev/null "$well_known" 2>/dev/null; then
  echo "ERROR: OIDC issuer unreachable: $well_known" >&2
  echo "       The issuer is on private (VPN-only) IPs. Are you on the Red Hat network?" >&2
  exit 1
fi
echo "  issuer reachable: yes"

# Build the CLI if it isn't already present at the repo root.
HARNESS_BIN="$ROOT_DIR/harness"
if [[ ! -x "$HARNESS_BIN" ]]; then
  echo "building harness ..."
  ( cd "$ROOT_DIR" && CGO_ENABLED=0 go build -ldflags '-s -w -X main.version=dev' -o harness . ) \
    || { echo "ERROR: harness build failed" >&2; exit 1; }
fi

# Gateway caps sandbox names at 19 chars; keep it short.
name="hsl-$(date +%s)"
echo "=== apply $name ==="
out="$("$HARNESS_BIN" apply "$name" --file "$WORKFLOW_FILE" 2>&1)"; rc=$?
echo "$out"
if [[ $rc -eq 0 ]] && grep -q 'canonical-sdk-ok' <<<"$out"; then
  echo "RESULT: PASS (canonical-sdk-ok; sandbox auto-deleted)"
  exit 0
fi
echo "RESULT: FAIL (apply exit=$rc; marker not found)" >&2
exit 1
