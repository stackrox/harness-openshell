# harness

> **Experimental.** Built on [OpenShell](https://github.com/NVIDIA/OpenShell), which is itself alpha software. Expect breaking changes in both.

Declarative workflow layer for OpenShell AI agent sandboxes.

## Quick Start

```bash
harness init                        # generate a config
harness doctor                      # check your environment
harness apply -f harness.yaml       # launch a sandbox
```

### Coding agent

Launch an interactive coding session with Claude Code or OpenCode.

```bash
harness apply --attach                                        # local Podman with built-in harness
harness apply -f harness.yaml --attach --gateway openshift    # Agent config in harness.yaml on OpenShift
harness apply -f harness.yaml --attach --entrypoint opencode  # OpenCode
```

### One-shot tasks

Run a task headlessly -- the agent executes in a sandbox and outputs results.

```bash
harness apply -f harness.yaml --task "review this codebase for security issues"
harness apply -f harness.yaml --task @skills/cpp-pro/SKILL.md
```

### Clone a repo into the sandbox

Use `base_agent` to inherit providers and inference routing from an existing config. The `repo` field clones the repository outside the sandbox and uploads it -- OpenShell sandboxes have no host mounts by design.

```yaml
name: reviewer
base_agent: default
repo: https://github.com/stackrox/collector
task: "identify the highest-priority C++ remediation"
```

```bash
harness apply -f reviewer.yaml
```

To get results out: `--task` mode outputs to stdout, `openshell sandbox exec` pulls files, or attach a `github` provider so the agent can push directly via the scoped proxy token.

## Why this exists

[OpenShell](https://github.com/NVIDIA/OpenShell) is a sandbox management layer with deny-by-default L7 network policy, credential proxy, filesystem isolation, and inference routing. It is designed as a strict, secure base that supports other workflows. 

One YAML file defines the agent, providers, payloads, and policy and one command deploys it via Podman or remotely on Kubernetes.

OpenShell's upstream direction is toward a [Kubernetes Operator](https://github.com/NVIDIA/OpenShell/issues/1719) where providers and sandboxes become CRDs and the gateway narrows to data-plane only. The harness explores what the workflow layer looks like above that with a developer mindset from local machine to cluster.

## The Agent YAML

A single file defines the entrypoint, credential providers, inference routing, environment, and files uploaded to the sandbox. This is the default config (`profiles/agent-default.yaml`):

```yaml
name: agent
entrypoint: claude
tty: true

providers:
  - profile: github                               # scoped GITHUB_TOKEN via proxy
  - profile: google-vertex-ai                     # inference routing through gateway
  - profile: atlassian                            # Jira/Confluence via mcp-atlassian
    env:
      JIRA_URL:                                   # empty = read from host env
      JIRA_USERNAME:
  - profile: google-workspace                     # Gmail, Calendar, Drive via gws CLI

env:
  ANTHROPIC_BASE_URL: https://inference.local     # route inference through gateway proxy
  ANTHROPIC_API_KEY: sk-ant-openshell-proxy-managed
  CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS: "1"

payloads:
  - sandbox_path: /sandbox/.claude/CLAUDE.md      # agent instructions
    local_path: profiles/images/sandbox-default/CLAUDE.md
  - sandbox_path: /sandbox/.claude.json           # claude code settings
    local_path: profiles/images/sandbox-default/claude.json
  - sandbox_path: /sandbox/.claude/settings.json  # permissions and defaults
    local_path: profiles/images/sandbox-default/settings.json
  - sandbox_path: /sandbox/.mcp.json              # MCP server config (jira, confluence)
    local_path: profiles/images/sandbox-default/mcp.json
```

Credentials never enter the sandbox -- the gateway proxy resolves placeholder tokens at the network boundary. Each provider also contributes its own L7 network policy endpoints and binary allowlists.

Use `harness apply -o yaml` to see the fully resolved config -- providers expand to show credential definitions, endpoint policies, scopes, and refresh strategies.

### Multi-document YAML

Bundle agent, providers, payloads, and policy in one self-contained file. Use `base_agent` to inherit from an existing config:

```yaml
---
kind: agent
name: security-reviewer
base_agent: default                               # inherits providers, env, payloads
repo: https://github.com/stackrox/collector
task: "review for memory safety issues"
---
kind: payload
sandbox_path: /sandbox/.claude/CLAUDE.md
content: |
  You are a C++ security review agent specializing in RAII,
  move semantics, and concurrency safety. Focus on the
  highest-priority remediation and explain the fix.
---
kind: policy
network_policies:
  github_git:
    endpoints:
      - host: github.com
        port: 443
        rules:
          - allow: { method: GET, path: "/**/info/refs*" }
          - allow: { method: POST, path: "/**/git-upload-pack" }
    binaries:
      - { path: /usr/bin/git }
```

This inherits all four providers and inference routing from `agent-default.yaml`, adds a custom CLAUDE.md as the agent's instructions, and defines an L7 policy that allows git clone but blocks git push at the HTTP method level.

## How It Works

```
harness apply -f config.yaml
    |
    +-> Deploy gateway (Podman container or K8s StatefulSet)
    +-> Register providers (credentials from host env)
    +-> Upload payloads (CLAUDE.md, MCP config, skills)
    +-> Create sandbox (isolated container, deny-by-default network)
    +-> Run task (agent executes, outputs results)
```

OpenShell provides the runtime isolation. The harness provides the workflow.

For runtime operations and policy management, use openshell directly:
```bash
openshell sandbox connect <name>     # interactive shell
openshell sandbox exec <name> -- ... # run commands
openshell sandbox logs <name>        # view logs
openshell policy get <name>          # inspect active policy
openshell term                       # interactive policy terminal
```

`openshell term` provides a live view of policy decisions -- which requests are allowed, denied, or pending review. This is how you audit and tune the deny-by-default L7 network policy while an agent is running.

## Install

```bash
# macOS
brew tap nvidia/openshell && brew install openshell && brew services start openshell

# Download the harness binary
curl -L https://github.com/stackrox/harness-openshell/releases/latest/download/harness_darwin_arm64 -o harness
chmod +x harness
```

Or build from source: `make cli`

## Reference

### Commands

| Command | What it does |
|---------|--------------|
| `harness init` | Generate a harness.yaml (interactive or `--non-interactive`) |
| `harness doctor` | Validate environment (offline + online checks) |
| `harness apply -f FILE` | Deploy a sandbox from config |
| `harness apply --task TEXT` | One-shot headless run |
| `harness apply --task @FILE` | One-shot from a skill/playbook file |
| `harness apply --attach` | Interactive TTY mode |
| `harness apply --dry-run` | Validate without deploying |
| `harness apply -o yaml` | Output resolved config |
| `harness deploy <gateway>` | Deploy gateway only |
| `harness get agents\|providers\|gateways` | List resources |
| `harness describe <name>` | Sandbox details |
| `harness delete <name> [--all]` | Tear down |

### Credentials

Each provider discovers credentials from the host. Missing providers are skipped.

| Provider | Required |
|----------|----------|
| `github` | `GITHUB_TOKEN` env var |
| `google-vertex-ai` | `gcloud auth application-default login` + `ANTHROPIC_VERTEX_PROJECT_ID` |
| `atlassian` | `JIRA_API_TOKEN` + `JIRA_URL` + `JIRA_USERNAME` |
| `google-workspace` | `gws auth login` ([gws CLI](https://github.com/googleworkspace/cli)) |

### Config Files

| File | Purpose |
|------|---------|
| `profiles/agent-*.yaml` | Agent configs |
| `profiles/providers/` | Provider profiles (imported to gateway) |
| `profiles/gateways/*.yaml` | Gateway profiles per target |
| `profiles/images/sandbox-default/` | Sandbox image defaults (overridable via payloads) |

## Testing

Tested on macOS (arm64) with Podman. Linux support is expected but not yet validated.

```bash
make test             # vet + unit tests (5 packages)
make lint             # golangci-lint
make test-suite       # config parsing (23 tests, no gateway needed)
make test-local       # full e2e on local Podman (22 tests)
make test-kind        # self-contained kind cluster lifecycle
make test-remote      # full e2e on OCP (needs KUBECONFIG)
```

`test-local` is the primary validation target. It deploys the gateway, registers all 4 providers, creates sandboxes, verifies exec/env/GWS token resolution/MCP config/Claude inference, tests missing-provider recovery, and tears down.

`test-kind` creates its own kind cluster, builds and loads the sandbox image, runs the full flow, and deletes the cluster on exit. Use `KEEP=1` to keep the cluster for debugging.

`test-remote` requires `KUBECONFIG` pointing at an OCP cluster and pushes the image automatically. Use `--reuse-gateway` to skip deploy/teardown when iterating.

Each integration target builds (and pushes, for remote) the sandbox image automatically.

## Future Work

- **GitHub Action** -- run harness tasks in CI (review PRs, enforce standards, generate reports)
- **Observability** -- structured telemetry export (Langfuse, MLflow, OpenTelemetry) for agent tool calls, token usage, and policy decisions
- **Skills integration** -- first-class support for community skill packs (e.g., [awesome-omni-skills](https://github.com/diegosouzapw/awesome-omni-skills)) as task inputs
- **OpenShell plugin** -- register the harness as an `openshell` CLI plugin so `openshell harness apply` works natively alongside other openshell commands
- **Linux validation** -- CI and local testing on Linux (currently macOS-only)

## Documentation

| Document | What it is |
|----------|------------|
| [SPEC.md](SPEC.md) | Behavior spec for the CLI |
| [AGENTS.md](AGENTS.md) | Contributor guide |
| [TODO.md](TODO.md) | Roadmap and upstream tracking |
