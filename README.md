# OpenShell Harness

An orchestration layer for [OpenShell](https://github.com/NVIDIA/OpenShell) that manages gateway deployment, provider registration, credential validation, and sandbox configuration. One command gets you from zero to a running AI agent sandbox on local Podman or remote OpenShift.

## Relationship to OpenShell

The harness wraps `openshell` — it doesn't replace it. Every operation delegates to the OpenShell CLI via `exec.Command`. Users can drop to raw `openshell` commands at any time.

The harness exists to bridge gaps in OpenShell's current workflow:

- **Gateway deployment** — OpenShell provides the gateway binary and Helm chart but leaves orchestration to the user (namespace setup, CRDs, SCCs on OpenShift, mTLS cert extraction, Helm values). The harness automates this via config-driven gateway definitions (`gateways/local/`, `gateways/ocp/`, `gateways/kind/`).

- **Provider lifecycle** — OpenShell manages credentials once registered, but doesn't validate prerequisites or discover credentials from local tooling. The harness adds preflight checks (env vars, files, connectivity probes) and profile-driven provider selection.

- **Credential validation** — preflight checks verify credentials are present and valid on the host before registration. In-sandbox verification (confirming providers work end-to-end from inside a running sandbox) is planned but not yet implemented.

- **Parity across targets** — a sandbox created locally via Podman should behave identically to one on OpenShift. The harness enforces this by using the same profiles, provider catalog, and validation on both.

As OpenShell matures, the harness should shrink. Every workaround tracks the upstream issue that would eliminate it (see [AGENTS.md](AGENTS.md)).

## How It Compares

| Concern | OpenShell Harness | [Kaiden](https://github.com/openkaiden/kaiden) | [Plandex](https://github.com/plandex-ai/plandex) |
|---------|-------------------|--------|---------|
| **Primary focus** | Gateway deploy + provider orchestration | GUI workspace management, resource selection | Plan-driven coding agent |
| **Sandbox runtime** | OpenShell (delegates entirely) | OpenShell (migrating to it) | None (runs locally) |
| **Entry point** | Container image (image-first) | Local folder or git URL (source-first) | Local directory |
| **Provider management** | Preflight validation + registration | References by name (delegates to OpenShell) | N/A |
| **Target environments** | Local Podman + remote K8s/OCP | Local only (desktop app) | Local only |
| **Credential isolation** | Proxy-resolved placeholders, sandbox never sees tokens | Delegates to OpenShell | None |
| **Configuration** | TOML profiles + provider catalog | JSON projects (GUI-driven) | YAML plans |

The harness operates at the infrastructure layer — deploying gateways, registering providers, validating credentials. Kaiden operates at the workspace layer — selecting which skills, MCP servers, and knowledge bases a workspace gets. They are complementary, not competing. See [profile.md](profile.md) for a detailed analysis.

## Goals

1. **One command to working sandbox.** `harness new` chains gateway deployment, provider registration, and sandbox creation into a single invocation.

2. **Reproducible environments.** Profiles define exactly what a sandbox needs. The same profile produces the same environment regardless of who runs it or where.

3. **Credential visibility.** Preflight checks validate credentials locally before registration — env vars set, files present, connectivity confirmed. You know what's broken before you try to create a sandbox.

4. **Clean separation of concerns.** Infrastructure config, provider management, and sandbox profiles are independent. Changing your Jira token doesn't require editing sandbox profiles. Switching clusters doesn't require re-registering providers.

5. **Thin wrapper, not a platform.** Orchestration and validation on top of OpenShell. No reimplementation of sandbox runtime, network policy, or credential injection.

6. **Image-first, no host mounts.** Sandboxes boot from container images with tools baked in. Files are uploaded, not bind-mounted. Host-mounted workflows break parity between local and remote targets, bypass credential isolation, and create implicit dependencies on the host filesystem. If it doesn't work on OpenShift, it shouldn't work locally either. The sandbox interacts with the outside world through providers — git commits, PR reviews, Jira comments, email — not by writing files that get pulled back to the host.

## Three Domains

| Domain | Question | Config | Commands |
|--------|----------|--------|----------|
| **Infrastructure** | How is the gateway deployed? | `gateways/<name>/gateway.toml` | `deploy`, `teardown --k8s` |
| **Providers** | What credentials are available and valid? | `providers.toml` | `providers`, `preflight` |
| **Sandbox** | What sandbox do I want? | `profiles/*.toml` | `new`, `connect` |

Each domain has its own config, its own code boundary, and its own concerns. A sandbox profile says what providers a sandbox *wants*. The provider catalog says what *exists* and how to validate it. The infrastructure layer handles *where* it all runs.

## Quick Start

```bash
# Install OpenShell CLI
curl -LsSf https://raw.githubusercontent.com/NVIDIA/OpenShell/main/install.sh | sh

# Authenticate with Google Cloud (Vertex AI inference)
gcloud auth application-default login --project your-gcp-project-id

# Set credentials
export GITHUB_TOKEN="ghp_..."
export JIRA_API_TOKEN="..."
export JIRA_URL="https://mysite.atlassian.net"
export JIRA_USERNAME="you@example.com"

# Build the harness
make cli

# Local — deploy gateway, register providers, create sandbox
harness new --local

# Remote — same flow on OpenShift
harness new --remote

# Reconnect to a running sandbox
harness connect
```

Podman is required for local sandboxes. kubectl + helm are required for remote.

## Commands

```
harness new [--local|--remote] [--profile NAME]
    Full flow: deploy gateway + register providers + create sandbox.

harness connect [NAME]
    Reconnect to a running sandbox.

harness deploy [GATEWAY_NAME]
    Deploy or verify a gateway by name (local, ocp, kind).

harness providers [--force]
    Register providers with the gateway.

harness preflight [--strict]
    Validate local environment prerequisites.

harness teardown [--sandboxes] [--providers] [--k8s]
    Tear down resources. At least one flag required.
```

## Profiles

Sandboxes are configured via TOML profiles. A profile defines the sandbox shape — image, command, which providers to attach, and environment variables.

```toml
# profiles/default.toml
name = "agent"
from = "ghcr.io/robbycochran/harness-openshell:sandbox"
command = "claude --bare"
providers = ["github", "vertex-local", "atlassian"]

[env]
ANTHROPIC_BASE_URL = "https://inference.local"
```

Provider credentials and provider-specific config are provider concerns, not profile concerns. The profile just lists which providers the sandbox wants.

Use a specific profile: `harness new --profile research`

## Provider Catalog

`providers.toml` defines available providers and how to validate them:

```toml
[[providers]]
name = "atlassian"
type = "openshell"
description = "Jira and Confluence (Basic auth resolved by proxy)"
inputs = [
  { key = "JIRA_API_TOKEN", kind = "env", secret = true },
  { key = "JIRA_URL", kind = "env", sandbox = true },
  { key = "JIRA_USERNAME", kind = "env", sandbox = true },
]
```

Providers with `type = "openshell"` are registered with the gateway and managed by OpenShell's credential proxy. Providers with `type = "custom"` are workarounds for integrations OpenShell doesn't natively support yet — each tracks its upstream issue. See [PROVIDERS-SPEC.md](PROVIDERS-SPEC.md) for the full schema.

## Architecture

```
Local (Podman)                    Remote (OpenShift)
┌──────────┐                    ┌──────────────────────────────┐
│ harness  │  openshell CLI     │ Gateway (StatefulSet)         │
│ CLI      ├───────────────────►│   ├─ OpenShell API             │
│          │  localhost:17670   │   ├─ inference.local proxy     │
└──────────┘                    │   ├─ Provider credential store │
                                │   └─ OAuth token refresh       │
       or                       │                                │
                                │ Sandbox Pods                   │
┌──────────┐  Route (mTLS)      │   ├─ Claude Code → Vertex AI   │
│ harness  ├───────────────────►│   ├─ mcp-atlassian             │
│ CLI      │  OCP :443          │   ├─ gws CLI                   │
└──────────┘                    │   ├─ gh CLI                    │
                                │   └─ L7 network proxy          │
                                └──────────────────────────────┘
```

The harness talks to the gateway via the `openshell` CLI (exec). Direct gRPC is a planned future improvement — the Gateway interface already abstracts the transport.

Each sandbox gets credential isolation (proxy-resolved placeholders, the sandbox never sees real tokens), per-binary network policy enforcement, and a reproducible toolchain pinned in the container image.

## Project Layout

| Path | Purpose |
|------|---------|
| `main.go`, `cmd/` | CLI commands (Go) |
| `internal/gateway/` | OpenShell CLI wrapper (Gateway interface) |
| `internal/k8s/` | kubectl/helm/oc runner with retry and transient error handling |
| `internal/profile/` | Profile TOML parsing and staging |
| `internal/preflight/` | Provider prerequisite validation |
| `profiles/` | Sandbox profiles (TOML) |
| `providers.toml` | Provider catalog (inputs, prerequisites, upstream tracking) |
| `gateways/` | Per-target gateway configs (`local/`, `ocp/`, `kind/`) |
| `sandbox/` | Sandbox container image (Dockerfile, startup, policy, agent instructions) |
| `sandbox/launcher/` | In-cluster launcher for OCP sandboxes |
| `sandbox/profiles/` | OpenShell provider type profiles (YAML, imported into gateway) |
| `test/` | Tests (bats preflight, test-flow.sh integration) |

## Future Direction

- **In-sandbox provider verification** — validate that providers actually work from inside a running sandbox, not just that credentials are present on the host. Catches expired tokens, proxy misconfig, and endpoint reachability issues.
- **Proto-based profiles** — align profile schema with OpenShell's proto types (`SandboxSpec`, `ProviderProfile`) for compile-time upstream compatibility.
- **Direct gRPC** — replace CLI exec with gRPC calls to the gateway, eliminating output parsing. The Gateway interface already abstracts this swap.
- **Shrink** — as OpenShell adds native support for GWS credentials, provider config injection, and in-cluster sandbox creation, remove the corresponding harness workarounds.

## Testing

```bash
make validate-dev        # build dev images + run integration tests
go test ./...            # Go unit tests
bats test/preflight.bats # 29 preflight unit tests
```

## Contributing

See [AGENTS.md](AGENTS.md) for coding guidelines, project principles, and workaround tracking.
