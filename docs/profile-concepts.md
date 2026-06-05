# Profile Concepts: OpenShell, Kaiden, and harness-openshell

Three projects use the word "profile" to mean fundamentally different things. This document maps each concept, identifies where they overlap and diverge, and notes implications for harness-openshell.

---

## 1. OpenShell — Provider Type Profiles

**Source:** [NVIDIA/OpenShell#896](https://github.com/NVIDIA/OpenShell/issues/896) ("Enhanced Provider Management")

### What it is

A **Provider Type Profile** is a declarative YAML file that bundles everything OpenShell needs to know about a single external service (GitHub, Claude, Slack, Vertex AI, etc.):

| Section | Purpose |
|---------|---------|
| `credentials` | Expected secrets, env vars, injection style (bearer, header, query, path) |
| `endpoints` | Host/port allowlist using the existing network policy language |
| `deny` | Deny rules the user cannot override (e.g., block branch-protection endpoints) |
| `binaries` | Allowed executables for this provider |
| `verify` | Optional probe endpoint for creation-time connectivity checks |
| `inference` | Base URL, protocol, default headers (inference-capable providers only) |
| `refresh` | Credential refresh strategy (static, oauth2, external) |

### Key design decisions

- **Policy composition:** Three layers composed JIT — base sandbox policy, auto-generated provider policy, user-authored policy. Deny wins over allow; provider deny rules cannot be bypassed.
- **Attach/detach lifecycle:** Providers can be added/removed from running sandboxes because credential injection is proxy-side, not env-var-at-start.
- **Custom registry:** Users can `export`, fork, and `import` their own profiles, enabling custom provider types without upstream changes.
- **Credential scoping:** Credentials bound to `(credential, endpoint, binary)` triples — a GitHub PAT can only reach `api.github.com`, not be exfiltrated to an arbitrary endpoint.

### Implementation status (as of June 2026)

**Shipped:** Profile schema, built-in profiles, custom registry (import/export/lint), JIT policy composition, attach/detach lifecycle, `providers_v2_enabled` feature gate, docs.

**Remaining:** Profile-driven credential injection (replacing placeholder env scanning), credential scoping enforcement, credential verification, credential refresh lifecycle, inference automation (`inference.local` routing), HA hardening.

### Open sub-issues (blockers for full vision)

- [#894](https://github.com/NVIDIA/OpenShell/issues/894) — Placeholder model breaks SDKs that validate token format before any network call (Slack `xoxb-` prefix check)
- [#913](https://github.com/NVIDIA/OpenShell/issues/913) — WSS payloads bypass L7 rewriting (Discord gateway opcode 4004)

Both motivate the move from placeholder env vars to proxy-side credential injection declared in provider profiles.

### Related active work

- [#1306](https://github.com/NVIDIA/OpenShell/issues/1306) — Gateway-owned credential refresh for short-lived tokens
- [#1622](https://github.com/NVIDIA/OpenShell/pull/1622) — Path-based `auth_style` for provider profiles
- [#1638](https://github.com/NVIDIA/OpenShell/pull/1638) — AWS SigV4 credential signing at the proxy
- [#1681](https://github.com/NVIDIA/OpenShell/pull/1681) — Okta OBO token exchange
- [#1736](https://github.com/NVIDIA/OpenShell/issues/1736) — Dynamic identity sources (SPIFFE/K8s) for OAuth2TokenExchange

---

## 2. Kaiden — Projects (Workspace Configuration Profiles)

**Source:** [openkaiden/kaiden#1272](https://github.com/openkaiden/kaiden/issues/1272) ("Projects management"), [Milestone 0.3 — Mycelium](https://github.com/openkaiden/kaiden/milestone/4)

### What it is

A **Project** is a reusable configuration template that defines everything needed to spin up an agent workspace. It sits between "raw resources" (models, MCP servers, skills, knowledge bases) and "running sandboxed workspaces."

```typescript
interface WorkspaceProjectInfo {
  id: string;
  name: string;
  folder: string;          // local directory or git URL
  skills: string[];        // enabled skill names
  mcpServers: string[];    // enabled MCP server names
  knowledges: string[];    // enabled RAG knowledge base names
  secrets: string[];       // enabled secret names
  filesystem: FilesystemConfiguration;  // mode + mount points
  network: NetworkConfiguration;        // mode + allowed hosts
}
```

### Key design decisions

- **Local-directory-first:** A project is anchored to a folder on the host machine (or a Git URL). The folder is the source code the agent will work in.
- **Resource selection, not definition:** Projects reference skills, MCP servers, knowledges, and secrets by name — they don't define them. The resources exist independently; projects toggle which ones are active.
- **Template → instance separation:** Projects are reusable templates; workspaces are ephemeral running instances created from projects.
- **UI-driven creation:** Two-step wizard (source → review) with per-resource checklists for skills, MCP servers, etc.
- **Stored as JSON** on disk under a `workspace-projects` directory.

### Implementation status (as of June 2026, milestone due 2026-06-17)

| Sub-issue | Status | What |
|-----------|--------|------|
| [#1925](https://github.com/openkaiden/kaiden/issues/1925) | Merged | Backend CRUD, JSON storage, reference validation |
| [#1926](https://github.com/openkaiden/kaiden/issues/1926) | Merged | Projects list page (searchable table) |
| [#1927](https://github.com/openkaiden/kaiden/issues/1927) | Merged | Project details page (Overview + Settings tabs) |
| [#1928](https://github.com/openkaiden/kaiden/issues/1928) | PR open | Create wizard (local folder or Git URL) |
| [#1929](https://github.com/openkaiden/kaiden/issues/1929) | Open | Wire MCP servers to create wizard |
| [#1930](https://github.com/openkaiden/kaiden/issues/1930) | PR open | Wire skills to create wizard |
| [#2066](https://github.com/openkaiden/kaiden/issues/2066) | Open | Feed project config into workspace creation |

### What Kaiden's "project" is NOT

- It is not about provider type definitions — Kaiden delegates that entirely to OpenShell
- It does not define credential injection, endpoint allowlists, or deny rules
- It does not manage credential refresh or verification
- It is a higher-level orchestration concern: "which combination of pre-existing resources does this workspace need?"

---

## 3. harness-openshell — Profiles + Custom Providers

### What it is

**Profiles** in harness-openshell are TOML files (`profiles/*.toml`) that define per-sandbox configuration:

```toml
image = "ghcr.io/org/agent-image:latest"
command = "claude --bare"
name = "agent"
keep = true
providers = ["github", "vertex-local", "atlassian"]

[env]
SOME_VAR = "value"
```

**Custom providers** are a workaround layer for integrations OpenShell doesn't natively support yet. Defined in `providers.toml` with type `"custom"` (vs. `"openshell"` for gateway-managed providers). Today only `gws` (Google Workspace) is custom — its credentials are decrypted locally and uploaded directly into the sandbox, bypassing the gateway's credential store.

### How the pieces fit

```
providers.toml            profiles/*.toml         sandbox/profiles/*.yaml
─────────────             ──────────────          ──────────────────────
Catalog of all known      Per-sandbox config:     OpenShell-native provider
providers + their         image, command, env,    type profiles (YAML) that
prerequisites (inputs)    which providers to      get imported into the
                          attach                  gateway via `openshell
                                                  provider profile import`
```

1. `harness providers` registers providers with the gateway (using `providers.toml` inputs)
2. `harness new --profile NAME` reads `profiles/NAME.toml`, validates its `providers` list against the gateway, creates the sandbox
3. Custom providers (like `gws`) bypass the gateway entirely — files are uploaded directly

### Current limitations that motivated this analysis

- Only one profile exists today: `default.toml`
- Custom providers are workarounds that should shrink as OpenShell's provider v2 matures
- The `sandbox/profiles/*.yaml` directory (OpenShell provider type profiles) only has `atlassian.yaml` — as more custom providers get native support upstream, more would move here
- Profile config doesn't express the resource-selection semantics Kaiden projects add (skills, MCP servers, knowledge bases)

---

## Comparison Matrix

| Dimension | OpenShell Provider Profile | Kaiden Project | harness-openshell Profile |
|-----------|---------------------------|----------------|---------------------------|
| **Unit of** | A service/integration type | A workspace template | A sandbox configuration |
| **Defines** | Credentials, endpoints, deny rules, binaries, inference, refresh | Which skills/MCP/secrets/knowledge are active, source folder, filesystem/network mode | Image, command, env vars, which providers to attach |
| **Scope** | Single provider (GitHub, Claude, etc.) | Entire workspace (many providers, tools, data sources) | Single sandbox (one image, one command, N providers) |
| **Stored as** | YAML in gateway registry | JSON on disk | TOML on disk |
| **Lifecycle** | Registered once, attached/detached per sandbox | Created once, instantiated as workspaces | Read at sandbox creation time |
| **Credential handling** | Proxy-side injection, refresh, scoping | References secrets by name (delegates to OpenShell) | Delegates to gateway (openshell providers) or uploads directly (custom providers) |
| **Network policy** | Auto-generated from endpoint declarations + deny rules | Mode + hosts array (delegates enforcement to OpenShell) | Not expressed — relies on gateway defaults + manual policy |
| **Custom extensibility** | export/fork/import registry | Add resources to the project's resource lists | Add TOML files to `profiles/` or YAML to `sandbox/profiles/` |

---

## Key Observations

### 1. Three layers of the same stack

These three concepts operate at different layers of the same stack:

```
┌─────────────────────────────────────────────┐
│  Kaiden Project                              │  "What combination of resources
│  (workspace template)                        │   does this workspace need?"
├─────────────────────────────────────────────┤
│  harness-openshell Profile                   │  "What image, command, env, and
│  (sandbox configuration)                     │   providers does this sandbox use?"
├─────────────────────────────────────────────┤
│  OpenShell Provider Type Profile             │  "What does this single service
│  (service integration definition)            │   need to work inside a sandbox?"
└─────────────────────────────────────────────┘
```

They are complementary, not competing. Kaiden selects which providers a workspace gets; harness-openshell attaches those providers to a sandbox; OpenShell's provider profiles define what each provider actually means.

### 2. Kaiden's "project" adds the resource-selection layer harness-openshell doesn't have

harness-openshell profiles today are focused narrowly: image + command + env + provider list. Kaiden adds:
- **Skills selection** (enable/disable per workspace)
- **MCP server selection** (enable/disable per workspace)
- **Knowledge base selection** (RAG sources per workspace)
- **Secrets selection** (which secrets are accessible)
- **Filesystem configuration** (mount mode + mount points)
- **Network configuration** (mode + host allowlist)

This is relevant if harness-openshell ever needs to support multi-agent or multi-tool configurations where different sandboxes need different subsets of available resources.

### 3. harness-openshell's custom providers are a bridge

The `providers.toml` custom provider mechanism (type: `"custom"`) exists specifically to fill gaps in OpenShell's provider v2 system. As #896's remaining work lands (especially profile-driven credential injection and credential refresh), custom providers like `gws` should migrate to native OpenShell provider type profiles. The `upstream` field in `providers.toml` already tracks which upstream issue would obsolete each workaround.

### 4. OpenShell's credential model is evolving toward what harness-openshell needs

The two open sub-issues under #896 (#894 and #913) are exactly the problems that make custom providers necessary in harness-openshell — SDKs that can't work with placeholder tokens need real credentials, which means bypassing the gateway. Once proxy-side injection and credential refresh land upstream, the custom provider workaround layer can shrink.

### 5. Kaiden's worktree/local-dir focus is orthogonal to harness-openshell's image-first approach

Kaiden projects start from a local folder (the code you want the agent to work on). harness-openshell profiles start from a container image (the pre-built agent environment). These are different entry points to the same result (a running sandbox), but they reflect different usage patterns:
- **Kaiden:** "I have source code, give me a configured agent to work on it"
- **harness-openshell:** "I have a pre-built agent image, configure the sandbox it runs in"

---

## Implications for harness-openshell

### Short term (now)
- Continue using custom providers as the bridge for services OpenShell doesn't natively support
- Track OpenShell #896 progress — each shipped sub-feature is a chance to remove a custom workaround
- Keep profiles simple (TOML, image + command + env + providers) — don't add Kaiden-style resource selection unless needed

### Medium term (as OpenShell provider v2 matures)
- Migrate custom providers to native OpenShell provider type profiles (`sandbox/profiles/*.yaml`)
- Adopt profile-driven credential injection when it ships, eliminating direct credential upload
- Consider whether `providers.toml` should shrink to just a preflight validation catalog (no longer distinguishing custom vs. openshell types)

### Long term (if scope expands)
- If harness-openshell needs multi-tool or multi-agent workspace configuration, Kaiden's resource-selection model (skills, MCP servers, knowledge bases per profile) is a reasonable pattern to follow
- The three-layer stack (provider type → sandbox config → workspace template) is a natural factoring if the project grows beyond single-sandbox use cases
