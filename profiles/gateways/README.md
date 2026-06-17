# profiles/gateways/

Gateway profiles define where and how the OpenShell gateway is deployed.

## Format

```yaml
gateway:
  type: local                    # local or remote
  platform: ocp                  # k8s or ocp (remote only)
  service: route                 # route, nodeport, or loadbalancer (remote only)
  name: openshell-remote-ocp     # gateway name for openshell CLI
  mode: direct                   # direct or launcher (remote only)

chart:
  version: "0.0.59"             # Helm chart version

helm:
  values:                        # Helm values passed to openshell chart
    server:
      auth:
        allowUnauthenticatedUsers: true
    pkiInitJob:
      enabled: true

addons:
  manifests:                     # additional K8s manifests applied after install
    - apiVersion: route.openshift.io/v1
      kind: Route
      metadata:
        name: gateway
      spec:
        tls:
          termination: passthrough
        to:
          kind: Service
          name: openshell

ocp:                             # OpenShift-specific config
  scc-privileged: [openshell]    # ServiceAccounts needing privileged SCC
  scc-anyuid: [openshell]        # ServiceAccounts needing anyuid SCC

secrets:
  mtls: openshell-client-tls     # K8s Secret containing mTLS client certs
```

## Targets

### `local-container.yaml` -- Podman on your machine

The default. Requires openshell installed and running via `brew services start openshell` or equivalent. No Helm, no K8s.

### `helm.yaml` -- local kind cluster

Deploys to a kind cluster. Uses NodePort access (no Ingress needed). TLS disabled for local dev simplicity. Requires `kind create cluster`.

### `openshift.yaml` -- OpenShift cluster

Deploys to an OpenShift cluster with Route-based access and mTLS. Requires `oc login` and cluster-admin for SCC grants.

## Selecting a gateway

```bash
harness apply -f harness.yaml                    # uses local (default)
harness apply -f harness.yaml --gateway openshift      # uses gateways/openshift.yaml
harness apply -f harness.yaml --gateway helm      # uses gateways/helm.yaml
```

Agent configs can also set a default gateway:

```yaml
name: agent
gateway: openshift
```

The `OPENSHELL_GATEWAY` env var works as a fallback.
