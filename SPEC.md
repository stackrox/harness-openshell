# OpenShell Harness Specification

This document specifies the behavior of the OpenShell Harness independently of its implementation. Any conforming implementation (bash, Go, Rust, Python) should produce the same observable behavior.

## Overview

The harness deploys and manages AI agent sandboxes on two platforms:
- **Local** — Podman containers via a local OpenShell gateway
- **Remote** — Kubernetes pods via an OpenShift-hosted OpenShell gateway

Each sandbox is an isolated container with a Claude Code agent, credential providers, MCP servers, network policies, and uploaded configuration.

---

## CLI

The harness exposes a single entry point (`harness`) with subcommands.

### `harness new [--local|--remote] [--profile NAME] [--name SANDBOX_NAME] [--no-tty]`

Create a new sandbox. This is the primary command. It performs these steps in order:

1. **Ensure gateway** — if `--local`, verify the local podman gateway is running. If `--remote`, deploy to OpenShift (Helm chart, CRDs, SCCs, route, mTLS certs). If neither, check for an active gateway.
2. **Ensure providers** — if no providers are registered on the gateway, run provider registration.
3. **Ensure credentials** (remote only) — if K8s secrets for GWS/Atlassian don't exist, create them.
4. **Parse profile** — read `profiles/<name>.toml` (default: `default`).
5. **Stage files** — write `sandbox.env` from profile `[env]`, export GWS credentials.
6. **Create sandbox** — call `openshell sandbox create` with `--from` (image), `--provider` (each provider), `--upload` (staged files), and the startup command. Retry up to 5 times for supervisor race conditions.

If `--no-tty` is passed, the sandbox runs `startup.sh` and exits (for testing). Otherwise, it runs `startup.sh` then execs into the configured command (e.g., `claude --bare`).

If `--name` is not provided, the sandbox name comes from the profile's `name` field.

### `harness connect [SANDBOX_NAME]`

Reconnect to a running sandbox via `openshell sandbox connect`.

### `harness deploy --local|--remote [--kubeconfig PATH]`

Deploy or verify the gateway without creating a sandbox. Requires `--local` or `--remote`.

**Local:** Check podman installed, find a gateway with endpoint `127.0.0.1`, select it, verify it responds.

**Remote:** Create namespace, install CRDs, grant SCCs, deploy Helm chart from OCI registry (`oci://ghcr.io/nvidia/openshell/helm-chart`), create TLS passthrough route, register CLI gateway with mTLS certs from the cluster. `--kubeconfig` sets the kubeconfig path (or set `KUBECONFIG` env var).

### `harness teardown [--sandboxes] [--providers] [--k8s]`

Tear down resources. Default (no flags) tears down everything applicable.

- `--sandboxes` — delete all sandboxes on the active gateway
- `--providers` — delete all providers and inference config (requires no running sandboxes)
- `--k8s` — Helm uninstall, delete CRDs, SCCs, secrets, namespace, and gateway config

### `harness preflight [--strict]`

Read-only environment check. Validates all inputs defined in `providers.toml` for each enabled provider in `openshell.toml`. Reports per-input status with `✓`/`✗` prefixes.

With `--strict`, exits non-zero if any `required` provider has missing inputs.

Subcommands:
- `harness preflight available` — print space-separated names of openshell-type providers where all inputs pass
- `harness preflight names` — print space-separated names of all enabled openshell-type providers

### `harness providers [--force]`

Register credential providers with the gateway:

1. Enables providers v2 via `openshell settings set`
2. Imports custom provider profiles from `sandbox/profiles/`
3. Registers each provider (github, vertex-local, atlassian) if the required env vars are set:
   - `github` — requires `GITHUB_TOKEN`
   - `vertex-local` — requires ADC file + project ID (`ANTHROPIC_VERTEX_PROJECT_ID` or fallback from ADC's `quota_project_id`). Sets inference model from `OPENSHELL_MODEL` (default: `claude-sonnet-4-6`).
   - `atlassian` — requires `JIRA_API_TOKEN`
4. Skips providers that already exist

With `--force`: deletes existing providers and custom profiles before recreating. Requires no running sandboxes.

### `harness test [podman|ocp|all] [--full]`

End-to-end validation. Quick mode: deploy → providers → gateway check → teardown. Full mode adds: sandbox create → verify env vars, GWS creds, MCP config, Claude responds → sandbox delete → teardown.

---

## Configuration

### `providers.toml`

Catalog of provider definitions. Each `[[providers]]` entry has:

| Field | Required | Description |
|-------|----------|-------------|
| `name` | yes | Unique identifier |
| `type` | yes | `"openshell"` or `"custom"` |
| `description` | yes | Shown in preflight |
| `required` | no | If true, `--strict` preflight fails when inputs missing |
| `method` | no | Registration method (e.g., `"from-gcloud-adc"`) |
| `upstream` | no | Link to upstream issue (custom providers) |

Each provider has an `inputs` array of inline tables:

| Field | Required | Description |
|-------|----------|-------------|
| `key` | yes | Env var name, file path, or shell command |
| `kind` | yes | `"env"`, `"file"`, or `"check"` |
| `secret` | no | Mask value in preflight output |

### `openshell.toml`

Deployment configuration:

```toml
providers = ["github", "vertex-local", "atlassian"]
providers-custom = ["gws"]

[inference]
model = "claude-sonnet-4-6"

[upstream]
chart-version = "0.0.55"
```

### `profiles/<name>.toml`

Per-sandbox configuration:

```toml
name = "agent"
image = "quay.io/rcochran/openshell:sandbox"
command = "claude --bare"
keep = true
providers = ["github", "vertex-local", "atlassian"]

[env]
ANTHROPIC_BASE_URL = "https://inference.local"
ANTHROPIC_API_KEY = "sk-ant-openshell-proxy-managed"
JIRA_URL = "https://mysite.atlassian.net"
JIRA_USERNAME = "user@example.com"
```

| Field | Default | Description |
|-------|---------|-------------|
| `name` | `"agent"` | Sandbox name (overridden by `--name`) |
| `image` | none | Container image for the sandbox |
| `command` | `"claude --bare"` | Command to exec after startup |
| `keep` | `true` | Keep sandbox alive after command exits |
| `providers` | `[]` | Provider names to attach |
| `[env]` | `{}` | Environment variables injected into the sandbox |

---

## Sandbox Lifecycle

### Creation

1. Profile parsed → `SANDBOX_IMAGE`, `SANDBOX_COMMAND`, `SANDBOX_PROVIDERS`, `SANDBOX_ENV`
2. Files staged to `/tmp/openshell/`:
   - `sandbox.env` — export statements from `[env]` section
   - `credentials.json` — GWS OAuth credentials (if available)
   - `client_secret.json` — GWS OAuth client config (if available)
3. `openshell sandbox create` called with:
   - `--from <image>` — sandbox container image
   - `--provider <name>` — for each provider
   - `--upload /tmp/openshell:/sandbox/.config` — files land at `/sandbox/.config/openshell/`
   - `-- bash -c '. /sandbox/startup.sh && exec <command>'` (tty mode)
   - `-- bash /sandbox/startup.sh` (no-tty mode)
4. On failure (supervisor race), delete sandbox and retry (up to 5 times, 5s between for local, 10s for OCP launcher)

### Startup (inside sandbox)

`startup.sh` runs once at creation:
1. Source `/sandbox/.config/openshell/sandbox.env` → append to `.bashrc`
2. Run `gh auth setup-git`

### Connection

`openshell sandbox connect <name>` opens an interactive session. Environment variables from `.bashrc` are inherited by Claude Code and its MCP servers.

### Deletion

`openshell sandbox delete <name>` or `harness teardown --sandboxes`.

---

## Credential Flow

| Credential | Provider Type | How It Works |
|------------|--------------|--------------|
| GitHub | `github` | PAT stored in gateway, proxy-managed. Sandbox sees `GITHUB_TOKEN` placeholder. |
| Vertex AI | `google-vertex-ai` | ADC-based OAuth via `--from-gcloud-adc`. Gateway refreshes tokens automatically. Inference routed through `inference.local`. |
| Atlassian | `atlassian` | API token stored in gateway. Proxy resolves base64 Basic auth header. `JIRA_URL`/`JIRA_USERNAME` injected via `sandbox.env`. |
| GWS | Custom (file upload) | Decrypted OAuth credentials uploaded to `/sandbox/.config/openshell/`. Not proxy-managed. |

---

## Sandbox Image

The sandbox image extends `ghcr.io/nvidia/openshell-community/sandboxes/base:latest` with:
- `mcp-atlassian` — Jira/Confluence MCP server
- `gws` CLI — Google Workspace
- `policy.yaml` — network egress rules
- `CLAUDE.md` — agent instructions
- `settings.json` — Claude Code permissions (`defaultMode: bypassPermissions`)
- `.mcp.json` — MCP server configuration (auto-loaded by Claude Code)
- `startup.sh` — runtime env setup

The image is multi-arch (`linux/amd64` + `linux/arm64`), built with `docker buildx`.

---

## Network Policy

`sandbox/policy.yaml` controls which processes can reach which hosts:

| Policy | Hosts | Binaries |
|--------|-------|----------|
| `claude_telemetry` | `*.anthropic.com`, `downloads.claude.ai`, `platform.claude.com`, `sentry.io` | claude, node |
| `github_git` | `github.com` (GET info/refs, POST git-upload-pack only) | git |
| `github_downloads` | `*.githubusercontent.com`, `codeload.github.com` | curl, gh, git, uv |
| `google_workspace` | `*.googleapis.com`, `oauth2.googleapis.com` | gws |
| `pypi` | `pypi.org`, `files.pythonhosted.org` | python, pip, uv |
| `npm` | `registry.npmjs.org` | npm, node |

Git push (`git-receive-pack`) is blocked by default.

---

## OCP-Specific

### Gateway Deployment

- Helm chart from `oci://ghcr.io/nvidia/openshell/helm-chart` (version pinned in `openshell.toml`)
- TLS passthrough route at `gateway-openshell.<apps-domain>`
- mTLS: client certs copied from `openshell-client-tls` K8s secret to `~/.config/openshell/gateways/<name>/mtls/`
- `allowUnauthenticatedUsers: true` (mTLS is the auth layer)

### In-Cluster Launcher

For OCP sandboxes, a Kubernetes Job runs the launcher image (`sandbox/launcher/`):
1. Register gateway via `http://` trick (avoids cert generation probe), patch to `https://` + mTLS
2. Parse profile from mounted ConfigMap
3. Build provider flags, create sandbox with retry
4. Upload files (GWS creds, sandbox.env)
5. Run startup.sh via `sandbox exec`

The launcher connects to the gateway at `https://openshell.openshell.svc.cluster.local:8080` using mounted mTLS certs from `openshell-client-tls`.

---

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `OPENSHELL_CLI` | `openshell` | Path or name of the openshell CLI binary |
| `OPENSHELL_MODEL` | `claude-sonnet-4-6` | Inference model for `harness providers` |
| `HARNESS_DIR` | auto-detected | Root directory of the harness project |
| `OPENSHELL_NAMESPACE` | `openshell` | Kubernetes namespace for OCP deployments |

---

## Testing

### Bats Tests (`test/preflight.bats`)

29 bats tests covering the preflight check engine. Runs against both the Python (`lib/providers.py`) and Go (`harness preflight`) implementations via `USE_GO=true`. Uses stubbed CLI, isolated temp dirs, and no gateway dependency. Tests:
- env inputs (set/missing/secret/masked)
- file inputs (exists/missing/metadata extraction)
- check inputs (pass/fail/env expansion)
- provider status (all pass/any fail/required/optional)
- config filtering (enabled/disabled/no config)
- CLI detection (present/missing)
- Gateway detection (podman/k8s)

### Go Unit Tests

- `internal/gateway/cli_test.go` — stub-based tests for CLI output parsing and argument building
- `internal/profile/profile_test.go` — TOML parsing, env generation, provider validation with mock gateway
- `cmd/new_test.go` — orchestration tests: no gateway, missing providers, retry logic, create opts
- `sandbox/launcher/main_test.go` — launcher config parsing and file staging

### Integration Tests (`test/test-flow.sh`)

End-to-end validation requiring a live gateway:
- Quick mode: deploy → providers → gateway check → teardown
- Full mode: + sandbox create → verify env/GWS/MCP/Claude → delete → teardown
- Error scenarios: bad profile, teardown idempotency, missing providers
- Targets: `podman`, `ocp`, `all`

Flags:
- `--go` — run the Go binary instead of bash scripts
- `--full` — include sandbox lifecycle tests
- `--reuse-gateway` — skip helm deploy/teardown-k8s, reuse existing gateway (49s vs 137s for OCP)

### Test Matrix (`make validate`)

Full validation across all paths. Run before every commit:
1. Go unit tests (harness + launcher)
2. Bats preflight (Python + Go paths)
3. Integration: `{bash, go}` × `{podman, ocp}`
