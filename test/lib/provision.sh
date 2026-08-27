#!/usr/bin/env bash
# Gateway provisioning for the integration tests, using UPSTREAM tooling only.
#
# The harness no longer provisions gateways (PR7b): provisioning is OpenShell's
# job. This library reproduces, in bash, exactly what the retired `harness
# deploy` did — the OpenShell installer already stands up the local gateway, and
# `helm install openshell` + `openshell gateway add/select` stand up cluster
# gateways. Sourced by test-flow.sh and kind-lifecycle.sh.
#
# Requires in the environment: CLI (the openshell binary name), and for cluster
# flows: kubectl, helm, a reachable cluster via KUBECONFIG.

# Chart/CRD coordinates — kept in lockstep with the values the retired
# cmd/deploy.go + profiles/gateways/*.yaml used. Version follows
# .openshell-version (the single source of truth), overridable via
# OPENSHELL_CHART_VERSION to match cmd/deploy.go's old behavior.
OPENSHELL_CHART_OCI="${OPENSHELL_CHART_OCI:-oci://ghcr.io/nvidia/openshell/helm-chart}"
OPENSHELL_CRD_URL="${OPENSHELL_CRD_URL:-https://github.com/kubernetes-sigs/agent-sandbox/releases/download/v0.5.0/manifest.yaml}"

_chart_version() {
  if [[ -n "${OPENSHELL_CHART_VERSION:-}" ]]; then
    echo "$OPENSHELL_CHART_VERSION"
    return
  fi
  local root ver
  root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
  ver="$(cat "$root/.openshell-version" 2>/dev/null | tr -d 'v[:space:]')"
  echo "${ver:-0.0.110}"
}

# provision_local: the OpenShell installer already provisioned and started the
# local gateway (see .github/workflows/integration.yml "Install openshell" +
# "Wait for gateway"). Select the 127.0.0.1 registration and confirm it responds.
provision_local() {
  local gw
  gw="$("$CLI" gateway list 2>/dev/null | strip_ansi | awk '/127\.0\.0\.1/ {gsub(/^\*/, ""); print $1; exit}')"
  if [[ -z "$gw" ]]; then
    echo "  ERROR: no local (127.0.0.1) gateway registered — is OpenShell installed and running?" >&2
    return 1
  fi
  "$CLI" gateway select "$gw" || return 1
  local i
  for i in $(seq 1 5); do
    "$CLI" inference get &>/dev/null && return 0
    sleep 3
  done
  echo "  ERROR: local gateway $gw not responding" >&2
  return 1
}

# provision_kind: faithful bash port of cmd/deploy.go deployFromConfig()'s
# nodeport path (profiles/gateways/helm.yaml). Assumes KUBECONFIG points at a
# reachable kind cluster with helm available.
provision_kind() {
  local ver values np ip i
  ver="$(_chart_version)"

  kubectl create ns openshell --dry-run=client -o yaml | kubectl apply -f - || return 1
  kubectl label ns openshell \
    pod-security.kubernetes.io/enforce=privileged \
    pod-security.kubernetes.io/warn=privileged --overwrite || return 1

  kubectl apply -f "$OPENSHELL_CRD_URL" || return 1

  values="$(mktemp /tmp/os-kind-values-XXXXXX.yaml)"
  cat > "$values" <<'EOF'
service:
  type: NodePort
server:
  disableTls: true
  auth:
    allowUnauthenticatedUsers: true
pkiInitJob:
  enabled: true
EOF

  local helm_args=(upgrade --install openshell "$OPENSHELL_CHART_OCI"
    --namespace openshell --version "$ver" --values "$values")
  [[ -n "${HARNESS_OS_IMAGE:-}" ]] && helm_args+=(--set "server.sandboxImage=$HARNESS_OS_IMAGE")
  helm "${helm_args[@]}" || { rm -f "$values"; return 1; }
  rm -f "$values"

  kubectl rollout status statefulset/openshell -n openshell --timeout=300s || return 1

  np="$(kubectl get svc openshell -n openshell -o jsonpath='{.spec.ports[?(@.port==8080)].nodePort}')"
  ip="$(kubectl get nodes -o jsonpath='{.items[0].status.addresses[?(@.type=="InternalIP")].address}')"
  if [[ -z "$np" || -z "$ip" ]]; then
    echo "  ERROR: could not resolve NodePort ($np) / node IP ($ip)" >&2
    return 1
  fi

  # kind runs disableTls=true → register plaintext HTTP (skips mTLS/browser auth).
  "$CLI" gateway remove openshell-kind 2>/dev/null || true
  "$CLI" gateway add "http://$ip:$np" --name openshell-kind --local || return 1
  "$CLI" gateway select openshell-kind || return 1

  for i in $(seq 1 30); do
    "$CLI" inference get &>/dev/null && return 0
    sleep 2
  done
  echo "  ERROR: kind gateway not reachable after 60s" >&2
  return 1
}

# provision_ocp: bash port of deployFromConfig()'s OCP route path
# (profiles/gateways/openshift.yaml). Not run in CI (no OCP cluster there);
# validated locally against real OpenShift. Requires oc/kubectl + helm + a
# route-capable cluster.
provision_ocp() {
  local ver domain route_host i
  ver="$(_chart_version)"

  kubectl create ns openshell --dry-run=client -o yaml | kubectl apply -f - || return 1
  kubectl label ns openshell \
    pod-security.kubernetes.io/enforce=privileged \
    pod-security.kubernetes.io/warn=privileged --overwrite || return 1
  kubectl apply -f "$OPENSHELL_CRD_URL" || return 1

  # SCCs (openshift.yaml ocp.scc-*).
  local sa
  for sa in openshell openshell-sandbox; do
    oc adm policy add-scc-to-user privileged -z "$sa" -n openshell || return 1
  done
  oc adm policy add-scc-to-user anyuid -z openshell -n openshell || return 1

  # Route (passthrough) — apply before helm so the PKI SAN can match.
  domain="$(kubectl get ingresses.config.openshift.io cluster -o jsonpath='{.spec.domain}')"
  if [[ -z "$domain" ]]; then
    echo "  ERROR: could not determine OpenShift apps domain (is this OCP?)" >&2
    return 1
  fi
  route_host="gateway-openshell.$domain"
  kubectl apply -n openshell -f - <<EOF || return 1
apiVersion: route.openshift.io/v1
kind: Route
metadata:
  name: gateway
spec:
  tls:
    termination: passthrough
  to:
    kind: Service
    name: openshell
  port:
    targetPort: grpc
EOF

  local values
  values="$(mktemp /tmp/os-ocp-values-XXXXXX.yaml)"
  cat > "$values" <<'EOF'
image:
  pullPolicy: Always
supervisor:
  image:
    pullPolicy: Always
securityContext:
  runAsUser: null
  runAsNonRoot: null
server:
  sandboxImagePullPolicy: Always
  auth:
    allowUnauthenticatedUsers: true
pkiInitJob:
  enabled: true
EOF
  local helm_args=(upgrade --install openshell "$OPENSHELL_CHART_OCI"
    --namespace openshell --version "$ver" --values "$values"
    --set "pkiInitJob.serverDnsNames[0]=$route_host")
  [[ -n "${HARNESS_OS_IMAGE:-}" ]] && helm_args+=(--set "server.sandboxImage=$HARNESS_OS_IMAGE")
  [[ -n "${HARNESS_OS_PULL_SECRET:-}" ]] && helm_args+=(--set "imagePullSecrets[0].name=$HARNESS_OS_PULL_SECRET")
  [[ -n "${HARNESS_OS_SANDBOX_PULL_SECRET:-}" ]] && helm_args+=(--set "server.sandboxImagePullSecrets[0].name=$HARNESS_OS_SANDBOX_PULL_SECRET")
  helm "${helm_args[@]}" || { rm -f "$values"; return 1; }
  rm -f "$values"

  kubectl rollout status statefulset/openshell -n openshell --timeout=300s || return 1

  # Extract mTLS bundle from the cluster secret and register the route gateway.
  local mtls_dir field
  mtls_dir="$HOME/.config/openshell/gateways/openshell-remote-ocp/mtls"
  mkdir -p "$mtls_dir"
  for field in ca.crt tls.crt tls.key; do
    kubectl get secret openshell-client-tls -n openshell \
      -o jsonpath="{.data.$field}" | base64 -d > "$mtls_dir/$field" || return 1
  done

  "$CLI" gateway remove openshell-remote-ocp 2>/dev/null || true
  "$CLI" gateway add "https://$route_host:443" --name openshell-remote-ocp --local || return 1
  "$CLI" gateway select openshell-remote-ocp || return 1

  for i in $(seq 1 30); do
    "$CLI" inference get &>/dev/null && return 0
    sleep 2
  done
  echo "  ERROR: OCP gateway not reachable after 60s" >&2
  return 1
}

# teardown_cluster: replaces `harness delete --k8s`. helm uninstall + gateway
# deregister + namespace delete. Best-effort (idempotent).
#
# Waits for the namespace to finish deleting before returning: the kind and
# fresh-OCP flows reprovision immediately after teardown, and a create/apply/helm
# against a still-`Terminating` namespace fails. Best-effort — warns rather than
# fails if a finalizer stalls deletion past the timeout.
teardown_cluster() {
  local gw_name="${1:-}"
  helm uninstall openshell -n openshell 2>/dev/null || true
  [[ -n "$gw_name" ]] && "$CLI" gateway remove "$gw_name" 2>/dev/null || true
  kubectl delete ns openshell --ignore-not-found --wait=false 2>/dev/null || true
  local i
  for i in $(seq 1 60); do
    kubectl get ns openshell &>/dev/null || return 0
    sleep 2
  done
  echo "  WARN: namespace 'openshell' still terminating after 120s" >&2
}
