# OpenShell Harness

A thin orchestration wrapper around [OpenShell](https://github.com/NVIDIA/OpenShell) for deploying and managing AI agent sandboxes. The harness handles the multi-step setup that OpenShell leaves to the user — gateway deployment, provider registration, credential validation, and sandbox configuration — so that one command gets you from zero to a running sandbox.

## Relationship to OpenShell

The harness wraps `openshell`, it doesn't replace it. Every operation delegates to the OpenShell CLI. Users can drop to raw `openshell` commands at any time.

The harness exists to bridge gaps in OpenShell's current workflow:

- **Gateway deployment** — OpenShell provides the gateway binary and Helm chart but not the orchestration to deploy it (namespace setup, CRDs, SCCs on OpenShift, mTLS cert extraction, Helm install with correct values). The harness automates this for both local (Podman) and remote (OpenShift) targets.

- **Provider lifecycle** — OpenShell manages credentials once registered, but doesn't validate prerequisites, discover credentials from local tooling, or compose provider profiles with sandbox configuration. The harness adds preflight checks, credential discovery, and profile-driven provider selection.

- **Parity across targets** — a sandbox created locally via Podman should behave identically to one on OpenShift. The harness provides this parity by using the same profiles, provider catalog, and validation on both targets.

As OpenShell matures, the harness should shrink. Every workaround tracks the upstream issue that would eliminate it (see [AGENTS.md](AGENTS.md)).

## Goals

1. **One command to working sandbox.** `harness new` takes a developer from zero to a running sandbox — gateway deployed, providers registered, credentials validated.

2. **Reproducible environments.** Profiles define exactly what a sandbox needs. The same profile produces the same environment regardless of who runs it or where.

3. **Credential visibility.** Preflight checks validate credentials locally before registration. Provider health checks verify they work end-to-end inside the sandbox.

4. **Clean separation of concerns.** Infrastructure config, provider management, and sandbox profiles are independent. Changing your Jira token doesn't require editing sandbox profiles. Switching clusters doesn't require re-registering providers.

5. **Thin wrapper, not a platform.** Orchestration and validation on top of OpenShell. No reimplementation of sandbox runtime, network policy, or credential injection.

6. **Image-first, no host mounts.** Sandboxes boot from container images with tools baked in. Files are uploaded, not bind-mounted. This is deliberate — host-mounted workflows break parity between local and remote targets, bypass credential isolation, and create implicit dependencies on the host filesystem. If it doesn't work on OpenShift, it shouldn't work locally either. The sandbox interacts with the outside world through providers — git commits, PR reviews, Jira comments, email — not by writing files that get pulled back to the host.

## Three Domains

| Domain | Question | Config | Commands |
|--------|----------|--------|----------|
| **Infrastructure** | How is the gateway deployed? | `openshell.toml` | `deploy`, `teardown --k8s` |
| **Providers** | What credentials are available and valid? | `providers.toml` | `providers`, `preflight` |
| **Sandbox** | What sandbox do I want? | `profiles/*.toml` | `new`, `connect` |

Each domain has its own config, its own code boundary, and its own concerns. A sandbox profile says what providers a sandbox *wants*. The provider catalog says what *exists* and how to validate it. The infrastructure layer handles *where* it all runs.

## Quick Start

```bash
# Authenticate with Google Cloud (Vertex AI inference)
gcloud auth application-default login

# Set credentials
export GITHUB_TOKEN="ghp_..."
export JIRA_API_TOKEN="..."

# Local (Podman gateway)
harness new --local

# Remote (OpenShift gateway)
harness new --remote

# Reconnect
harness connect
```

## Prerequisites

- [OpenShell CLI](https://github.com/NVIDIA/OpenShell) (`openshell`)
- `gcloud auth application-default login` (Vertex AI)
- Podman (local) or kubectl + helm (OpenShift)

Optional: `gws` CLI (Google Workspace), `bats` (tests)

## Commands

```
harness new [--local|--remote] [--profile NAME]
    Full flow: deploy gateway + register providers + create sandbox.

harness connect [NAME]
    Reconnect to a running sandbox.

harness deploy [--local|--remote]
    Deploy or verify the gateway without creating a sandbox.

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
image = "quay.io/rcochran/openshell:sandbox"
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
  { key = "JIRA_URL", kind = "env" },
]
```

Providers with `type = "openshell"` are registered with the gateway and managed by OpenShell's credential proxy. Providers with `type = "custom"` are workarounds for integrations OpenShell doesn't natively support yet — each tracks its upstream issue.

## Architecture

```
Your machine                      OpenShift cluster
┌──────────┐                    ┌──────────────────────────────┐
│ harness  │  Route (mTLS)      │ Gateway (StatefulSet)         │
│ CLI      ├───────────────────►│   ├─ gRPC API                 │
│          │                    │   ├─ inference.local proxy     │
└──────────┘                    │   ├─ Provider credential store │
                                │   └─ OAuth token refresh       │
                                │                                │
                                │ Sandbox Pods                   │
                                │   ├─ Claude Code → Vertex AI   │
                                │   ├─ mcp-atlassian             │
                                │   ├─ gws CLI                   │
                                │   ├─ gh CLI                    │
                                │   └─ L7 network proxy          │
                                └──────────────────────────────┘
```

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
| `sandbox/` | Sandbox container image (Dockerfile, startup, policy, agent instructions) |
| `sandbox/launcher/` | In-cluster launcher for OCP sandboxes |
| `sandbox/profiles/` | OpenShell provider type profiles (YAML, imported into gateway) |
| `deploy/` | K8s manifests (RBAC, Route) |
| `test/` | Tests (bats preflight, test-flow.sh integration) |

## Future Direction

- **Proto-based profiles** — align profile schema with OpenShell's proto types (`SandboxSpec`, `ProviderProfile`) for compile-time upstream compatibility. See [proto_migration.md](proto_migration.md).
- **Gateway configs** — per-target deployment configs (`gateways/ocp/`, `gateways/kind/`) replacing hardcoded deploy logic. See [docs/design.md](docs/design.md).
- **Direct gRPC** — replace CLI exec with gRPC calls to the gateway, eliminating output parsing. The Gateway interface already abstracts this swap.
- **Shrink** — as OpenShell adds native support for GWS credentials, provider config injection, and in-cluster sandbox creation, remove the corresponding harness workarounds.

## Testing

```bash
make validate            # full matrix: {bash,go} x {podman,ocp}
go test ./...            # Go unit tests
bats test/preflight.bats # 29 preflight unit tests
```

## Contributing

See [AGENTS.md](AGENTS.md) for coding guidelines, project principles, and workaround tracking.
