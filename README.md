# OpenShell Harness

Orchestration CLI for [OpenShell](https://github.com/NVIDIA/OpenShell) AI agent sandboxes.
Automates gateway deployment, provider registration, and sandbox creation across local Podman and remote Kubernetes/OpenShift targets.

## Quick Start

**Prerequisites:** [OpenShell v0.0.59+](https://github.com/NVIDIA/OpenShell) installed and running, Podman.

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

This harness fills a different gap: multi-provider credential management (inline validation, registration, health checks) across deployment targets (local Podman, kind, OpenShift) with declarative agent configs. It is model-agnostic -- the agent config chooses the entrypoint and inference backend. The harness orchestrates the infrastructure around it.

## What `harness up` Replaces

A single command:

```bash
harness up -f agents/default.yaml
```

replaces this sequence of 8+ `openshell` commands:

```bash
# 1. Enable the providers v2 system
openshell settings set --global --key providers_v2_enabled --value true

# 2. Import custom provider profiles
openshell provider profile import --from agents/providers/profiles/

# 3. Register GitHub (reads GITHUB_TOKEN from environment)
openshell provider create --name github --type github --from-existing

# 4. Register Vertex AI (reads ADC from gcloud login)
openshell provider create --name vertex-local --type google-vertex-ai \
  --from-gcloud-adc \
  --config VERTEX_AI_PROJECT_ID=my-project \
  --config VERTEX_AI_REGION=global

# 5. Register Atlassian (reads JIRA_API_TOKEN from environment)
openshell provider create --name atlassian --type atlassian --from-existing

# 6. Register GWS (multi-step: create, configure refresh, rotate token)
openshell provider create --name gws --type google-workspace \
  --credential GOOGLE_WORKSPACE_CLI_TOKEN=pending
openshell provider refresh configure gws \
  --credential-key GOOGLE_WORKSPACE_CLI_TOKEN \
  --strategy oauth2-refresh-token \
  --material client_id=... \
  --material client_secret=... \
  --material refresh_token=... \
  --secret-material-key client_secret \
  --secret-material-key refresh_token
openshell provider refresh rotate gws \
  --credential-key GOOGLE_WORKSPACE_CLI_TOKEN

# 7. Configure inference routing
openshell inference set --provider vertex-local --model claude-sonnet-4-6 --no-verify

# 8. Create the sandbox with all providers and env vars attached
openshell sandbox create --name agent \
  --from ghcr.io/robbycochran/harness-openshell:sandbox \
  --provider github --provider vertex-local --provider atlassian --provider gws \
  --env ANTHROPIC_BASE_URL=https://inference.local \
  --env ANTHROPIC_API_KEY=sk-ant-openshell-proxy-managed \
  --upload payload:/sandbox/.config --no-git-ignore \
  --tty \
  -- bash /sandbox/.config/openshell/run.sh
```

The harness also handles: local gateway deployment (Podman), version checking (openshell >= v0.0.59), payload rendering (run.sh, task.md, bin/), retry logic on sandbox creation, and graceful skipping of providers whose credentials are not available.

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
name: standup
entrypoint: claude
task: tasks/daily-standup.md
tty: false
# ... same providers and env
```

When `task:` is set, the harness passes its content to the entrypoint via `-p`.

### OpenCode Support

[OpenCode](https://github.com/opencode-ai/opencode) is supported as an alternative entrypoint:

```bash
harness up --agent opencode
```

Uses the same providers and gateway -- just a different agent binary. See `agents/opencode.yaml`.

## Local Setup

### Prerequisites

- [OpenShell CLI v0.0.59+](https://github.com/NVIDIA/OpenShell) (`brew tap nvidia/openshell && brew install openshell && brew services start openshell` on macOS)
- Podman/Docker
- Go 1.23+ (only needed for building from source)

### Credentials

Each provider requires credentials on the host. The harness validates these inline during registration. Providers with missing credentials are skipped with an info message.

| Provider | Required |
|----------|----------|
| `github` | `GITHUB_TOKEN` env var |
| `vertex-local` | `gcloud auth application-default login --project <id>` + `ANTHROPIC_VERTEX_PROJECT_ID` + `CLOUD_ML_REGION` env vars |
| `atlassian` | `JIRA_API_TOKEN` + `JIRA_URL` + `JIRA_USERNAME` env vars |
| `gws` | `gws auth login` (OAuth via [gws CLI](https://github.com/googleworkspace/cli)) |

Provider profiles are defined in `agents/providers/profiles/` and validated inline during registration.

### Build from Source

```bash
make cli
./harness up
```

For remote OpenShift: `./harness up --remote` (requires `kubectl`, `helm`, cluster access).

## How It Works

The harness orchestrates three OpenShell components via the `openshell` CLI:

- **Gateway** -- OpenShell's credential proxy and L7 network policy engine. Runs as a Podman container (local) or Kubernetes StatefulSet (remote). Manages provider credentials, inference routing, and sandbox lifecycle.
- **Providers** -- Credential registrations on the gateway. Provider profiles are declared in `agents/providers/profiles/`. The harness validates credentials inline during registration -- providers with missing credentials are skipped.
- **Sandbox** -- Container running the agent entrypoint (Claude Code or OpenCode), configured by `agents/*.yaml`. The gateway injects credentials at the network boundary -- the sandbox process sees proxy-managed placeholder tokens. Network egress is deny-by-default at L7.

```
harness CLI ──> openshell CLI ──> Gateway (Podman or K8s)
                                    |── Provider credentials
                                    |── L7 network policy
                                    |── inference.local proxy
                                    └── Sandbox container
                                         |── claude / opencode
                                         |── gh, mcp-atlassian, gws
                                         └── placeholder tokens
```

See the [OpenShell docs](https://github.com/NVIDIA/OpenShell) for the full security model (L7 policy, Landlock, proxy credential resolution).

## Config Files

| File | Purpose |
|------|---------|
| `agents/*.yaml` | Agent config: image, entrypoint, providers, env, optional task file |
| `agents/providers/profiles/` | OpenShell provider profiles (imported to gateway on registration) |
| `gateways/*/gateway.yaml` | Deployment target config: `local/` (Podman), `kind/`, `ocp/` (OpenShift) |
| `sandbox/Dockerfile` | Sandbox image: OpenShell base + MCP servers + CLI tools |
| `sandbox/policy.yaml` | Network egress rules applied to sandboxes |
| `sandbox/opencode.json` | MCP server config for OpenCode agent |

## Commands

```
harness up [--remote] [--agent NAME] [-f FILE] [--name SANDBOX] [--provider-refresh]
    Deploy gateway + register providers + create sandbox.
    Defaults to local gateway (use --remote for OCP).
    --agent defaults to "default" (embedded or agents/default.yaml).
    -f renders any agent YAML file directly.
    --provider-refresh deletes and recreates all providers.

harness create [--agent NAME] [-f FILE] [--name SANDBOX]
    Create a sandbox without deploying the gateway.
    Assumes gateway is running. Auto-registers missing providers.

harness connect [NAME]
    Reconnect to a running sandbox.

harness deploy [local|ocp|kind]
    Deploy or verify the gateway for a target.

harness status
    Show sandbox status.

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
| [SPEC.md](SPEC.md) | **Authoritative** behavior spec for the CLI -- commands, configs, payload |
| [AGENTS.md](AGENTS.md) | Contributor guide: coding principles, workaround tracking, validation modes |
| [TODO.md](TODO.md) | Roadmap and known gaps |
| [docs/archive/](docs/archive/README.md) | Historical design docs (e.g. the June 2026 design-v1 proposal) -- outdated, kept for context |
| [docs/release-plan.md](docs/release-plan.md) | Release phases: CI (done), embed + `harness init`, GoReleaser |
| [docs/proto-migration.md](docs/proto-migration.md) | Deferred plan to adopt proto-generated config types |
