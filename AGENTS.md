# Contributing

## Principles

1. **Simplicity** — fewer scripts, fewer moving parts, less code. If something can be a single command, don't wrap it in a script.

2. **Scoped customization** — anything custom should be clearly scoped so it can be replaced by a built-in OpenShell feature when available. Document which upstream issue or feature would eliminate each workaround.

3. **Upstream alignment** — OpenShell is alpha and developing quickly. Don't fight the framework. Use native patterns (providers v2, profiles, inference.local, policy composition) and adapt when upstream changes.

## The harness should shrink, not grow

This harness exists to bridge gaps in OpenShell's current capabilities. As OpenShell matures, custom code should be replaced by upstream features. Every workaround should reference the upstream issue that would eliminate it.

Current workarounds and their upstream tracking:

| Workaround | Why | Upstream |
|------------|-----|----------|
| GWS credential export/upload | gws CLI reads encrypted local files | [#1268](https://github.com/NVIDIA/OpenShell/issues/1268), [#1423](https://github.com/NVIDIA/OpenShell/issues/1423) |
| Custom gateway image | `google-vertex-ai` provider not in released builds yet | Will ship in upstream release |
| `CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS=1` | Vertex AI rejects `context_management` beta header | Anthropic/Google to align APIs |
| Atlassian `JIRA_URL`/`JIRA_USERNAME` as uploaded config | Provider v2 config keys not injected as env vars yet | OpenShell roadmap |
| In-cluster launcher Job | OpenShell doesn't have a native K8s-triggered sandbox creation | Potential future CRD |

## Adding a new integration

Before writing custom code:

1. Check if OpenShell already supports it (provider profiles, policy rules, inference routing)
2. Check if there's an open issue or PR upstream
3. If you must write custom code, scope it narrowly and document the upstream path to removal
