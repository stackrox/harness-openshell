# OpenShell Harness for OpenShift

Deploy OpenShell sandboxes on OpenShift with Claude Code (Vertex AI), Atlassian MCP, Google Workspace, and GitHub integrations.

## What This Is

A deployment harness for running AI agent sandboxes on OpenShift using [OpenShell](https://github.com/NVIDIA/OpenShell). Each sandbox gets:

- **Claude Code** via Google Vertex AI (`inference.local` routing)
- **Jira/Confluence** via mcp-atlassian MCP server (read-only)
- **Gmail, Calendar, Drive** via gws CLI
- **GitHub** via gh CLI (read-only at proxy level)
- Network policy enforcement per sandbox
- Persistent workspace across reconnects

## Prerequisites

- OpenShift cluster with `KUBECONFIG` set
- `kubectl`, `helm` on PATH
- OpenShell CLI (`openshell`) built from source (>= 0.0.55-dev for `google-vertex-ai` provider)
- NVIDIA/OpenShell repo cloned (for the Helm chart)
- `gcloud auth application-default login` completed
- Custom images pushed to `quay.io/rcochran/openshell` (sandbox + gateway)

## Quick Start (Local — Podman/Docker)

```shell
# 1. Install OpenShell (auto-starts the gateway)
curl -LsSf https://raw.githubusercontent.com/NVIDIA/OpenShell/main/install.sh | sh

# 2. Verify gateway is running
./deploy-podman.sh

# 3. Register providers (one-time per gateway)
export GITHUB_TOKEN="ghp_..."
export JIRA_API_TOKEN="..."
./setup-providers.sh

# 4. Launch a sandbox
export JIRA_URL="https://mysite.atlassian.net"
export JIRA_USERNAME="user@example.com"
./sandbox-podman.sh
```

## Quick Start (OpenShift)

```shell
# 1. Build and push images (one-time, or on version bumps)
make push-sandbox push-launcher push-gateway push-supervisor

# 2. Deploy to the cluster
./deploy-ocp.sh

# 3. Store credentials in cluster + register providers
./setup-creds.sh
./setup-providers.sh

# 4. Launch a sandbox
# or: ./sandbox-ocp.sh
```

## Files

| File | Purpose |
|------|---------|
| `deploy-podman.sh` | Verify local gateway is running (Podman/Docker) |
| `deploy-ocp.sh` | Deploy OpenShell to OpenShift (Helm, SCCs, route) |
| `setup-providers.sh` | Register credential providers — works on any gateway |
| `sandbox-podman.sh` | Launch sandbox on local gateway (direct CLI) |
| `sandbox-ocp.sh` | Launch sandbox on OpenShift (kubectl apply) |
| `sandbox/Dockerfile` | Custom sandbox image (extends community base) |
| `sandbox/policy.yaml` | Network policy (endpoints not covered by provider profiles) |
| `sandbox/startup.sh` | Runtime env, GWS, MCP config |
| `sandbox/profiles/atlassian.yaml` | Custom provider v2 profile for Atlassian |
| `sandbox/CLAUDE.md` | Agent instructions baked into sandbox image |
| `sandbox/settings.json` | Claude permissions baked into sandbox image |
| `credentials.md` | Credential flows, mechanisms, and rotation guide |
| `AGENTS.md` | Project principles and workaround tracking |

## Credentials

See [credentials.md](credentials.md) for the full reference.

| Credential | Mechanism | Setup |
|------------|-----------|-------|
| GitHub | Provider v2 (`github` profile, read-only) | `./setup-providers.sh` |
| Vertex AI | Provider v2 (`google-vertex-ai`, `inference.local`) | `./setup-providers.sh` |
| Atlassian | Provider v2 (`atlassian` profile, Basic auth resolved by proxy, read-only) | `./setup-providers.sh` |
| Google Workspace | File upload (encrypted local files) | Pre-authenticate with `gws auth login` |

## Sandbox Usage

```shell
# Local
./sandbox-podman.sh                          # launch
./sandbox-podman.sh --name dev               # named sandbox

# OpenShift
./sandbox-ocp.sh my-config.yaml             # use a custom config

# Either platform
openshell sandbox connect <name>            # reconnect to running sandbox
openshell sandbox list                      # list sandboxes
openshell sandbox delete <name>             # delete a sandbox
```

## Architecture

```
Your Mac                         OpenShift Cluster
┌──────────┐                   ┌──────────────────────────────┐
│ openshell│   OpenShift Route │ Gateway (StatefulSet)         │
│ CLI      ├──────────────────▶│   ├─ gRPC API                 │
│          │   TLS passthrough │   ├─ inference.local proxy     │
│          │   mTLS :443       │   ├─ Provider credential store │
└──────────┘                   │   └─ OAuth token refresh       │
                               │                               │
                               │ Sandbox Pods                  │
                               │   ├─ Claude Code → inference  │
                               │   │   .local → Vertex AI      │
                               │   ├─ mcp-atlassian (read-only)│
                               │   ├─ gws CLI                  │
                               │   ├─ gh CLI (read-only)       │
                               │   └─ Network proxy            │
                               └──────────────────────────────┘
```
