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
harness apply -f harness.yaml --attach                        # interactive agent
harness apply -f harness.yaml --attach --entrypoint opencode  # override the executable
```

`harness apply` uses `spec.target`, `--gateway`, and `--workspace` with flag,
environment, then config precedence. Provisioning the gateway is OpenShell's or
HyperShell's job, not the harness's (see [Install](#install)).

### One-shot tasks

Run a task headlessly -- the agent executes in a sandbox and outputs results.

Declare the command in `spec.agent.type` and `spec.agent.args`, then run
`harness apply -f harness.yaml`. Payload files can carry longer instructions.

### Clone a repo into the sandbox

Set `spec.source.repo`. The harness clones outside the sandbox and uploads the
checkout; OpenShell sandboxes have no host mounts by design.

```yaml
apiVersion: harness.openshell.dev/v1alpha1
kind: Harness
metadata:
  name: reviewer
spec:
  source:
    repo: https://github.com/stackrox/collector
  sandbox:
    image: quay.io/example/reviewer:v1
  agent:
    type: claude
    args: [--print, "identify the highest-priority C++ remediation"]
```

```bash
harness apply -f reviewer.yaml
```

The command writes results to stdout. For retained sandboxes, use
`openshell sandbox exec`; a referenced GitHub provider can allow a scoped push.

## Why this exists

[OpenShell](https://github.com/NVIDIA/OpenShell) provides a strict, secure sandbox runtime — deny-by-default L7 network policy, credential proxying, Landlock filesystem isolation, and inference routing. It also provisions the gateway itself (the local installer, or `helm install openshell` on a cluster). What it doesn't provide is the developer workflow layer on top: the config that wires up providers, the declarative reconciliation that makes a gateway match your intent, or the CI harness that catches breakage before developers hit it.

Without a shared harness layer, every team building on OpenShell independently solves the same problems — writing shell scripts to register providers, hand-rolling container images, re-deriving inference routing. The configs diverge, the security posture varies, and nobody catches regressions until something breaks in production.

**The design boundary**: managing a gateway is OpenShell's problem; the harness is a declarative setup/run layer with zero compute-backend opinion. It never provisions or tears down a gateway — it declares providers, inference, and policy against one OpenShell already stood up, and runs agents in it. The workflow remains portable because its target can be overridden by standard gateway and workspace flags or environment variables.

**The core design constraint**: if the developer harness isn't running and live-tested in CI, the developer experience can't be maintained. OpenShell, agent CLIs, and provider APIs all change frequently — often multiple times per week. A harness that works today and isn't continuously validated will silently break. CI exercises the workflow against local and Kind gateways on Linux. OpenShift remains a manually credentialed integration target.

**The path from local to automated**: a developer runs
`harness apply -f harness.yaml --attach` for interactive work, then checks agent
arguments and payloads into the same workflow for headless CI.

Every config command uses `harness.openshell.dev/v1alpha1`. Plan and apply share
strict parsing, environment resolution, target resolution, and action decisions.
Unversioned files are rejected.

OpenShell's upstream direction is toward a [Kubernetes Operator](https://github.com/NVIDIA/OpenShell/issues/1719) where providers and sandboxes become CRDs and the gateway narrows to data-plane only. The harness explores what the workflow layer looks like above that with a developer mindset from local machine to cluster.

## The v1alpha1 workflow

The canonical workflow is accepted by both `plan` and `apply`:

```yaml
apiVersion: harness.openshell.dev/v1alpha1
kind: Harness
metadata:
  name: security-review
spec:
  target:
    gateway: acs
    workspace: stackrox
  providers:
    - name: github-read
      management: referenced
  sandbox:
    image: quay.io/example/security-reviewer:v1
    providers: [github-read]
    keep: false
    tty: false
  agent:
    type: claude
    args: [--print, "Review the repository for security defects"]
  source:
    repo: https://github.com/stackrox/stackrox
    ref: main
    destination: /sandbox/stackrox
```

`plan` is read-only and may render desired state while the gateway is offline.
`apply` requires the effective gateway to be reachable, verifies referenced
providers before sandbox creation, and disables OpenShell provider auto-discovery.
Managed providers may be updated or explicitly adopted, but apply does not create
credentialed providers; platform bootstrap owns their creation. Relative payload
and policy paths resolve from the workflow file's directory.

Canonical workflows use the OpenShell SDK for sandbox creation, policy
application, readiness, source and payload uploads, execution, and cleanup.
Interactive workflows use the same path with host terminal resize and raw-mode
handling. Canonical sandbox images must be registry references; local build
contexts are rejected.

## How It Works

```
(OpenShell has already provisioned the gateway; you selected it)
harness apply -f config.yaml
    |
    +-> Verify/reconcile declared providers and inference
    +-> Upload payloads (CLAUDE.md, MCP config, skills)
    +-> Create sandbox (isolated container, deny-by-default network)
    +-> Run task (agent executes, outputs results)
```

OpenShell provisions the gateway and provides the runtime isolation. The harness provides the workflow.

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
# OpenShell CLI + local gateway, pinned to the version this repo targets
# (.openshell-version). Installs the exact release CI uses and starts the
# managed gateway service (Homebrew/launchd on macOS, systemd on Linux).
make openshell

# Download the harness binary
curl -L https://github.com/stackrox/harness-openshell/releases/latest/download/harness_darwin_arm64 -o harness
chmod +x harness
```

Install a bare `brew install openshell` off the tap and you get whatever version
the formula defaults to — usually behind. `make openshell` runs the upstream
`install.sh` at the pinned version instead, so local matches CI exactly.

The installer starts the gateway service; register and select it once:

```bash
openshell gateway add https://127.0.0.1:17670 --local --name openshell
openshell gateway select openshell
```

If you need to restart the service later: `brew services restart openshell`
(macOS) or `systemctl --user restart openshell-gateway` (Linux).

Or build the harness from source: `make cli`

### On a cluster

Provisioning a cluster gateway is OpenShell's job too — the harness has no
`deploy` command. Install the chart, then register and select the gateway:

```bash
helm install openshell oci://ghcr.io/nvidia/openshell/helm-chart
openshell gateway add https://<gateway-endpoint> --name my-cluster
openshell gateway select my-cluster
harness apply -f harness.yaml            # same YAML, cluster gateway
```

Tear the gateway down with `helm uninstall openshell` and
`openshell gateway remove my-cluster`. The harness `delete` command removes
sandboxes; add `--providers` (or `--all`) to remove providers too. It never
removes the gateway.

> **Migration:** `harness deploy`, `harness teardown`, `harness status`, and
> `delete --k8s` are removed. Provision the gateway with OpenShell (the
> `openshell` installer or `helm install openshell`); the harness declares
> providers/inference/policy and runs agents against it.

## Reference

### Commands

| Command | What it does |
|---------|--------------|
| `harness init` | Generate a harness.yaml (interactive or `--non-interactive`) |
| `harness doctor` | Validate environment (offline + online checks) |
| `harness apply -f FILE` | Deploy a sandbox from config |
| `harness apply -f FILE --attach` | Interactive TTY mode |
| `harness apply --dry-run` | Render the v1alpha1 action plan without mutating |
| `harness apply -f FILE -o yaml` | Output resolved config |
| `harness get agents\|providers\|gateways` | List resources |
| `harness describe <name>` | Sandbox details |
| `harness delete <name> [--all]` | Tear down |
| `harness plan -f FILE` | Read-only reconciliation plan (mutates nothing) |

### Credentials

Apply is strict: referenced providers must already exist, and credentialed
provider creation is a separate platform/bootstrap responsibility.

| Provider | Required |
|----------|----------|
| `github` | `GITHUB_TOKEN` env var |
| `google-vertex-ai` | `gcloud auth application-default login` + `ANTHROPIC_VERTEX_PROJECT_ID` |
| `atlassian` | `JIRA_API_TOKEN` + `JIRA_URL` + `JIRA_USERNAME` |
| `google-workspace` | `gws auth login` ([gws CLI](https://github.com/googleworkspace/cli)) |

### Config Files

| File | Purpose |
|------|---------|
| `profiles/harness-basic.yaml` | Canonical v1alpha1 scaffold used by `harness init` and default `doctor` checks |
| `profiles/providers/` | Provider-profile examples used by diagnostics and platform bootstrap |
| `profiles/images/sandbox-default/` | Build context for the published sandbox image |

## Testing

Developer testing primarily uses macOS (arm64) with Podman. GitHub Actions runs
unit, local-gateway, and Kind integration coverage on Linux. OpenShift integration
is available as a manually credentialed target.

```bash
make test             # vet + unit tests
make lint             # golangci-lint
make test-suite       # config and CLI checks (no gateway needed)
make test-local       # full e2e on local Podman
make test-kind        # self-contained kind cluster lifecycle
make test-remote      # full e2e on OCP (needs KUBECONFIG)
```

`test-local` is the primary validation target. It provisions a gateway via the
OpenShell installer, runs the canonical sandbox lifecycle, exercises available
pre-registered provider capabilities, and tears down the resources it created.

`test-kind` creates its own kind cluster, `helm install`s OpenShell, builds and loads the sandbox image, runs the full flow, and deletes the cluster on exit. Use `KEEP=1` to keep the cluster for debugging.

`test-remote` requires `KUBECONFIG` pointing at an OCP cluster and pushes the image automatically. Use `--reuse-gateway` to skip gateway provisioning/teardown when iterating.

Each integration target builds (and pushes, for remote) the sandbox image automatically.

## Documentation

| Document | What it is |
|----------|------------|
| [AGENTS.md](AGENTS.md) | Contributor guide |
| [docs/ci.md](docs/ci.md) | HyperShell CI bootstrap and repository contract |
