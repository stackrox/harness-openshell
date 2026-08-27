# profiles/

Agent configs, provider profiles, and gateway profiles.

## Agent configs (`agent-*.yaml`)

Define what runs in the sandbox. One agent config = one sandbox.

```yaml
name: agent                     # sandbox name
base_agent: default             # inherit from agent-default.yaml (providers, env, payloads)
entrypoint: claude              # claude, opencode, bash, or any binary on PATH
tty: true                       # enable TTY (default: true)
repo: https://github.com/org/repo  # cloned outside sandbox, uploaded to /sandbox/<repo>
repo_ref: main                  # branch, tag, or ref to clone (default: HEAD)
task: @tasks/review.md          # task file passed to entrypoint via -p
image: ghcr.io/...              # override sandbox image
policy: path/to/policy.yaml     # network policy file

providers:                      # credential providers to register
  - profile: github             # references profiles/providers/ or OpenShell built-in
  - profile: google-vertex-ai
  - profile: atlassian
    env:                        # non-secret env vars for this provider
      JIRA_URL:                  # empty = read from host env
      JIRA_USERNAME:

env:                            # additional env vars injected into sandbox
  ANTHROPIC_BASE_URL: https://inference.local

payloads:                       # files uploaded to sandbox before start
  - sandbox_path: /sandbox/.claude/CLAUDE.md
    local_path: images/sandbox-default/CLAUDE.md
  - sandbox_path: /sandbox/.config/instructions.md
    content: |
      Inline content works too.
```

All fields except `name` are optional. Minimal config:

```yaml
name: agent
```

### Multi-document format

Agent configs support multi-document YAML (`---` separated). Each document is dispatched by the `kind` field. No `kind` = agent (backwards compatible).

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
kind: payload
sandbox_path: /sandbox/.claude/CLAUDE.md
content: |
  You are a code review agent.
---
kind: policy
network_policies:
  github:
    endpoints:
      - { host: "api.github.com", port: 443 }
```

Supported kinds: `agent`, `provider`, `payload` (alias: `config`), `policy`.

A `kind: gateway` document is still accepted for backward compatibility but has
no effect: the harness no longer provisions gateways — it targets whichever
gateway OpenShell already stood up and you selected (`openshell gateway
select`). `harness migrate` warns when it finds one so you can register it with
the openshell CLI instead.

## Providers (`providers/`)

See [providers/README.md](providers/README.md).
