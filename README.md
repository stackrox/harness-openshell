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

## Quick Start

```shell
# 1. Build and push images (one-time, or on version bumps)
docker build --platform linux/amd64 -t quay.io/rcochran/openshell:sandbox sandbox/
docker push quay.io/rcochran/openshell:sandbox

# 2. Deploy OpenShell to the cluster
GATEWAY_IMAGE_REPO=quay.io/rcochran/openshell GATEWAY_IMAGE_TAG=gateway ./deploy-ocp.sh

# 3. Register providers (one-time, or after teardown + redeploy)
export GITHUB_TOKEN="ghp_..."
export JIRA_URL="https://mysite.atlassian.net"
export JIRA_USERNAME="user@example.com"
export JIRA_API_TOKEN="..."
./setup-providers.sh

# 4. Launch a sandbox
./sandbox.sh --name my-agent
```

## Files

| File | Purpose |
|------|---------|
| `deploy-ocp.sh` | Deploy OpenShell (namespace, CRD, SCCs, Helm, route, gateway config) |
| `setup-providers.sh` | Register credential providers (GitHub, Vertex AI, Atlassian) |
| `sandbox.sh` | Launch/rejoin sandboxes with Claude Code |
| `teardown-ocp.sh` | Remove all OpenShell resources from the cluster |
| `sandbox/Dockerfile` | Custom sandbox image (extends community base) |
| `sandbox/policy.yaml` | Network policy (endpoints not covered by provider profiles) |
| `sandbox/startup.sh` | Runtime env wiring + GWS file placement |
| `sandbox/configure-mcp.py` | Generates `.claude.json` MCP server config |
| `sandbox/profiles/atlassian.yaml` | Custom provider v2 profile for Atlassian |
| `sandbox/CLAUDE.md` | Agent instructions baked into sandbox image |
| `sandbox/settings.json` | Claude permissions baked into sandbox image |
| `credentials.md` | Credential flows, mechanisms, and rotation guide |
| `verify-integrations.py` | Integration test script |
| `future-ideas.md` | Roadmap (observability, CronJobs, web UI) |

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
./sandbox.sh --name dev                 # interactive session
./sandbox.sh --rejoin dev               # reconnect
./sandbox.sh --name ephemeral --no-keep # delete after exit
./sandbox.sh --editor vscode            # open in VS Code
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
