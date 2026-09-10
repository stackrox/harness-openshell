# harness

> **Experimental.** Built on [OpenShell](https://github.com/NVIDIA/OpenShell), which is itself alpha software. Expect breaking changes in both.

Run workflows in OpenShell AI agent sandboxes. The current focus is portable
PR review with trusted, repository-controlled skills.

## Quick Start

```bash
harness init                        # generate a config
harness doctor -f harness.yaml      # check your environment
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
HyperShell's job, not the harness's (see [Install](#install)). When none is
declared, apply uses the active OpenShell gateway registration.

### Target resolution

Effective target resolution is:

1. explicit flags (`--gateway`, `--workspace`)
2. environment (`OPENSHELL_GATEWAY`, `OPENSHELL_WORKSPACE`)
3. workflow config (`spec.target.gateway`, `spec.target.workspace`)
4. OpenShell active gateway selection (gateway only)

When no flag, `OPENSHELL_GATEWAY`, or `spec.target.gateway` selects a named
gateway, direct SDK/OIDC targeting can come from
`OPENSHELL_GATEWAY_ENDPOINT` plus all three of `OPENSHELL_OIDC_ISSUER`,
`OPENSHELL_OIDC_CLIENT_ID`, and `OPENSHELL_OIDC_AUDIENCE`.
All direct-target fields are required; otherwise Harness falls back to the
CLI-managed gateway configuration. `OPENSHELL_OIDC_CLIENT_SECRET` remains
required at runtime and is never part of the workflow document.

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

[OpenShell](https://github.com/NVIDIA/OpenShell) owns gateway provisioning,
sandbox isolation, credential proxying, provider lifecycle, and network policy.
Harness prepares workflow inputs, stages source and skills, runs an agent, and
reports the result while cleaning up its sandbox.

The next portability milestone is running the same PR-review package in a
second repository with that repository's trusted skill. Users should customize
review behavior through skills; maintained integrations and platform setup
should supply provider credentials and native OpenShell policy. The current
review example still has repository-local orchestration and requires credential
wiring improvements before it meets that goal.

Workflows target a local OpenShell gateway or a configured HyperShell gateway.
Provider references are read-only. Inference reconciliation remains supported
while existing callers migrate to platform-configured routes. See the
[code audit](docs/code-audit.md) for the dependency inventory and remaining cuts.

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
Providers are read-only references; OpenShell/platform bootstrap owns their
creation, updates, and deletion. Relative payload
and policy paths resolve from the workflow file's directory.

Workflow schema essentials:

- `spec.providers` verifies existing provider references; `spec.sandbox.providers` attaches provider capabilities to the sandbox runtime.
- `management: referenced` is optional and is the only supported management mode. Provider configuration belongs in OpenShell/platform bootstrap.
- `spec.inference.verify: true` enforces inference-route endpoint checks during inference route writes.
- `spec.source.repo` is cloned outside the sandbox and uploaded; `spec.payloads[*].source` and `spec.sandbox.policy.file` resolve relative to the workflow file.
- Pin `spec.source.ref` to a full commit SHA for repeatable source inputs. Branches and tags resolve at preparation time; an omitted ref uses remote HEAD. Apply reports the actual prepared commit from the host checkout, including the commit behind an annotated tag. Missing refs fail instead of falling back to HEAD. This identifies the initial checkout, not later agent edits or payload overlays, and is not yet a structured run-result artifact.
- `spec.source.destination` is the parent directory for the checkout, not a rename: `/sandbox` plus a repository named `stackrox` produces `/sandbox/stackrox`. Omitting the destination uses `/sandbox`.

Canonical workflows use the OpenShell SDK for sandbox creation, policy
application, readiness, source and payload uploads, execution, and cleanup.
Interactive workflows use the same path with host terminal resize and raw-mode
handling. Canonical sandbox images must be registry references; local build
contexts are rejected.

### Execution results

For a machine-readable completion record alongside normal agent output:

```bash
harness apply -f workflow.yaml --result-file result.json
```

The opt-in JSON record contains `version: 1`, a random `runId`, UTC `startedAt`
and `finishedAt`, monotonic `durationMillis`, `status`, and the last `phase`.
`sourceCommit` is included after source preparation succeeds and comes from the
host checkout—not agent output. It identifies the initial source commit, not
payload overlays or later agent modifications.

Statuses are `succeeded`, `failed`, `cancelled`, or `timed_out`. Phases are
`load`, `plan`, `preflight`, `prepare`, `reconcile`, `execute`, and `complete`.
`execute` includes sandbox creation, upload, command execution, and sandbox
cleanup; a cleanup error returned by the runner makes the result unsuccessful.
Success is not a claim about review quality or independent cleanup verification.
This flag does not introduce a task timeout; `timed_out` records a reported
deadline failure.

The file is created with owner-only permissions before gateway access, must not
already exist (including as a symlink), and its parent directory must exist.
It cannot be combined with `--dry-run`, `--output`, or `--setup-only`, or used
with a workflow that has no sandbox run. Ordinary execution failures still
produce a result and a nonzero process exit; file-writing errors also fail the
command. Forced termination or disk failure can leave an empty/incomplete file:
consumers must require valid JSON and check the process exit, not file existence.

The result deliberately omits configuration values, prompts, raw errors, and
agent output. It is a completion record, not yet a complete input manifest or
review artifact bundle.

## How It Works

```
(OpenShell has already provisioned the gateway; you selected it)
harness apply -f config.yaml
    |
    +-> Verify provider references and configure declared inference
    +-> Create sandbox (isolated container, deny-by-default network)
    +-> Upload payloads (CLAUDE.md, MCP config, skills)
    +-> Run task (agent executes, outputs results)
```

OpenShell provisions the gateway and provides the runtime isolation. The harness provides the workflow.

## Architecture boundary

Harness owns the trusted execution contract: resolving the gateway and target,
passing provider references without exposing credential values, relying on
OpenShell's proxy and masking behavior, creating and cleaning up the sandbox,
enforcing bounded execution, validating results, and preventing stale or
untrusted inputs from becoming part of a run.

The repository using Harness owns the task: its trusted skills, review or task
criteria, source inputs, agent and model choice, and what to do with the
result. Those decisions should stay in the consuming repository rather than
become Harness policy.

This is a portability boundary, not an obligation to use Harness everywhere.
If a repository can run a native OpenShell workflow with the same safety and
less bookkeeping, that is the better choice. Harness earns its place when it
removes repeated credential, lifecycle, and CI integration code while keeping
task behavior in the repository that owns it.

For runtime operations and policy management, use openshell directly:
```bash
openshell sandbox connect <name>     # interactive shell
openshell sandbox exec <name> -- ... # run commands
openshell sandbox logs <name>        # view logs
openshell policy get <name>          # inspect active policy
openshell term                       # interactive policy terminal
```

`openshell term` provides a live view of policy decisions -- which requests are allowed, denied, or pending review. This is how you audit and tune the deny-by-default L7 network policy while an agent is running.

## Prerequisites

- OpenShell CLI and gateway service at the repo-pinned version (see `make openshell` and `.openshell-version`).
- An active OpenShell gateway registration (`openshell gateway add ...`, `openshell gateway select ...`).
- Providers already configured on the gateway for any references.

## Install

```bash
# OpenShell CLI + local gateway, pinned to the version this repo targets
# (.openshell-version). Installs the exact release CI uses and starts the
# managed gateway service (Homebrew/launchd on macOS, systemd on Linux).
make openshell

# Download the harness binary for your OS/arch
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64) ARCH=amd64 ;;
  arm64|aarch64) ARCH=arm64 ;;
esac
curl -L "https://github.com/stackrox/harness-openshell/releases/latest/download/harness_${OS}_${ARCH}" -o harness
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

Or build from source with `make cli` (uses your local Go toolchain).

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
sandboxes only. Use `openshell provider delete` to remove providers and upstream
tools to remove the gateway.

Provider-management migration: `management: managed`, provider `adopt`/`config`,
and `harness delete --providers`/`--all` are removed. Configure providers with
OpenShell and reference their names in workflows. Use `delete --sandboxes` only
for a dedicated workspace, or delete individual sandbox names.

> **Migration:** `harness deploy`, `harness teardown`, `harness status`, and
> `delete --k8s` are removed. Provision the gateway with OpenShell (the
> `openshell` installer or `helm install openshell`); the harness declares
> providers/inference/policy and runs agents against it.

## Reference

### Commands

| Command | What it does |
|---------|--------------|
| `harness init` | Generate a harness.yaml (interactive or `--non-interactive`) |
| `harness doctor` | Validate gateway reachability and referenced providers |
| `harness apply -f FILE` | Deploy a sandbox from config |
| `harness apply -f FILE --attach` | Interactive TTY mode |
| `harness apply -f FILE --setup-only` | Verify provider references and configure inference (skip sandbox run) |
| `harness apply -f FILE --dry-run` | Render the v1alpha1 action plan without mutating |
| `harness apply -f FILE -o yaml` | Output resolved config with interpolated and credential-bearing map values redacted |
| `harness get gateways` | Show active gateway only (name, endpoint, status, version) |
| `harness get agents\|providers` | List resources |
| `harness describe <name>` | Sandbox details |
| `harness delete <name>` / `harness delete --sandboxes` | Delete named sandboxes or all sandboxes in the selected workspace |
| `harness plan -f FILE` | Read-only reconciliation plan (mutates nothing) |

### Credentials

Apply is strict: referenced providers must already exist, and credentialed
provider creation is a separate platform/bootstrap responsibility. The harness
does not read local provider credentials; `doctor` verifies that each referenced
provider is registered on the selected gateway.

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

Interactive TTY behavior is covered by unit and race tests but remains a manual
terminal check: run `harness apply -f harness.yaml --attach`, resize the terminal,
then exit and confirm the terminal mode is restored. CI has no stable controlling
TTY, so it does not claim a live interactive proof.

## Documentation

| Document | What it is |
|----------|------------|
| [AGENTS.md](AGENTS.md) | Contributor guide |
| [docs/](docs/) | Repo-facing docs index |
| [docs/ci.md](docs/ci.md) | HyperShell CI bootstrap and repository contract |
| [docs/compatibility.md](docs/compatibility.md) | Tested OpenShell, ACP, and Go versions |
| [profiles/README.md](profiles/README.md) | Profile layout and examples |
