#!/usr/bin/env bash
# In-cluster sandbox launcher.
#
# Runs as a Kubernetes Job. Reads sandbox configuration from a mounted
# ConfigMap (config.yaml) and credentials from volume-mounted Secrets.
# Calls openshell sandbox create in-cluster.
#
# Config mounted at: /etc/openshell/sandbox/config.yaml
# Secrets mounted at: /secrets/gws/, /secrets/mtls/
set -uo pipefail

GATEWAY_ENDPOINT="${GATEWAY_ENDPOINT:-https://openshell.openshell.svc.cluster.local:8080}"
CLI="${OPENSHELL_CLI:-openshell}"
CONFIG="/etc/openshell/sandbox/config.toml"

# ── Configure gateway connection ──────────────────────────────────────
MTLS_DIR="/secrets/mtls"
if [[ -f "$MTLS_DIR/tls.crt" ]]; then
  # Register as http:// (skips cert generation), then fix endpoint + add real certs
  HTTP_ENDPOINT="${GATEWAY_ENDPOINT/https:/http:}"
  "$CLI" gateway add "$HTTP_ENDPOINT" --name openshell 2>/dev/null || true

  # Patch to https and set auth_mode to mtls
  GW_DIR="$HOME/.config/openshell/gateways/openshell"
  mkdir -p "$GW_DIR/mtls"
  python3 -c "
import json
with open('$GW_DIR/metadata.json') as f: d = json.load(f)
d['gateway_endpoint'] = '$GATEWAY_ENDPOINT'
d['auth_mode'] = 'mtls'
with open('$GW_DIR/metadata.json', 'w') as f: json.dump(d, f)
"
  cp "$MTLS_DIR/ca.crt"  "$GW_DIR/mtls/ca.crt"
  cp "$MTLS_DIR/tls.crt" "$GW_DIR/mtls/tls.crt"
  cp "$MTLS_DIR/tls.key" "$GW_DIR/mtls/tls.key"
  echo "  ✓ mTLS gateway configured"
else
  echo "  No mTLS certs, using insecure mode"
  export OPENSHELL_GATEWAY_ENDPOINT="$GATEWAY_ENDPOINT"
  export OPENSHELL_GATEWAY_INSECURE=true
fi

# ── Parse config.toml ─────────────────────────────────────────────────
if [[ ! -f "$CONFIG" ]]; then
  echo "ERROR: Config not found at $CONFIG"
  exit 1
fi

eval "$(python3 -c "
try:
    import tomllib
except ImportError:
    import tomli as tomllib
import sys, shlex
with open(sys.argv[1], 'rb') as f:
    c = tomllib.load(f)
print(f'SANDBOX_NAME={shlex.quote(c.get(\"name\", \"agent\"))}')
print(f'SANDBOX_IMAGE={shlex.quote(c.get(\"image\", \"\"))}')
print(f'SANDBOX_COMMAND={shlex.quote(c.get(\"command\", \"claude --bare\"))}')
print(f'SANDBOX_KEEP={shlex.quote(str(c.get(\"keep\", True)).lower())}')
providers = c.get('providers', [])
print(f'SANDBOX_PROVIDERS={shlex.quote(\" \".join(providers))}')
" "$CONFIG")"

FROM_FLAGS=()
if [[ -n "$SANDBOX_IMAGE" ]]; then
  FROM_FLAGS=(--from "$SANDBOX_IMAGE")
fi

echo "=== Sandbox Launcher ==="
echo "  Name:      $SANDBOX_NAME"
echo "  Image:     $SANDBOX_IMAGE"
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

# ── Stage files for upload to /sandbox/.config/openshell/ ─────────────
HARNESS_DIR="/tmp/openshell"
mkdir -p "$HARNESS_DIR"

# Sandbox env vars (from agent config via ConfigMap)
if [[ -f /etc/openshell/env/sandbox.env ]]; then
  cp /etc/openshell/env/sandbox.env "$HARNESS_DIR/sandbox.env"
  echo "  Env: $(wc -l < "$HARNESS_DIR/sandbox.env") vars"
fi

# GWS credentials
if [[ -f /secrets/gws/credentials.json ]]; then
  cp /secrets/gws/* "$HARNESS_DIR/"
  echo "  GWS: staged"
else
  echo "  GWS: not mounted (skipping)"
fi

# ── Keep flag ──────────────────────────────────────────────────────────
KEEP_ARGS=()
[[ "$SANDBOX_KEEP" != "true" ]] && KEEP_ARGS=(--no-keep)

echo ""
echo "=== Creating sandbox ==="
for attempt in 1 2 3 4 5; do
  "$CLI" sandbox create \
    --name "$SANDBOX_NAME" \
    --no-tty \
    ${FROM_FLAGS[@]+"${FROM_FLAGS[@]}"} \
    ${PROVIDER_FLAGS[@]+"${PROVIDER_FLAGS[@]}"} \
    ${KEEP_ARGS[@]+"${KEEP_ARGS[@]}"} \
    -- true \
    && break
  echo "  Attempt $attempt failed (supervisor race), retrying in 10s..."
  "$CLI" sandbox delete "$SANDBOX_NAME" 2>/dev/null || true
  sleep 10
  [[ $attempt -eq 5 ]] && { echo "ERROR: Failed after 5 attempts."; exit 1; }
done

# Upload all staged files in one shot
echo "  Uploading to /sandbox/.config/openshell/..."
"$CLI" sandbox upload "$SANDBOX_NAME" "$HARNESS_DIR" /sandbox/.config --no-git-ignore

echo "  Running startup..."
"$CLI" sandbox exec --name "$SANDBOX_NAME" -- bash /sandbox/startup.sh

echo ""
echo "Sandbox '$SANDBOX_NAME' is ready."
echo "Connect with: openshell sandbox connect $SANDBOX_NAME"
echo "Or from inside the sandbox: $SANDBOX_COMMAND"
