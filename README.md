# OpenShell Harness

Orchestration CLI for [OpenShell](https://github.com/NVIDIA/OpenShell) AI agent sandboxes.
Automates gateway deployment, provider registration, and sandbox creation across local Podman and remote Kubernetes/OpenShift targets.

## Quick Start

**Prerequisites:** [OpenShell](https://github.com/NVIDIA/OpenShell) installed and running, Podman.

```bash
# macOS
brew tap nvidia/openshell && brew install openshell && brew services start openshell

# Download the harness binary (macOS ARM64 shown -- see Releases for other platforms)
curl -L https://github.com/robbycochran/harness-openshell/releases/latest/download/harness_darwin_arm64 -o harness
chmod +x harness

# Set credentials (any combination -- missing ones are skipped gracefully)
export GITHUB_TOKEN=ghp_...                   # GitHub (gh CLI in sandbox)
export JIRA_API_TOKEN=...                     # Jira (mcp-atlassian MCP server)
export JIRA_URL=https://your-org.atlassian.net
export JIRA_USERNAME=you@company.com

# Launch a sandbox
./harness up
```

The built-in config registers three providers: GitHub, Jira, and Vertex AI. Providers with missing credentials are skipped with an info message -- you don't need all three to get started. The sandbox runs Claude Code with whatever providers are available.

To customize providers or add GWS, create an `agents/default.yaml` in your project directory -- it takes precedence over the builtin. See [Agent Configs](#agent-configs) below.

## Where This Fits

[OpenShell](https://github.com/NVIDIA/OpenShell) provides the runtime: gateway, sandboxes, L7 network policy, and credential proxy. It handles sandbox lifecycle and credential injection once providers are registered, but leaves gateway deployment orchestration, credential validation, and multi-target configuration to the user.

This harness fills a different gap: multi-provider credential management (preflight validation, registration, health checks) across deployment targets (local Podman, kind, OpenShift) with declarative agent configs. It is model-agnostic -- the agent config chooses the entrypoint and inference backend. The harness orchestrates the infrastructure around it.

## Agent Configs

An agent config declares the sandbox image, entrypoint, and which providers to attach:

```yaml
# agents/default.yaml
name: agent
image: ghcr.io/robbycochran/harness-openshell:sandbox
entrypoint: claude
tty: true

providers:
  - profile: github
  - profile: vertex-local
  - profile: atlassian
    config:
      JIRA_URL: ${JIRA_URL}
      JIRA_USERNAME: ${JIRA_USERNAME}
  - profile: gws

env:
  ANTHROPIC_BASE_URL: https://inference.local
  ANTHROPIC_API_KEY: sk-ant-openshell-proxy-managed
```

```bash
harness up
```

Credentials are proxy-managed. The sandbox holds placeholder tokens; real secrets are substituted by the gateway at the network boundary.

For non-interactive task agents, set `task:` and `tty: false`:

```yaml
# agents/demo.yaml
name: demo
entrypoint: claude -p
task: demo/DEMO-TASK.md
tty: false
# ... same providers and env
```

## Local Setup

### Prerequisites

- [OpenShell CLI](https://github.com/NVIDIA/OpenShell) (`brew tap nvidia/openshell && brew install openshell && brew services start openshell` on macOS)
- Podman/Docker
- Go 1.23+ (only needed for building from source)

### Credentials

Each provider requires credentials on the host. The harness validates these before registration. Providers with missing credentials are skipped with an info message.

| Provider | Required |
|----------|----------|
| `github` | `GITHUB_TOKEN` env var |
| `vertex-local` | `gcloud auth application-default login --project <id>` + `ANTHROPIC_VERTEX_PROJECT_ID` + `CLOUD_ML_REGION` env vars |
| `atlassian` | `JIRA_API_TOKEN` + `JIRA_URL` + `JIRA_USERNAME` env vars |
| `gws` | OAuth client secret at `~/.config/gws/client_secret.json` |

See `providers.toml` for the full input schema and health checks per provider.

### Build from Source

```bash
make cli
./harness up
```

For remote OpenShift: `./harness up --remote` (requires `kubectl`, `helm`, cluster access).

## How It Works

The harness orchestrates three OpenShell components via the `openshell` CLI:

- **Gateway** -- OpenShell's credential proxy and L7 network policy engine. Runs as a Podman container (local) or Kubernetes StatefulSet (remote). Manages provider credentials, inference routing, and sandbox lifecycle.
- **Providers** -- Credential registrations on the gateway. Each provider's required inputs (env vars, files, connectivity checks) are declared in `providers.toml`. The harness runs preflight validation before registering.
- **Sandbox** -- Container running the agent entrypoint, configured by `agents/*.yaml`. The gateway injects credentials at the network boundary -- the sandbox process sees proxy-managed placeholder tokens. Network egress is deny-by-default at L7.

```
harness CLI ──→ openshell CLI ──→ Gateway (Podman or K8s)
                                    ├── Provider credentials
                                    ├── L7 network policy
                                    ├── inference.local proxy
                                    └── Sandbox container
                                         ├── claude
                                         ├── gh, mcp-atlassian, gws
                                         └── placeholder tokens
```

See the [OpenShell docs](https://github.com/NVIDIA/OpenShell) for the full security model (L7 policy, Landlock, proxy credential resolution).

## Config Files

| File | Purpose |
|------|---------|
| `agents/*.yaml` | Agent config: image, entrypoint, providers, env, optional task file |
| `agents/providers/profiles/` | OpenShell provider profiles (imported to gateway on registration) |
| `providers.toml` | Provider catalog: required inputs and health checks per provider |
| `gateways/*/gateway.toml` | Deployment target config: `local/` (Podman), `kind/`, `ocp/` (OpenShift) |
| `openshell.toml` | Deployment-level overrides (enabled providers, inference model, chart version) |
| `sandbox/Dockerfile` | Sandbox image: OpenShell base + MCP servers + CLI tools |
| `build/runner/Dockerfile` | Runner image: harness binary for in-cluster sandbox creation |
| `sandbox/policy.yaml` | Network egress rules applied to sandboxes |

## Commands

```
harness up [--remote] [--agent NAME] [-f FILE] [--name SANDBOX]
    Deploy gateway + register providers + create sandbox.
    Defaults to local gateway (use --remote for OCP).
    --agent defaults to "default" (embedded or agents/default.yaml).
    -f renders any agent YAML file directly.

harness create [--agent NAME] [-f FILE] [--name SANDBOX]
    Create a sandbox without deploying the gateway. Assumes gateway is running.

harness connect [NAME]
    Reconnect to a running sandbox.

harness deploy [local|ocp|kind]
    Deploy or verify the gateway for a target.

harness providers [--force]
    Register providers with the gateway. --force re-registers all.

harness preflight [--strict]
    Validate local credentials and prerequisites.

harness status
    Show gateway, provider, and sandbox status.

harness logs [NAME] [-f]
    Stream sandbox logs (-f to follow).

harness stop [NAME] / harness start [NAME]
    Stop or start a sandbox without deleting it.

harness teardown [--sandboxes] [--providers] [--k8s]
    Tear down resources. At least one flag required.
```

## Documentation Map

| Document | What it is |
|----------|------------|
| [SPEC.md](SPEC.md) | **Authoritative** behavior spec for the CLI — commands, configs, payload |
| [AGENTS.md](AGENTS.md) | Contributor guide: coding principles, workaround tracking, validation modes |
| [TODO.md](TODO.md) | Roadmap and known gaps |
| [docs/archive/](docs/archive/README.md) | Historical design docs (e.g. the June 2026 design-v1 proposal) — outdated, kept for context |
| [docs/release-plan.md](docs/release-plan.md) | Release phases: CI (done), embed + `harness init`, GoReleaser |
| [docs/proto-migration.md](docs/proto-migration.md) | Deferred plan to adopt proto-generated config types |
