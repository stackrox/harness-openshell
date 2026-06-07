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

## The harness should shrink, not grow

This harness exists to bridge gaps in OpenShell's current capabilities. As OpenShell matures, custom code should be replaced by upstream features. Every workaround should reference the upstream issue that would eliminate it.

Current workarounds and their upstream tracking:

| Workaround | Why | Upstream |
|------------|-----|----------|
| Custom gateway image | `google-vertex-ai` provider not in released builds yet | Will ship in upstream release |
| `CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS=1` | Vertex AI rejects `context_management` beta header | Anthropic/Google to align APIs |
| Atlassian `JIRA_URL`/`JIRA_USERNAME` as `--config` material | Provider v2 config keys not injected as env vars yet | OpenShell roadmap |
| In-cluster launcher Job | OpenShell doesn't have a native K8s-triggered sandbox creation | Potential future CRD |

Previously worked around, now resolved:

| Was | Resolution |
|-----|-----------|
| GWS credential export/upload to sandbox | GWS is now a native provider (`google-workspace`). Gateway manages OAuth refresh via `oauth2_refresh_token` strategy + `request_body_credential_rewrite` on `oauth2.googleapis.com`. Sandbox gets a proxy-resolved placeholder. |

## Validation

There are two validation tiers, depending on whether real credentials are available.

### No-credential flows (CI / kind)

Run without any provider credentials. Tests sandbox lifecycle only — deploy, create, exec, delete.

```bash
make validate-kind          # kind cluster (local)
./test/test-flow.sh kind --full --no-providers --profile=ci
```

The `ci` profile uses the public community base image and attaches no providers. These flows run in GitHub Actions on every PR (the `kind` job in `.github/workflows/integration.yml`).

### Full flows (local dev, personal credentials)

Run with real credentials. Tests the complete provider chain including GWS OAuth token lifecycle.

```bash
make validate-local         # local Podman gateway
./test/test-flow.sh local --full
```

Requires:
- `openshell` gateway running locally (`brew services start openshell`)
- `JIRA_API_TOKEN`, `JIRA_URL`, `JIRA_USERNAME` for Atlassian
- `gcloud auth application-default login` for Vertex AI
- `gws auth login` for Google Workspace
- `GITHUB_TOKEN` for GitHub

These flows do not run in GitHub Actions today — they require personal OAuth credentials. Future work: service accounts for Vertex AI and Atlassian can run in GHA; GWS would need a dedicated service account.

### What each flow tests

| Check | No-cred (CI) | Full (local) |
|-------|-------------|--------------|
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
