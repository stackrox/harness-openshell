# harness

Deploy a fully sandboxed AI agent in one command.

```bash
harness apply -f agent.yaml
```

One file defines your agent, providers, gateway, and policy. The harness resolves credentials, deploys the gateway, registers providers, and launches a sandbox with deny-by-default L7 egress. Works on local Podman and remote Kubernetes/OpenShift with the same config.

## What You Get

- **Sandbox isolation** -- every agent runs in a container with Landlock filesystem restrictions and deny-by-default network policy
- **Credential proxy** -- secrets are resolved at the gateway boundary, never exposed inside the sandbox
- **Multi-target** -- same agent YAML deploys to local Podman, kind, or OpenShift
- **Declarative config** -- multi-document YAML bundles agent + providers + gateway + policy in one file
- **Dry-run validation** -- `--dry-run` checks gateway, providers, env vars, and image before deploying
- **Config inspection** -- `-o yaml` outputs the fully resolved harness config

## Install

```bash
# macOS
brew tap nvidia/openshell && brew install openshell && brew services start openshell

# Download the harness binary
curl -L https://github.com/robbycochran/harness-openshell/releases/latest/download/harness_darwin_arm64 -o harness
chmod +x harness
```

Or build from source: `make cli`

## Quick Start

```bash
# Set credentials (missing ones are skipped gracefully)
export GITHUB_TOKEN=ghp_...

# Deploy a sandbox
./harness apply

# Interactive mode
./harness apply --attach

# Validate without deploying
./harness apply --dry-run

# See the fully resolved config
./harness apply -o yaml
```

The built-in config registers providers for GitHub, Jira, Vertex AI, and Google Workspace. Providers with missing credentials are skipped with an info message.

## The Agent YAML

```yaml
# profiles/agent-default.yaml
name: agent
entrypoint: claude
tty: true

providers:
  - profile: github
  - profile: vertex-local
  - profile: atlassian
    env:
      JIRA_URL: ${JIRA_URL}
      JIRA_USERNAME: ${JIRA_USERNAME}
  - profile: gws

env:
  ANTHROPIC_BASE_URL: https://inference.local
  ANTHROPIC_API_KEY: sk-ant-openshell-proxy-managed
```

### Multi-Document Harness YAML

Bundle everything in one file:

```yaml
---
kind: agent
name: my-agent
entrypoint: claude
gateway: local
providers:
  - profile: github
---
kind: provider
name: github
type: github
credentials: [GITHUB_TOKEN]
endpoints:
  - { host: "api.github.com", port: 443 }
---
kind: gateway
name: local
type: local
```

```bash
harness apply -f harness.yaml
```

## Targets

```bash
harness apply                        # local Podman (default)
harness apply --gateway ocp          # deploy to OpenShift
harness deploy ocp                   # deploy gateway only
```

The `gateway:` field in the agent YAML or `--gateway` flag selects the target. Gateway profiles live in `profiles/gateways/`.

### OpenCode

```bash
harness apply --agent opencode
```

Same providers and gateway, different agent binary. See `profiles/agent-opencode.yaml`.

## How It Works

```
harness CLI --> openshell CLI --> Gateway (Podman or K8s)
                                   |-- Provider credentials
                                   |-- L7 network policy
                                   |-- inference.local proxy
                                   +-- Sandbox container
                                        |-- claude / opencode
                                        |-- gh, mcp-atlassian, gws
                                        +-- placeholder tokens
```

The harness orchestrates three OpenShell components:

- **Gateway** -- credential proxy and L7 network policy engine. Runs as Podman container (local) or K8s StatefulSet (remote).
- **Providers** -- credential registrations. Provider profiles in `profiles/providers/` are imported to the gateway. Missing credentials are skipped.
- **Sandbox** -- isolated container running the agent entrypoint. Credentials are proxy-managed placeholder tokens. Network egress is deny-by-default at L7.

See the [OpenShell docs](https://github.com/NVIDIA/OpenShell) for the full security model.

## Reference

### Commands

```
harness apply [-f FILE] [--agent NAME] [--gateway NAME] [--attach] [--dry-run] [-o yaml|json]
    Deploy a sandboxed agent. Primary command.
    -f loads a harness/agent YAML file directly.
    --agent selects from profiles/ by name (default: "default").
    --attach enables interactive TTY mode.
    --dry-run validates without deploying.
    -o yaml outputs the fully resolved config.

harness deploy [local|ocp|kind]
    Deploy or verify the gateway for a target.

harness get agents [-o table|json|yaml]
    List running sandboxes. Wraps openshell sandbox list with
    consistent structured output. Aliases: sandboxes, sandbox.

harness get providers [-o table|json|yaml]
    List registered providers. Credentials never included in output.

harness get gateways [-o table|json|yaml]
    List gateways. Aliases: gateway, gw.

harness describe <name>
    Detailed status for a specific sandbox (phase, gateway, providers).

harness delete <name> [<name>...]
    Delete specific sandboxes by name.

harness delete --all
    Delete all sandboxes, providers, and k8s resources.

harness delete --providers / --k8s
    Delete providers or k8s resources selectively.

harness stop [NAME] / harness start [NAME]
    Stop or start a sandbox without deleting it.
```

For sandbox connect/logs, use openshell directly:
```
openshell sandbox connect [NAME]
openshell sandbox logs [NAME] [--tail]
```

### Config Files

| File | Purpose |
|------|---------|
| `profiles/agent-*.yaml` | Agent config: image, entrypoint, providers, env, optional task file |
| `profiles/providers/` | OpenShell provider profiles (imported to gateway on registration) |
| `profiles/gateways/*.yaml` | Gateway profiles: `local.yaml`, `kind.yaml`, `ocp.yaml` |
| `profiles/images/sandbox-default/` | Sandbox image: Dockerfile, policy, MCP configs, Claude settings |

### Credentials

Each provider requires credentials on the host. Missing providers are skipped.

| Provider | Required |
|----------|----------|
| `github` | `GITHUB_TOKEN` env var |
| `vertex-local` | `gcloud auth application-default login` + `ANTHROPIC_VERTEX_PROJECT_ID` + `CLOUD_ML_REGION` |
| `atlassian` | `JIRA_API_TOKEN` + `JIRA_URL` + `JIRA_USERNAME` |
| `gws` | `gws auth login` (OAuth via [gws CLI](https://github.com/googleworkspace/cli)) |

## Documentation Map

| Document | What it is |
|----------|------------|
| [SPEC.md](SPEC.md) | Authoritative behavior spec for the CLI |
| [AGENTS.md](AGENTS.md) | Contributor guide: coding principles, upstream conventions, validation |
| [TODO.md](TODO.md) | Roadmap and known gaps |
| [docs/archive/](docs/archive/README.md) | Historical design docs |
