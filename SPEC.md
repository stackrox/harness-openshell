# OpenShell Harness Specification

Behavior specification for the OpenShell Harness CLI.

## Overview

The harness deploys and manages AI agent sandboxes on three targets:
- **Local** -- Podman containers via a local OpenShell gateway
- **Kind** -- Kubernetes pods via a kind cluster
- **Remote** -- Kubernetes pods via an OpenShift-hosted OpenShell gateway

Each sandbox is an isolated container running an agent entrypoint (Claude Code or OpenCode), with credential providers, network policies, and a rendered payload (run.sh, task.md).

Requires OpenShell v0.0.59+.

## Agent Config

Agent configs live in `agents/*.yaml`. Each declares the sandbox image, entrypoint, providers, and environment:

```yaml
name: agent
entrypoint: claude      # or: opencode
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
```

Fields:
- `name` (required) -- sandbox name, used for `openshell sandbox connect`
- `image` -- container image for the sandbox (default: version-matched from ghcr.io, override with `SANDBOX_IMAGE` env)
- `entrypoint` -- command to run (default: `claude`). Supports `claude`, `opencode`, `bash`, or any binary on PATH.
- `tty` -- enable TTY (default: false)
- `task` -- path to a task.md file, passed to entrypoint via `-p "$(cat task.md)"`
- `providers` -- list of provider profile references
- `providers[].profile` -- OpenShell provider profile name
- `providers[].env` -- non-secret env vars for this provider (resolved via `os.ExpandEnv`; empty values read from host env; injected via `--env` on sandbox create)
- `env` -- additional environment variables injected via `--env` on sandbox create (empty values read from host env)
- `include` -- extra files to include in the payload
- `policy` -- path to a network policy YAML
- `gateway` -- target gateway name (overrides active gateway)

Provider profiles live in `profiles/providers/`. These are imported to the gateway during provider registration.

## CLI

### `harness up [--gateway NAME] [--gateway-profile FILE] [--agent NAME] [--agent-profile|-f FILE] [--name SANDBOX] [--no-tty] [--provider-refresh]`

Full flow: deploy gateway, register providers, render agent config, create sandbox.

1. **Check version** -- warn if openshell CLI is below v0.0.59.
2. **Ensure gateway** -- deploy if needed (local: Podman, remote: Helm to K8s/OCP). `--gateway` selects a profile by name; `--gateway-profile` loads from a file path.
3. **Parse agent config** -- read `agents/<name>.yaml` (default: `default`). `--agent-profile` (`-f`) overrides with a direct file path.
4. **Ensure providers** -- auto-register missing providers. Three registration flows:
   - **Standard** (`--from-existing`): GitHub, Atlassian -- OpenShell discovers credentials from local env.
   - **ADC** (`--from-gcloud-adc`): Vertex AI -- reads ADC file, configures inference routing.
   - **Custom**: GWS -- multi-step OAuth refresh flow (harness workaround until OpenShell adds native support).
5. **Render payload** -- `run.sh` (entrypoint wrapper with PATH setup, git auth, `-p` task), `task.md` (if set).
6. **Create sandbox** -- `openshell sandbox create` with `--env` (env vars), `--upload` (payload), and startup command. Retry up to 5 times.

`--provider-refresh` deletes and recreates all providers (replaces the old `harness providers --force`).

### `harness create [--agent NAME] [--agent-profile|-f FILE] [--name SANDBOX]`

Create a sandbox without deploying the gateway. Assumes gateway is running. Auto-registers missing providers.

### `harness deploy [local|ocp|kind]`

Deploy or verify the gateway for a target. Reads `profiles/gateways/<target>.yaml`.

### `harness status`

Show sandbox status. Read-only.

### `harness stop [NAME]` / `harness start [NAME]`

Stop or start a sandbox without deleting it.

### `harness teardown [--sandboxes] [--providers] [--k8s]`

Tear down resources. At least one flag required.

## Config Files

| File | Purpose |
|------|---------|
| `agents/*.yaml` | Agent config: image, entrypoint, providers, env, task |
| `profiles/providers/` | OpenShell provider profile YAMLs |
| `profiles/gateways/*.yaml` | Gateway profiles: deployment target config with inline Helm values |
| `sandbox/Dockerfile` | Sandbox image: OpenShell base + MCP servers + CLI tools |
| `sandbox/policy.yaml` | Network egress rules applied to sandboxes |
| `sandbox/opencode.json` | MCP server config for OpenCode agent |

## Image Tags

All images are published to `ghcr.io/robbycochran/harness-openshell`. CI never publishes floating tags (`:latest`, `:sandbox`); the bare `:sandbox` fallback below exists only for local `go build` binaries without version ldflags.

| Trigger | Sandbox |
|---------|---------|
| Release `v0.1.2` | `:sandbox-v0.1.2` |
| Any push/PR | `:sandbox-<sha>` |

The CLI resolves images from its embedded version (set via `-ldflags` at build time):

- `v0.1.2` → `:sandbox-v0.1.2` (tagged release)
- `v0.1.2-5-gabc1234` → `:sandbox-v0.1.2-5-gabc1234` (dev build, matches `make dev-sandbox`)
- `dev` → `:sandbox` (bare `go build` without ldflags)

`SANDBOX_IMAGE` env var overrides the version-based resolution.

## Environment Variables

| Variable | Purpose |
|----------|---------|
| `SANDBOX_IMAGE` | Override sandbox image (dev/CI builds) |
| `HARNESS_DIR` | Override harness directory detection |
| `OPENSHELL_NAMESPACE` | Override K8s namespace (default: `openshell`) |
| `OPENSHELL_CLI` | Override openshell binary path |
| `OPENSHELL_MODEL` | Inference model for provider registration (default: `claude-sonnet-4-6`) |
| `OPENSHELL_CHART_VERSION` | Override Helm chart version (beats `gateway.yaml`) |
| `PULL_SECRET` / `SANDBOX_PULL_SECRET` | Image pull secret names passed to the Helm install |
| `KUBECONFIG` | K8s cluster config for remote targets |

`GATEWAY_NAME` is internal -- used by env override in gateway config, not typically set by users.

## Payload

The harness renders agent config into a self-contained payload uploaded to `/sandbox/.config/openshell/`:

```
openshell/
  run.sh          -- validates entrypoint, execs it (with -p task if set)
  task.md         -- task file with envsubst applied (if task: is set)
  bin/            -- wrapper scripts
```

Environment variables are injected directly via `--env KEY=VALUE` flags on `openshell sandbox create` -- no file upload needed for env vars. `run.sh` is the entrypoint for interactive mode.
