# Design: openshell-harness

## Why this tool exists

AI agent sandboxes need a consistent, reproducible environment: the right
gateway deployed, the right providers registered with valid credentials,
the right image with the right tools, and the right configuration — every
time, on every machine, for every team member.

Without this tool, setting up a sandbox means:
- Manually deploying an OpenShell gateway (Helm install with 6 steps, mTLS
  cert extraction, SCC grants on OpenShift)
- Manually registering providers (Vertex AI, GitHub, Atlassian) with
  credentials scattered across env vars, ADC files, and API tokens
- Debugging when credentials expire, proxies misconfigure, or sandboxes
  fail to reach endpoints — with no visibility into what's broken
- Repeating all of this on every new machine, every cluster, every time
  someone joins the team

**Goals:**

1. **One command to working sandbox.** `harness up` takes a developer from
   zero to a running AI agent sandbox — gateway deployed, providers
   registered, sandbox created, credentials validated.

2. **Reproducible environments.** Sandbox profiles define exactly what a
   sandbox needs. The same profile produces the same environment regardless
   of who runs it or where.

3. **Credential visibility.** Two-level provider health checking tells you
   whether credentials are valid both locally (can you register?) and
   inside the sandbox (can the agent actually use them?). No more "it
   worked on my machine."

4. **Clean separation of concerns.** Infrastructure config, provider
   management, and sandbox profiles are independent. Changing your Jira
   token doesn't require editing sandbox profiles. Switching clusters
   doesn't require re-registering providers.

5. **Thin wrapper, not a platform.** The harness wraps openshell — it
   doesn't replace it. Users can drop to `openshell` commands at any time.
   The harness adds orchestration, validation, and configuration management
   on top.

## What this tool is

openshell-harness is a **gateway harness** — it deploys, configures, and manages
an OpenShell gateway so you can create sandboxes through it. The gateway is the
central concept. Providers and sandboxes are things you do through the gateway.

```
              ┌─────────────┐
              │   gateway    │  ← the harness IS this
              │  (deploy,    │
              │   configure, │
              │   manage)    │
              └──────┬───────┘
                     │
             ┌───────┴───────┐
             │               │
        providers        sandboxes
        (register,       (create,
         validate)        connect)
```

Every command is implicitly scoped to "the gateway." The tool name is the
noun — `harness deploy` means deploy the gateway, `harness create` means
create a sandbox through it, `harness providers` means register providers
with it.

## Three domains

The tool has three domains of concern. Each has its own config, commands,
and code boundary.

### 1. Infrastructure — "How is the backend deployed?"

The gateway itself: deploying to local podman or remote OpenShift, Helm
chart management, mTLS, namespace setup, CRDs, RBAC.

**Config:** `harness.toml` (chart version, namespace, registry)
**Commands:** `deploy`, `down`, `status`
**Code:** `internal/gateway/` (CLI wrapper), k8s deploy/teardown logic

### 2. Providers — "What's available and ready?"

Provider catalog, credential validation, registration with the gateway,
and runtime health checking inside sandboxes.

**Config:** `providers.toml` (catalog with preflight inputs and in-sandbox verification)
**Commands:** `providers register`, `providers list`, `preflight`, `status providers`
**Code:** provider registration, preflight checks

### 3. Sandbox — "What sandbox do I want?"

Sandbox profiles, creation, connection, lifecycle management.

**Config:** `profiles/*.toml` (image, command, env, provider list)
**Commands:** `create`, `connect`, `status`
**Code:** profile parsing, staging, sandbox create/connect

## Command interface

All commands are flat — no `gateway X` grouping because the gateway is
implicit in everything the harness does.

```
harness deploy [--local|--remote]       Deploy the gateway (local is default)
harness create [NAME] [--profile P]     Create a sandbox (errors if gateway not ready)
harness connect [NAME]                  Reconnect to a running sandbox
harness up [--local|--remote]           Full flow: deploy → providers → create
harness down [--sandboxes|--providers|--gateway|--all]
harness providers register [--force]    Register providers with the gateway
harness providers list                  List registered providers
harness preflight [--strict]            Validate local environment prerequisites
harness status                          Overview: gateway, providers, sandboxes
harness status providers                Deep provider health (local + in-sandbox)
```

### Command design principles

- `create` only creates a sandbox. It does not deploy the gateway or
  register providers. If the gateway isn't ready, it errors with guidance.
- `up` is the orchestrator. It chains deploy → providers → create. This is
  the "I don't care, just make it work" command.
- `down` is the inverse of `up`. Bare `down` prompts for confirmation.
- `status` is always read-only, always safe.
- Commands mirror openshell verbs where possible (`create` not `new`,
  matching `openshell sandbox create`).

### Naming decisions

| Current | Proposed | Why |
|---------|----------|-----|
| `new` | `create` | Matches `openshell sandbox create`. "new" is for scaffolding. |
| `deploy` (requires --local) | `deploy` (defaults to local) | Local is the 90% case. |
| `teardown --k8s` | `down --gateway` | `--k8s` is an implementation detail. |
| `openshell.toml` | `harness.toml` | The file configures harness, not openshell. |
| `OPENSHELL_MODEL` etc. | `HARNESS_MODEL` etc. | With backward-compat shims. |
| `sandbox/profiles/` | `sandbox/provider-profiles/` | Disambiguate from sandbox profiles. |

## Config files

Three files, one per domain. No duplication.

### `harness.toml` — infrastructure config

```toml
# Gateway deployment settings.

[upstream]
chart-version = "0.0.55"

# Optional overrides (also settable via env vars):
# namespace = "openshell"          # HARNESS_NAMESPACE
# registry = "quay.io/rcochran"   # HARNESS_REGISTRY
```

Also serves as the harnessDir sentinel (replaces `profiles/default.toml`
for directory detection).

### `providers.toml` — provider catalog

Defines all available providers, their preflight inputs (local validation),
and in-sandbox verification commands.

```toml
[[providers]]
name = "atlassian"
type = "openshell"
description = "Jira and Confluence via mcp-atlassian"
required = false

# Local preflight checks — run on the user's machine before registration
inputs = [
  { key = "JIRA_API_TOKEN", kind = "env", secret = true },
  { key = "JIRA_URL", kind = "env" },
]

# In-sandbox verification — run inside a running sandbox to confirm
# the provider actually works end-to-end
verify = "curl -sf -u $JIRA_USERNAME:$JIRA_API_TOKEN $JIRA_URL/rest/api/2/myself -o /dev/null"
```

The provider list in the profile (domain 3) says what a sandbox *wants*.
The provider catalog says what *exists* and how to validate it.
Registration errors surface the gap.

### `profiles/*.toml` — sandbox profiles

```toml
name = "agent"
image = "quay.io/rcochran/openshell:sandbox"
command = "claude --bare"
keep = true

providers = ["github", "vertex-local", "atlassian"]

[env]
# Sandbox-level env vars only. Provider-specific config (JIRA_URL, etc.)
# flows from provider registration, not from here.
CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS = "1"
```

**Principle:** the profile describes the sandbox shape. Provider credentials
and provider-specific env vars are provider concerns, not profile concerns.

## Provider health: two levels

Provider validation happens at two levels:

### Level 1: Local preflight

"Do I have the credentials to register this provider?"

Runs on the user's machine. Uses `inputs` from `providers.toml` to check
env vars, files, and external commands. This is what `harness preflight`
does today.

### Level 2: In-sandbox verification

"Can the sandbox actually use this provider?"

Runs inside a running sandbox via `openshell sandbox exec`. Uses the
`verify` field from `providers.toml`. Catches:
- Expired credentials
- Gateway proxy misconfiguration
- Network endpoint unreachable from sandbox
- Env vars staged incorrectly

### `harness status providers`

Shows both levels:

```
=== Providers ===
  ✓ github        local: GITHUB_TOKEN set    sandbox: git ls-remote ok
  ✓ vertex-local  local: ADC valid           sandbox: inference reachable
  ✗ atlassian     local: JIRA_API_TOKEN set  sandbox: curl /myself failed (401)
```

If no sandbox is running, only local checks run. The in-sandbox column
shows "no sandbox" instead of pass/fail.

## Code organization

The three domains map to internal packages:

```
internal/
  gateway/     Domain 1: Gateway CLI wrapper, gateway admin operations
  provider/    Domain 2: Provider catalog, preflight, registration (new, from existing preflight/)
  profile/     Domain 3: Sandbox profile parsing, staging, env building (exists)
  k8s/         Shared: kubectl/helm/oc runner (exists)
  status/      Shared: terminal output helpers (exists)
```

`cmd/` files are thin — flag parsing and one function call into the
appropriate domain package. Orchestration logic (`up` = deploy → providers
→ create) lives in `cmd/up.go`, composing domain operations.

### Migration path

This is not a rewrite. The domains already exist in the code — they're
just tangled in `cmd/new.go` (325 lines spanning all three domains).
The migration is:

1. Extract provider registration from `cmd/providers.go` into `internal/provider/`
2. Move preflight from `internal/preflight/` into `internal/provider/` (it's provider validation)
3. Slim `cmd/new.go` → `cmd/create.go` (profile parse + sandbox create, ~60 lines)
4. New `cmd/up.go` composes: deploy + providers register + create
5. Rename `cmd/teardown.go` → `cmd/down.go`
6. New `cmd/status.go` reads across all three domains

Each step is independently committable and testable.

## Proto migration (deferred)

The proto migration plan (proto-migration.md) is architecturally sound but
premature. The tool has ~5 profiles and ~4 providers. Schema drift is real
but manageable at this scale.

**When to do it:** when the tool moves from CLI exec to gRPC, at which
point proto types ARE the request payloads and the migration pays for itself.

**Interim approach:** if compile-time safety is needed before gRPC, parse
TOML into proto-generated structs internally with a ~50-line mapping layer.
Users keep writing TOML. No textproto, no format change.

## Open questions

- Should `harness init` (from release-plan.md) be a separate command or
  part of the first `harness deploy` run?
- Should provider-specific env vars (JIRA_URL, JIRA_USERNAME) flow from
  `providers.toml` values or from a separate provider config mechanism?
- Should `verify` commands in providers.toml support a timeout field?
- How should `status providers` handle providers that take >5s to verify?
