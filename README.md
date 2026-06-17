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
harness apply -f harness.yaml --attach                       # local Podman
harness apply -f harness.yaml --attach --gateway openshift    # on OpenShift
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

[OpenShell](https://github.com/NVIDIA/OpenShell) is a foundation layer -- sandboxed containers with deny-by-default L7 network policy, credential proxy, Landlock filesystem isolation, and inference routing. It is designed as a strict, secure base that other tooling builds workflows on top of.

The harness is one such workflow layer. One YAML file defines the agent, providers, payloads, and policy. One command deploys it -- locally via Podman or remotely on Kubernetes.

OpenShell's upstream direction is toward a [Kubernetes Operator](https://github.com/NVIDIA/OpenShell/issues/1719) where providers and sandboxes become CRDs and the gateway narrows to data-plane only. The harness explores what the workflow layer looks like above that -- and covers the local Podman development path that no operator will own.

## The Agent YAML

A single file defines what runs, what credentials it gets, and what files are uploaded to the sandbox.

```yaml
name: agent
entrypoint: claude
tty: true
repo: https://github.com/stackrox/collector   # cloned outside sandbox, uploaded in

providers:
  - profile: github
  - profile: google-vertex-ai
  - profile: atlassian
    env:
      JIRA_URL:
      JIRA_USERNAME:

env:
  ANTHROPIC_BASE_URL: https://inference.local

payloads:
  - sandbox_path: /sandbox/.claude/CLAUDE.md
    local_path: profiles/images/sandbox-default/CLAUDE.md
  - sandbox_path: /sandbox/.mcp.json
    local_path: profiles/images/sandbox-default/mcp.json
```

### Multi-document YAML

Bundle agent, providers, payloads, and policy in one file:

```yaml
---
kind: agent
name: cpp-reviewer
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
  You are a C++ security review agent.
---
kind: policy
network_policies:
  github:
    endpoints:
      - { host: "api.github.com", port: 443 }
```

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
curl -L https://github.com/robbycochran/harness-openshell/releases/latest/download/harness_darwin_arm64 -o harness
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

## Documentation

| Document | What it is |
|----------|------------|
| [SPEC.md](SPEC.md) | Behavior spec for the CLI |
| [AGENTS.md](AGENTS.md) | Contributor guide |
| [TODO.md](TODO.md) | Roadmap and upstream tracking |
