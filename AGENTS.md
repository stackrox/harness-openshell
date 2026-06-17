# Contributing

## Coding Guidelines

**Tradeoff:** These guidelines bias toward caution over speed. For trivial tasks, use judgment.

### Think Before Coding

Don't assume. Don't hide confusion. Surface tradeoffs.

- State your assumptions explicitly. If uncertain, ask.
- If multiple interpretations exist, present them - don't pick silently.
- If a simpler approach exists, say so. Push back when warranted.
- If something is unclear, stop. Name what's confusing. Ask.

### Simplicity First

Minimum code that solves the problem. Nothing speculative.

- No features beyond what was asked.
- No abstractions for single-use code.
- No "flexibility" or "configurability" that wasn't requested.
- No error handling for impossible scenarios.
- If you write 200 lines and it could be 50, rewrite it.

### Surgical Changes

Touch only what you must. Clean up only your own mess.

When editing existing code:
- Don't "improve" adjacent code, comments, or formatting.
- Don't refactor things that aren't broken.
- Match existing style, even if you'd do it differently.
- If you notice unrelated dead code, mention it - don't delete it.

When your changes create orphans:
- Remove imports/variables/functions that YOUR changes made unused.
- Don't remove pre-existing dead code unless asked.

The test: Every changed line should trace directly to the user's request.

### Goal-Driven Execution

Define success criteria. Loop until verified.

- "Add validation" → "Write tests for invalid inputs, then make them pass"
- "Fix the bug" → "Write a test that reproduces it, then make it pass"
- "Refactor X" → "Ensure tests pass before and after"

For multi-step tasks, state a brief plan:
```
1. [Step] → verify: [check]
2. [Step] → verify: [check]
3. [Step] → verify: [check]
```

## Project Principles

1. **Simplicity** — fewer scripts, fewer moving parts, less code. If something can be a single command, don't wrap it in a script.

2. **Scoped customization** — anything custom should be clearly scoped so it can be replaced by a built-in OpenShell feature when available. Document which upstream issue or feature would eliminate each workaround.

3. **Upstream alignment** — OpenShell is alpha and developing quickly. Don't fight the framework. Use native patterns (providers v2, profiles, inference.local, policy composition) and adapt when upstream changes.

## Upstream Conventions

Follow these conventions from the OpenShell ecosystem. Do not invent alternatives.

### Output format: `-o table|json|yaml`
Every list/get command must support `-o` with three formats. `table` is the default
for human consumption, `json` and `yaml` for machine consumption. Define a shared
`OutputFormat` type used by all commands. Match the convention from OpenShell
issues #1745 and #1750.

### Credential exclusion from structured output
Never serialize credential values into `-o json` or `-o yaml` output. Expose key
names only. This is a security invariant, not a nice-to-have. See OpenShell
PR #1830 `provider_to_json()` for the pattern.

### Flag resolution order
Explicit flag > `OPENSHELL_*` env var > config file > default. This matches the
plugin host contract from issue #1851. The env vars `OPENSHELL_GATEWAY`,
`OPENSHELL_GATEWAY_ENDPOINT`, and verbosity flags propagate from the plugin host.

### Policy schema
`kind: policy` documents must use the upstream OpenShell policy YAML schema
verbatim (from the `openshell-policy` crate). Do not invent a harness-specific
policy format. A policy written for the harness should be byte-compatible with
what `openshell-image-builder` generates.

### Provider abstraction
`kind: provider` is an abstraction layer, not a thin wrapper around
`openshell provider create`. The backend may change to gateway.toml entries
(#1886) or K8s CRDs (#1719) as upstream settles. Implement the imperative
CLI backend today. Do not hard-code the execution strategy.

### Plugin compatibility
The binary may eventually be discoverable as an OpenShell plugin via
`openshell-<name>` PATH-based discovery (#1851). Design for standalone first.
Plugin compatibility (binary naming, env var consumption) is additive. Do not
depend on plugin host behavior that is not yet accepted upstream.

### Do not cache or forward auth tokens
Issue #1851 explicitly prohibits token forwarding to plugins. The harness must
resolve credentials fresh via `openshell-bootstrap` or configured auth. Never
store, cache, or relay gateway auth tokens.

### Delegate image building
Do not replicate Dev Container Feature fetching or complex image composition.
The `openshell-image-builder` handles agent installation, settings bake-in,
policy composition, and OCI artifact fetching. If the harness needs advanced
image support, generate config the image-builder consumes rather than
reimplementing in Go.

## The harness should shrink, not grow

This harness exists to bridge gaps in OpenShell's current capabilities. As OpenShell matures, custom code should be replaced by upstream features. Every workaround should reference the upstream issue that would eliminate it.

Current workarounds and their upstream tracking:

| Workaround | Why | Upstream |
|------------|-----|----------|
| Custom sandbox image | Adds mcp-atlassian, GWS CLI, and opencode-ai to community base | Upstreaming MCP integrations |
| `CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS=1` | Vertex AI rejects `context_management` beta header | Anthropic/Google to align APIs |
| Atlassian `JIRA_URL`/`JIRA_USERNAME` as agent YAML provider config | Provider v2 config keys not injected as env vars yet | OpenShell roadmap |

Previously worked around, now resolved:

| Was | Resolution |
|-----|-----------|
| GWS credential export/upload to sandbox | GWS is now a native provider (`google-workspace`). Gateway manages OAuth refresh via `oauth2-refresh-token` strategy + `request_body_credential_rewrite` on `oauth2.googleapis.com`. Sandbox gets a proxy-resolved placeholder. |
| In-cluster runner Job | Eliminated — all targets now use direct mode. The gateway is accessible via external Route + mTLS, so sandboxes are created from the user's machine. |

## Validation

Validation has two modes — **default** and **ci** — that are independent of where
the gateway runs (local Podman, kind, OCP). The mode controls what is tested, not
the target.

### Modes

**`default`** — expects user credentials. Tests the full stack including provider
registration, credential injection, and the GWS OAuth token lifecycle.

**`ci`** — no credentials required. Tests gateway deploy and sandbox lifecycle only.
Runs in GitHub Actions on every PR.

```
--ci flag  =  --no-providers --agent=ci
```

CI mode also auto-activates when the `CI` env var is `true` (set by GitHub Actions).

### Make targets

See `make help` for the full list. The test entry points:

| Target | Gateway | Mode |
|--------|---------|------|
| `make test` | none | vet + unit tests |
| `make test-local` | local Podman | default locally, ci on GHA |
| `make test-kind` | kind cluster | default locally, ci on GHA |
| `make test-remote` | OCP | default (needs KUBECONFIG + creds) |
| `make test-all` | all of the above | |

Or directly:
```bash
./test/test-flow.sh local          # default mode
./test/test-flow.sh local --ci     # ci mode
./test/test-flow.sh kind --ci      # used in GitHub Actions
```

### Default mode requirements

- `openshell` gateway running locally (`brew services start openshell`)
- `JIRA_API_TOKEN`, `JIRA_URL`, `JIRA_USERNAME` for Atlassian
- `gcloud auth application-default login` for Vertex AI
- `gws auth login` for Google Workspace
- `GITHUB_TOKEN` for GitHub

Default mode does not run in GitHub Actions today — it requires personal OAuth
credentials. Future: service accounts for Vertex AI and Atlassian can run in GHA;
GWS would need a dedicated OAuth service account.

### What each mode tests

| Check | ci | default |
|-------|----|---------|
| Gateway deploy and rollout | ✓ | ✓ |
| Sandbox create / exec / delete | ✓ | ✓ |
| Provider registration | — | ✓ |
| `GOOGLE_WORKSPACE_CLI_TOKEN` is proxy placeholder | — | ✓ |
| Gmail/Calendar/Drive API via proxy | — | ✓ |
| GWS token rotation survives in sandbox | — | ✓ |
| Inference (Vertex AI) | — | ✓ |
| Atlassian MCP server | — | ✓ |

## Adding a new integration

Before writing custom code:

1. Check if OpenShell already supports it (provider profiles, policy rules, inference routing)
2. Check if there's an open issue or PR upstream
3. If you must write custom code, scope it narrowly and document the upstream path to removal
