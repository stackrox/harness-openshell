# OpenShell Harness Specification

Behavior specification for the OpenShell Harness CLI.

## Overview

The harness deploys and manages AI agent sandboxes on three targets:
- **Local** -- Podman containers via a local OpenShell gateway
- **Kind** -- Kubernetes pods via a kind cluster
- **Remote** -- Kubernetes pods via an OpenShift-hosted OpenShell gateway

Each sandbox is an isolated container running an agent entrypoint, with credential providers, network policies, and a rendered payload (env.sh, run.sh, task.md).

## Agent Config

Agent configs live in `agents/*.yaml`. Each declares the sandbox image, entrypoint, providers, and environment:

```yaml
name: agent
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
```

Fields:
- `name` (required) -- sandbox name, used for `openshell sandbox connect`
- `image` -- container image for the sandbox (default: version-matched from ghcr.io, override with `SANDBOX_IMAGE` env)
- `entrypoint` -- command to run (default: `claude`)
- `tty` -- enable TTY (default: false)
- `task` -- path to a task.md file, passed as argument to entrypoint
- `providers` -- list of provider profile references
- `providers[].profile` -- OpenShell provider profile name
- `providers[].config` -- non-secret config vars (resolved via `os.ExpandEnv`)
- `env` -- additional environment variables
- `include` -- extra files to include in the payload
- `policy` -- path to a network policy YAML

Provider profiles live in `agents/providers/profiles/`. These are imported to the gateway during provider registration.

## CLI

### `harness up [--local|--remote] [--agent NAME] [-f FILE] [--name SANDBOX] [--no-tty]`

Full flow: deploy gateway, register providers, render agent config, create sandbox.

1. **Ensure gateway** -- deploy if needed (local: Podman, remote: Helm to K8s/OCP).
2. **Parse agent config** -- read `agents/<name>.yaml` (default: `default`). `-f` overrides with a direct file path.
3. **Ensure providers** -- validate providers declared in the agent config. Auto-register missing ones.
4. **Render payload** -- `env.sh` (resolved env vars), `run.sh` (entrypoint wrapper), `task.md` (if set).
5. **Create sandbox** -- `openshell sandbox create` with `--upload` (payload) and the startup command. Retry up to 5 times for supervisor race conditions.

Local: sandbox created directly via the openshell CLI on the user's machine.
Remote: a runner Job is deployed to the cluster (`harness launch`), which creates the sandbox from inside the cluster with mTLS gateway access.

### `harness create [--agent NAME] [-f FILE] [--name SANDBOX]`

Create a sandbox without deploying the gateway. Errors if no gateway is active.

### `harness connect [NAME]`

Reconnect to a running sandbox via `openshell sandbox connect`.

### `harness deploy [local|ocp|kind]`

Deploy or verify the gateway for a target. Reads `gateways/<target>/gateway.toml`.

### `harness providers [--force]`

Register providers with the gateway. Reads `providers.toml` for the catalog, imports profiles from `agents/providers/profiles/`.

### `harness preflight [--strict]`

Validate local credentials and prerequisites against `providers.toml`.

### `harness status`

Show gateway, provider, and sandbox status. Read-only.

### `harness logs [NAME] [-f|--follow]`

Stream logs for a sandbox (name resolution delegated to `openshell sandbox logs` when NAME is omitted).

### `harness stop [NAME]` / `harness start [NAME]`

Stop or start a sandbox without deleting it. When NAME is omitted and exactly one sandbox is running, it is used; otherwise the command errors.

### `harness teardown [--sandboxes] [--providers] [--k8s]`

Tear down resources. At least one flag required.

### `harness launch` (hidden)

In-cluster command for the runner Job. Reads agent config from `/etc/openshell/sandbox/agent.yaml` (mounted ConfigMap), configures mTLS gateway, renders payload, creates sandbox, sets up environment. Not meant for direct user invocation.

## Config Files

| File | Purpose |
|------|---------|
| `agents/*.yaml` | Agent config: image, entrypoint, providers, env, task |
| `agents/providers/profiles/` | OpenShell provider profile YAMLs |
| `providers.toml` | Provider catalog: required inputs, health checks |
| `gateways/*/gateway.toml` | Deployment target config with Helm, images, RBAC |
| `openshell.toml` | Deployment-level overrides (enabled providers, inference model) |
| `sandbox/Dockerfile` | Sandbox image: OpenShell base + MCP servers + CLI tools |
| `build/runner/Dockerfile` | Runner image: harness binary + openshell CLI |
| `sandbox/policy.yaml` | Network egress rules applied to sandboxes |

## Image Tags

All images are published to `ghcr.io/robbycochran/harness-openshell`. CI never publishes floating tags (`:latest`, `:sandbox`, `:runner`); the bare `:sandbox` fallback below exists only for local `go build` binaries without version ldflags.

| Trigger | Sandbox | Runner |
|---------|---------|--------|
| Release `v0.1.2` | `:sandbox-v0.1.2` | `:runner-v0.1.2` |
| Any push/PR | `:sandbox-<sha>` | `:runner-<sha>` |

The CLI resolves images from its embedded version (set via `-ldflags` at build time):

- `v0.1.2` → `:sandbox-v0.1.2` (tagged release)
- `v0.1.2-5-gabc1234` → `:sandbox-v0.1.2-5-gabc1234` (dev build, matches `make dev-sandbox`)
- `dev` → `:sandbox` (bare `go build` without ldflags)

`SANDBOX_IMAGE` and `RUNNER_IMAGE` env vars override the version-based resolution.

## Environment Variables

| Variable | Purpose |
|----------|---------|
| `SANDBOX_IMAGE` | Override sandbox image (dev/CI builds) |
| `RUNNER_IMAGE` | Override runner image (dev/CI builds) |
| `HARNESS_DIR` | Override harness directory detection |
| `OPENSHELL_NAMESPACE` | Override K8s namespace (default: `openshell`) |
| `OPENSHELL_CLI` | Override openshell binary path |
| `OPENSHELL_MODEL` | Inference model for provider registration (default: `claude-sonnet-4-6`) |
| `OPENSHELL_CHART_VERSION` | Override Helm chart version (beats `openshell.toml` and `gateway.toml`) |
| `PULL_SECRET` / `SANDBOX_PULL_SECRET` | Image pull secret names passed to the Helm install |
| `CONFIG_TOML` / `PROVIDERS_TOML` | Override paths to `openshell.toml` / `providers.toml` (preflight) |
| `KUBECONFIG` | K8s cluster config for remote targets |

`GATEWAY_ENDPOINT` and `GATEWAY_NAME` are internal — set on the in-cluster runner Job, not by users.

## Payload

The harness renders agent config into a self-contained payload uploaded to `/sandbox/.config/openshell/`:

```
openshell/
  env.sh          -- export KEY="value" (resolved from agent config)
  run.sh          -- sources env.sh, validates entrypoint, execs it
  sandbox.env     -- same as env.sh (sourced by sandbox startup.sh)
  task.md         -- task file with envsubst applied
  bin/            -- wrapper scripts
```

`sandbox.env` is sourced as the sandbox's initial command, making env vars available in all subsequent `sandbox exec` sessions. `run.sh` is the entrypoint for interactive mode.
