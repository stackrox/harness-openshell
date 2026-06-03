#!/usr/bin/env bash
# In-cluster sandbox launcher.
#
# Runs as a Kubernetes Job. Reads sandbox configuration from environment
# variables (sourced from the ConfigMap via envFrom) and credentials from
# volume-mounted Secrets. Calls openshell sandbox create in-cluster.
#
# Environment (from ConfigMap):
#   SANDBOX_NAME      — sandbox name
#   SANDBOX_KEEP      — "true" to keep sandbox after exit (default: "true")
#   SANDBOX_PROVIDERS — space-separated provider names (default: "github vertex-local atlassian")
#   SANDBOX_COMMAND   — command to run (default: "claude --bare")
#
# Secrets mounted at:
#   /secrets/gws/credentials.json    — decrypted GWS OAuth credentials
#   /secrets/gws/client_secret.json  — GWS OAuth client config
#   /secrets/atlassian/JIRA_URL      — Atlassian site URL
#   /secrets/atlassian/JIRA_USERNAME — Atlassian username
#
# Provider credentials (GITHUB_TOKEN, JIRA_API_TOKEN, etc.) are managed
# by the OpenShell gateway provider system — they never appear here.
set -euo pipefail

GATEWAY_ENDPOINT="${GATEWAY_ENDPOINT:-https://openshell.openshell.svc.cluster.local:8080}"
CLI="${OPENSHELL_CLI:-openshell}"
export OPENSHELL_GATEWAY_ENDPOINT="$GATEWAY_ENDPOINT"

# Configure mTLS from mounted secret (openshell-client-tls)
MTLS_DIR="${MTLS_DIR:-/secrets/mtls}"
if [[ -f "$MTLS_DIR/tls.crt" ]]; then
  # Register the in-cluster gateway with mTLS certs
  mkdir -p "$HOME/.config/openshell/gateways/openshell/mtls"
  cp "$MTLS_DIR/ca.crt"  "$HOME/.config/openshell/gateways/openshell/mtls/ca.crt"  2>/dev/null || true
  cp "$MTLS_DIR/tls.crt" "$HOME/.config/openshell/gateways/openshell/mtls/tls.crt" 2>/dev/null || true
  cp "$MTLS_DIR/tls.key" "$HOME/.config/openshell/gateways/openshell/mtls/tls.key" 2>/dev/null || true
  "$CLI" gateway add "$GATEWAY_ENDPOINT" --name openshell --local 2>&1 || true
  export OPENSHELL_GATEWAY=openshell
  echo "  Gateway registered, certs: $(ls $HOME/.config/openshell/gateways/openshell/mtls/ 2>/dev/null || echo 'MISSING')"
else
  # No client cert — try without mTLS (requires allowUnauthenticatedUsers=true
  # and gateway TLS configured to not require client certs)
  export OPENSHELL_GATEWAY_INSECURE=true
fi

SANDBOX_NAME="${SANDBOX_NAME:-agent}"
SANDBOX_KEEP="${SANDBOX_KEEP:-true}"
SANDBOX_PROVIDERS="${SANDBOX_PROVIDERS:-github vertex-local atlassian}"
SANDBOX_COMMAND="${SANDBOX_COMMAND:-claude --bare}"

echo "=== Sandbox Launcher ==="
echo "  Name:      $SANDBOX_NAME"
echo "  Providers: $SANDBOX_PROVIDERS"
echo "  Gateway:   $GATEWAY_ENDPOINT"
echo ""

# ── Build provider flags ───────────────────────────────────────────────
PROVIDER_FLAGS=()
for name in $SANDBOX_PROVIDERS; do
  if "$CLI" provider get "$name" &>/dev/null; then
    PROVIDER_FLAGS+=(--provider "$name")
    echo "  Provider $name: attached"
  else
    echo "  Provider $name: not registered (skipping)"
  fi
done

# ── Stage credentials from mounted secrets ─────────────────────────────
# GWS credentials (mounted from openshell-gws secret)
# TODO: upload GWS files once the supervisor race condition is resolved.
# For now, GWS requires the local sandbox.sh workflow.
# Tracking: https://github.com/NVIDIA/OpenShell/issues/1268
UPLOAD_ARGS=()
if [[ -f /secrets/gws/credentials.json ]]; then
  echo "  GWS credentials: mounted (upload skipped — see #1268)"
else
  echo "  GWS: not mounted (skipping)"
fi

# Atlassian config sourced from env vars (injected via secretKeyRef in Job spec)
if [[ -n "${JIRA_URL:-}" ]]; then
  echo "  Atlassian config: $JIRA_URL"
else
  echo "  Atlassian: JIRA_URL not set (skipping)"
fi

# ── Keep flag ──────────────────────────────────────────────────────────
KEEP_ARGS=()
[[ "$SANDBOX_KEEP" != "true" ]] && KEEP_ARGS=(--no-keep)

# ── Create sandbox ─────────────────────────────────────────────────────
echo ""
echo "=== Creating sandbox ==="
# Create the sandbox (--keep so it stays alive after the launcher exits).
# The sandbox runs startup.sh then drops to a shell — you connect to it
# interactively with: openshell sandbox connect $SANDBOX_NAME
for attempt in 1 2 3; do
  "$CLI" sandbox create \
    --name "$SANDBOX_NAME" \
    --no-tty \
    ${PROVIDER_FLAGS[@]+"${PROVIDER_FLAGS[@]}"} \
    -- bash /sandbox/startup.sh \
    && break
  echo "Attempt $attempt failed (supervisor race), retrying in 5s..."
  "$CLI" sandbox delete "$SANDBOX_NAME" 2>/dev/null || true
  sleep 5
  [[ $attempt -eq 3 ]] && { echo "ERROR: Failed after 3 attempts."; exit 1; }
done

echo ""
echo "Sandbox '$SANDBOX_NAME' is ready."
echo "Connect with: openshell sandbox connect $SANDBOX_NAME"
echo "Or from inside the sandbox: claude --bare"
