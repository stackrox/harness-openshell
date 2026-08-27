# OpenShell Harness Specification

Behavior specification for the OpenShell Harness CLI.

## Overview

The harness declares providers, inference routing, and policy, then creates and
runs AI agent sandboxes against a gateway **OpenShell has already provisioned**.
Provisioning a gateway (local installer, or `helm install openshell` on a
cluster) is OpenShell's job — the harness has zero compute-backend opinion and
never deploys or tears down a gateway. It runs against whichever gateway is
selected (`openshell gateway select`), local or cluster, from the same YAML.

Each sandbox is an isolated container running an agent entrypoint (e.g. Claude Code, OpenCode, or Codex; `bash` or any binary on PATH also works), with credential providers, network policies, and a rendered payload (`task.md` and a `bin/` directory).

Requires OpenShell v0.0.110+.

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
      JIRA_URL:
      JIRA_USERNAME:
  - profile: google-workspace

env:
  ANTHROPIC_BASE_URL: https://inference.local
```

Fields:
- `name` (required) -- sandbox name, used for `openshell sandbox connect`
- `base_agent` -- name of a base agent config to inherit from (e.g., `default` resolves `agent-default.yaml`). Providers, env, and payloads are merged additively; scalar fields (entrypoint, repo, task, image, policy) from the overlay win when non-empty.
- `image` -- container image for the sandbox (default: version-matched from `quay.io/rcochran/openshell`, override with `HARNESS_OS_IMAGE` env)
- `entrypoint` -- command to run (default: `claude`). Supports `claude`, `codex`, `opencode`, `bash`, or any binary on PATH.
- `tty` -- enable TTY (default: true)
- `repo` -- git URL to clone outside the sandbox and upload to `/sandbox/<repo-name>`. Shallow clone (`--depth 1`) with submodules. Git credentials never enter the sandbox unless needed.
- `repo_ref` -- branch, tag, or ref to clone (default: HEAD). Passed as `--branch` to git clone.
- `task` -- path to a task.md file, passed to entrypoint via `-p "$(cat task.md)"`
- `providers` -- list of provider profile references
- `providers[].profile` -- OpenShell provider profile name
- `providers[].env` -- non-secret env vars for this provider (resolved via `os.ExpandEnv`; empty values read from host env; injected via `--env` on sandbox create)
- `env` -- additional environment variables injected via `--env` on sandbox create (empty values read from host env)
- `include` -- extra files to include in the payload
- `policy` -- path to a network policy YAML

The agent config names no gateway: `harness apply` runs against the gateway
OpenShell has provisioned and you have selected (`openshell gateway select`).

Provider profiles live in `profiles/providers/`. These are imported to the gateway during provider registration.

### Multi-document harness YAML

Agent configs support multi-document YAML (`---` separated) where provider and policy definitions are co-located in one file:

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
kind: policy
network_policies:
  github_git:
    endpoints:
      - host: github.com
        port: 443
```

Documents are dispatched by `kind` field. No `kind` field = agent (backwards compatible). Definitions in the harness file take priority over the `profiles/` tree.

## CLI

### `harness apply [-f FILE] [--agent NAME] [--name SANDBOX] [--task TEXT|@FILE] [--entrypoint CMD] [--attach] [--setup-only] [--dry-run] [-o yaml|json]`

Primary command. Resolves an agent config, reconciles providers and inference on the selected gateway, creates a sandbox. It never provisions a gateway — one must already be provisioned by OpenShell and selected.

1. **Parse agent config** -- resolve `agent-<name>.yaml` from harness directory (default: `default`). `-f` overrides with a direct file path. Falls back to embedded `agent-basic.yaml` when `agent-default.yaml` is not found on disk.
2. **Check output mode** -- if `-o yaml` or `-o json`, render the fully resolved config and exit. No gateway interaction needed.
3. **Check version** -- warn if openshell CLI is below v0.0.110.
4. **Require an active gateway** -- resolve the gateway from `openshell`'s active selection (`OPENSHELL_GATEWAY` env var overrides). Error up front if none is selected; `upLocal` additionally preflights that the gateway is reachable before touching providers or creating a sandbox.
5. **Dry-run check** -- if `--dry-run`, validate each step (gateway reachable, providers resolvable, env vars resolved, image available) and exit with pass/fail report.
6. **Ensure providers** -- auto-register missing providers. Three registration flows:
   - **Standard** (`--from-existing`): GitHub, Atlassian -- OpenShell discovers credentials from local env.
   - **ADC** (`--from-gcloud-adc`): Vertex AI -- reads ADC file, configures inference routing.
   - **Custom**: GWS -- multi-step OAuth refresh flow.
7. **Render payload** -- `task.md` (if set) and a `bin/` directory. The in-sandbox command is built by the agent adapter (`internal/agent/adapter.go`) as a `bash -lc` invocation (PATH setup, entrypoint validation via `command -v`), not a `run.sh` file. Task dispatch depends on mode and entrypoint: headless (default) uses `opencode run "$(cat task.md)"` for OpenCode and `--print "$(cat task.md)"` for claude/codex/custom entrypoints; interactive (`--attach`) uses `-p "$(cat task.md)"`.
8. **Create sandbox** -- `openshell sandbox create` with `--env` (env vars), `--upload` (payload), and startup command. Retry up to 5 times.

Default is non-interactive (headless). Use `--attach` for TTY mode.

`--setup-only` reconciles providers/inference on the selected gateway, then stops before creating a sandbox or running the agent.

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

### `harness delete [NAME...] [--all] [--providers]`

Delete sandboxes by name, or use flags for bulk operations. `--all` deletes sandboxes and providers. It never removes the gateway: cluster teardown is `helm uninstall openshell` plus `openshell gateway remove`.

### `harness init [-o FILE] [--force] [--non-interactive]`

Generate a `harness.yaml` config file. Interactive by default (prompts for entrypoint and providers); `--non-interactive` writes the embedded default. Writes to `harness.yaml` unless `-o` overrides the path. The generated config names no gateway — select one with `openshell gateway select`.

### `harness doctor [-f FILE] [--agent NAME] [--gateway NAME] [-o table|json|yaml]`

Validate the environment for a configured sandbox. Phase 1 (offline) checks the openshell binary and provider credentials without a running gateway; Phase 2 (online) checks provider registration when the gateway is reachable.

### `harness plan -f FILE [--gateway NAME] [-o table|json|yaml]`

Read-only reconciliation plan. Shows the actions `harness apply` would take without mutating anything. Distinct from `apply --dry-run`, which is a separate legacy path.

### `harness migrate -f FILE [-o FILE]`

Convert a legacy v1 harness config to the v1alpha1 format. The input YAML is normalized and written as v1alpha1 to stdout (or `-o FILE`). Fields with no v1alpha1 home (`task`, `include`, inline policy documents, unresolved `base_agent`) are reported as warnings on stderr. The legacy `gateway:` field named a deploy profile, a concept the harness no longer owns; it is dropped, leaving `spec.target.gateway` empty. The active gateway is chosen outside the config — with `openshell gateway select` or `$OPENSHELL_GATEWAY` — not by re-adding a field to the YAML.

## Config Files

| File | Purpose |
|------|---------|
| `profiles/agent-*.yaml` | Agent config: image, entrypoint, providers, env, task |
| `profiles/providers/` | OpenShell provider profile YAMLs |
| `profiles/images/sandbox-default/Dockerfile` | Sandbox image: OpenShell base + MCP servers + CLI tools |
| `profiles/images/sandbox-default/CLAUDE.md` | Claude Code project instructions for sandbox |
| `profiles/images/sandbox-default/claude.json` | Claude Code settings |
| `profiles/images/sandbox-default/mcp.json` | MCP server config for Claude agent |
| `profiles/images/sandbox-default/opencode.json` | MCP server config for OpenCode agent |
| `profiles/images/sandbox-default/policy.yaml` | Network egress rules applied to sandboxes |
| `profiles/images/sandbox-default/settings.json` | Claude Code settings overlay |

## Image Tags

All images are published to `quay.io/rcochran/openshell`. CI never publishes floating tags (`:latest`, `:sandbox`); the bare `:sandbox` fallback below exists only for local `go build` binaries without version ldflags.

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
| `OPENSHELL_CLI` | Override openshell binary path |
| `OPENSHELL_GATEWAY` | Override the active gateway name (used by apply, plugin-compatible) |
| `OPENSHELL_MODEL` | Inference model for provider registration (default: `claude-sonnet-4-6`) |

## Payload

The harness renders agent config into a self-contained payload uploaded to `/sandbox/.config/openshell/`:

```
openshell/
  task.md         -- task file with envsubst applied (if task: is set)
  bin/            -- payload binaries prepended to PATH
```

Environment variables are injected directly via `--env KEY=VALUE` flags on `openshell sandbox create` -- no file upload needed for env vars. The in-sandbox command (entrypoint validation and exec, with `-p`/`--print` task) is built by the agent adapter and wrapped in `bash -lc`; there is no `run.sh` file.
