#!/usr/bin/env bash
# In-cluster sandbox launcher.
#
# Runs as a Kubernetes Job. Reads sandbox configuration from a mounted
# ConfigMap (config.yaml) and credentials from volume-mounted Secrets.
# Calls openshell sandbox create in-cluster.
#
# Config mounted at: /etc/openshell/sandbox/config.yaml
# Secrets mounted at: /secrets/gws/, /secrets/mtls/
set -euo pipefail

GATEWAY_ENDPOINT="${GATEWAY_ENDPOINT:-https://openshell.openshell.svc.cluster.local:8080}"
CLI="${OPENSHELL_CLI:-openshell}"
CONFIG="/etc/openshell/sandbox/config.yaml"

# ── Parse config.yaml ─────────────────────────────────────────────────
if [[ ! -f "$CONFIG" ]]; then
  echo "ERROR: Config not found at $CONFIG"
  exit 1
fi

eval "$(python3 -c "
import yaml, sys, shlex
with open(sys.argv[1]) as f:
    c = yaml.safe_load(f)
print(f'SANDBOX_NAME={shlex.quote(c.get(\"name\", \"agent\"))}')
print(f'SANDBOX_COMMAND={shlex.quote(c.get(\"command\", \"claude --bare\"))}')
print(f'SANDBOX_KEEP={shlex.quote(str(c.get(\"keep\", True)).lower())}')
providers = c.get('providers', [])
print(f'SANDBOX_PROVIDERS={shlex.quote(\" \".join(providers))}')
" "$CONFIG")"

# ── Configure mTLS from mounted secret ────────────────────────────────
MTLS_DIR="${MTLS_DIR:-/secrets/mtls}"
if [[ -f "$MTLS_DIR/tls.crt" ]]; then
  mkdir -p "$HOME/.config/openshell/gateways/openshell/mtls"
  cp "$MTLS_DIR/ca.crt"  "$HOME/.config/openshell/gateways/openshell/mtls/ca.crt"  2>/dev/null || true
  cp "$MTLS_DIR/tls.crt" "$HOME/.config/openshell/gateways/openshell/mtls/tls.crt" 2>/dev/null || true
  cp "$MTLS_DIR/tls.key" "$HOME/.config/openshell/gateways/openshell/mtls/tls.key" 2>/dev/null || true
  "$CLI" gateway add "$GATEWAY_ENDPOINT" --name openshell --local 2>&1 || true
  export OPENSHELL_GATEWAY=openshell
else
  export OPENSHELL_GATEWAY_ENDPOINT="$GATEWAY_ENDPOINT"
  export OPENSHELL_GATEWAY_INSECURE=true
fi

echo "=== Sandbox Launcher ==="
echo "  Name:      $SANDBOX_NAME"
echo "  Providers: $SANDBOX_PROVIDERS"
echo "  Command:   $SANDBOX_COMMAND"
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

# ── GWS credentials ───────────────────────────────────────────────────
if [[ -f /secrets/gws/credentials.json ]]; then
  echo "  GWS: mounted"
else
  echo "  GWS: not mounted (skipping)"
fi

# ── Atlassian config ──────────────────────────────────────────────────
if [[ -n "${JIRA_URL:-}" ]]; then
  echo "  Atlassian: $JIRA_URL"
else
  echo "  Atlassian: not configured (skipping)"
fi

# ── Keep flag ──────────────────────────────────────────────────────────
KEEP_ARGS=()
[[ "$SANDBOX_KEEP" != "true" ]] && KEEP_ARGS=(--no-keep)

echo ""
echo "=== Creating sandbox ==="
for attempt in 1 2 3; do
  "$CLI" sandbox create \
    --name "$SANDBOX_NAME" \
    --no-tty \
    ${PROVIDER_FLAGS[@]+"${PROVIDER_FLAGS[@]}"} \
    ${KEEP_ARGS[@]+"${KEEP_ARGS[@]}"} \
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
echo "Or from inside the sandbox: $SANDBOX_COMMAND"
