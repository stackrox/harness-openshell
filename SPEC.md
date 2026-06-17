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

Agent configs live in `profiles/agent-<name>.yaml`. Each declares the sandbox image, entrypoint, providers, and environment:

```yaml
name: agent
entrypoint: claude      # or: opencode
tty: true

providers:
  - profile: github
  - profile: google-vertex-ai
  - profile: atlassian
    env:
      JIRA_URL: ${JIRA_URL}
      JIRA_USERNAME: ${JIRA_USERNAME}
  - profile: google-workspace

env:
  ANTHROPIC_BASE_URL: https://inference.local
```

Fields:
- `name` (required) -- sandbox name, used for `openshell sandbox connect`
- `image` -- container image for the sandbox (default: version-matched from ghcr.io, override with `HARNESS_OS_IMAGE` env)
- `entrypoint` -- command to run (default: `claude`). Supports `claude`, `opencode`, `bash`, or any binary on PATH.
- `tty` -- enable TTY (default: true)
- `task` -- path to a task.md file, passed to entrypoint via `-p "$(cat task.md)"`
- `providers` -- list of provider profile references
- `providers[].profile` -- OpenShell provider profile name
- `providers[].env` -- non-secret env vars for this provider (resolved via `os.ExpandEnv`; empty values read from host env; injected via `--env` on sandbox create)
- `env` -- additional environment variables injected via `--env` on sandbox create (empty values read from host env)
- `include` -- extra files to include in the payload
- `policy` -- path to a network policy YAML
- `gateway` -- target gateway name (overrides active gateway)

Provider profiles live in `profiles/providers/`. These are imported to the gateway during provider registration.

### Multi-document harness YAML

Agent configs support multi-document YAML (`---` separated) where provider, gateway, and policy definitions are co-located in one file:

```yaml
---
kind: agent
name: my-agent
entrypoint: claude
providers:
  - profile: github
---
kind: provider
name: github
type: github
credentials: [GITHUB_TOKEN]
---
kind: gateway
name: local
type: local
```

Documents are dispatched by `kind` field. No `kind` field = agent (backwards compatible). Definitions in the harness file take priority over the `profiles/` tree.

## CLI

### `harness apply [-f FILE] [--agent NAME] [--gateway NAME] [--gateway-profile FILE] [--name SANDBOX] [--attach] [--provider-refresh] [--dry-run] [-o yaml|json]`

Primary command. Resolves an agent config, deploys the gateway and providers, creates a sandbox.

1. **Parse agent config** -- resolve `agent-<name>.yaml` from harness directory (default: `default`). `-f` overrides with a direct file path. Falls back to embedded `agent-basic.yaml` when `agent-default.yaml` is not found on disk.
2. **Check output mode** -- if `-o yaml` or `-o json`, render the fully resolved config and exit. No gateway interaction needed.
3. **Check version** -- warn if openshell CLI is below v0.0.59.
4. **Resolve gateway** -- `--gateway` selects a profile by name; `--gateway-profile` loads from a file path. Default: `local`. `OPENSHELL_GATEWAY` env var is used as fallback.
5. **Dry-run check** -- if `--dry-run`, validate each step (gateway reachable, providers resolvable, env vars resolved, image available) and exit with pass/fail report.
6. **Ensure gateway** -- deploy if needed (local: Podman, remote: Helm to K8s/OCP).
7. **Ensure providers** -- auto-register missing providers. Three registration flows:
   - **Standard** (`--from-existing`): GitHub, Atlassian -- OpenShell discovers credentials from local env.
   - **ADC** (`--from-gcloud-adc`): Vertex AI -- reads ADC file, configures inference routing.
   - **Custom**: GWS -- multi-step OAuth refresh flow.
8. **Render payload** -- `run.sh` (entrypoint wrapper with PATH setup, entrypoint validation, `-p` task), `task.md` (if set).
9. **Create sandbox** -- `openshell sandbox create` with `--env` (env vars), `--upload` (payload), and startup command. Retry up to 5 times.

Default is non-interactive (headless). Use `--attach` for TTY mode.

`--provider-refresh` deletes and recreates all providers.

### `harness get <resource> [-o table|json|yaml]`

List resources. Wraps `openshell` list commands with consistent structured output across resource types. `-o table` is the default. Credential values are never included in structured output.

| Subcommand | Aliases | What it lists |
|------------|---------|--------------|
| `get agents` | `sandboxes`, `sandbox` | Running sandboxes (name, phase) |
| `get providers` | `provider` | Registered providers (name only, no credentials) |
| `get gateways` | `gateway`, `gw` | Gateways (name, endpoint, active) |

These are convenience wrappers. For full details, use `openshell sandbox list`, `openshell provider list`, etc. directly.

### `harness describe <name>`

Show detailed status for a specific sandbox: phase, active gateway, and registered providers.

### `harness delete [NAME...] [--all] [--providers] [--k8s]`

Delete sandboxes by name, or use flags for bulk operations. `--all` deletes sandboxes, providers, and k8s resources. Reuses the same teardown functions as the old `teardown` command.

### `harness deploy [local|ocp|kind]`

Deploy or verify the gateway for a target. Reads `profiles/gateways/<target>.yaml`.

### Deprecated Aliases

These commands still work but will be removed in a future release:

| Old command | Replacement | Notes |
|-------------|-------------|-------|
| `harness teardown` | `harness delete` | Same flags: `--sandboxes`, `--providers`, `--k8s` |
| `harness status` | `harness get agents` | |

## Config Files

| File | Purpose |
|------|---------|
| `profiles/agent-*.yaml` | Agent config: image, entrypoint, providers, env, task |
| `profiles/providers/` | OpenShell provider profile YAMLs |
| `profiles/gateways/*.yaml` | Gateway profiles: deployment target config with inline Helm values |
| `profiles/images/sandbox-default/Dockerfile` | Sandbox image: OpenShell base + MCP servers + CLI tools |
| `profiles/images/sandbox-default/CLAUDE.md` | Claude Code project instructions for sandbox |
| `profiles/images/sandbox-default/claude.json` | Claude Code settings |
| `profiles/images/sandbox-default/mcp.json` | MCP server config for Claude agent |
| `profiles/images/sandbox-default/opencode.json` | MCP server config for OpenCode agent |
| `profiles/images/sandbox-default/policy.yaml` | Network egress rules applied to sandboxes |
| `profiles/images/sandbox-default/settings.json` | Claude Code settings overlay |

## Image Tags

All images are published to `ghcr.io/robbycochran/harness-openshell`. CI never publishes floating tags (`:latest`, `:sandbox`); the bare `:sandbox` fallback below exists only for local `go build` binaries without version ldflags.

| Trigger | Sandbox |
|---------|---------|
| Release `v0.1.2` | `:sandbox-v0.1.2` |
| Any push/PR | `:sandbox-<sha>` |

The CLI resolves images from its embedded version (set via `-ldflags` at build time):

- `v0.1.2` -> `:sandbox-v0.1.2` (tagged release)
- `v0.1.2-5-gabc1234` -> `:sandbox-v0.1.2-5-gabc1234` (dev build, matches `make dev-sandbox`)
- `dev` -> `:sandbox` (bare `go build` without ldflags)

`HARNESS_OS_IMAGE` env var overrides the version-based resolution.

## Environment Variables

Harness-specific variables use the `HARNESS_OS_` prefix. OpenShell runtime variables use `OPENSHELL_`.

| Variable | Purpose |
|----------|---------|
| `HARNESS_OS_DIR` | Override harness directory detection |
| `HARNESS_OS_IMAGE` | Override sandbox image (dev/CI builds) |
| `HARNESS_OS_PULL_SECRET` | Image pull secret name passed to Helm install |
| `HARNESS_OS_SANDBOX_PULL_SECRET` | Sandbox image pull secret name passed to Helm install |
| `OPENSHELL_CLI` | Override openshell binary path |
| `OPENSHELL_GATEWAY` | Override gateway name (used by apply, plugin-compatible) |
| `OPENSHELL_NAMESPACE` | Override K8s namespace (default: `openshell`) |
| `OPENSHELL_MODEL` | Inference model for provider registration (default: `claude-sonnet-4-6`) |
| `OPENSHELL_CHART_VERSION` | Override Helm chart version (beats `gateway.yaml`) |
| `KUBECONFIG` | K8s cluster config for remote targets |

## Payload

The harness renders agent config into a self-contained payload uploaded to `/sandbox/.config/openshell/`:

```
openshell/
  run.sh          -- validates entrypoint, execs it (with -p task if set)
  task.md         -- task file with envsubst applied (if task: is set)
  bin/            -- wrapper scripts
```

Environment variables are injected directly via `--env KEY=VALUE` flags on `openshell sandbox create` -- no file upload needed for env vars. `run.sh` is the entrypoint for interactive mode.
