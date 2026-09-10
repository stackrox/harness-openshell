# harness

Harness is a declarative runner for [OpenShell](https://github.com/NVIDIA/OpenShell).
A repository checks in workflow documents describing repository automation or a
developer session. The same workflow can run from GitHub Actions, another CI
system, or a local terminal with `--attach`. Harness resolves the document,
runs it in an isolated OpenShell sandbox, returns the result, and cleans up the
run.

Its purpose is to remove repeated gateway, credential, policy, sandbox-lifecycle,
and CI plumbing from repository workflows. Each workflow can combine a target,
provider references, policies, skills, an agent, and an inference route for a
specific use case. The repository still owns task behavior, prompts, review
criteria, source checkout, and result handling.

Harness is not a second OpenShell, provider manager, credential store, policy
language, scheduler, controller, or release manager. The closest operational
model is submitting a Kubernetes `Job`: `plan` previews a run and `apply` runs
one. There is no Harness database, release history, rollback, or watch loop.

## The workflow model

The workflow file is a desired input document, not a stored Harness resource:

```yaml
version: 1
name: pr-review
target:
  gateway: acs
  workspace: stackrox
sandbox:
  image: quay.io/example/reviewer:v1
  providers: [github-review]
  policy:
    file: review-policy.yaml
payloads:
  - source: .github/skills/pr-review/SKILL.md
    destination: /sandbox/skills/pr-review/SKILL.md
source:
  repo: https://github.com/stackrox/stackrox
  ref: main
  destination: /sandbox/stackrox
agent:
  type: claude
  args: [--print, "Review the supplied repository input"]
```

The document can declare a gateway/workspace target, an inference route,
sandbox provider attachments, sandbox image/policy/environment, agent command,
source checkout, and payload files. Provider credentials and permissions remain
OpenShell-owned. Changing the target or policy lets the same repository
workflow run with a different trust boundary.

The one-shot lifecycle is:

```text
load and validate YAML
  → resolve flags, environment, and defaults
  → read gateway state and build a plan
  → verify references and reconcile compatibility settings
  → create, run, and observe the sandbox
  → return the result and clean up
```

The current inference-route write is a compatibility bridge for gateways that do
not yet own that configuration natively. It should shrink as OpenShell does.

## Use it locally

Install the OpenShell version in `.openshell-version`, then select a gateway
using the native CLI or target a configured HyperShell gateway:

```bash
make openshell
openshell gateway add https://127.0.0.1:17670 --local --name openshell
openshell gateway select openshell

harness workflow plan workflow.yaml
harness workflow apply workflow.yaml
harness workflow apply workflow.yaml --attach
```

`--attach` runs the same workflow with your terminal connected to the declared
agent command. It does not open a host shell or bypass the workflow policy.
For post-run debugging, set `sandbox.keep: true` and use native OpenShell
commands such as:

```bash
openshell sandbox connect <name>
openshell sandbox exec <name> -- <command>
openshell sandbox logs <name>
openshell sandbox delete <name>
```

## State, defaults, and configuration

Harness owns no durable workflow state. OpenShell or the platform owns gateway
registrations, workspaces, providers, inference routes, credential material,
policies, and sandboxes. GitHub Actions owns workflow-run state, labels,
artifacts, concurrency, and approvals. A host-side source cache and an explicit
result file are outputs or performance optimizations, not workflow state.

Resolution order for gateway and workspace targets is:

1. explicit flags (`--gateway`, `--workspace`);
2. `OPENSHELL_*` environment variables;
3. the workflow target;
4. the active/default OpenShell gateway.

`${VAR}` references in workflow strings are expanded from the calling process
environment. Harness does not implicitly load `.env` files; source one in the
calling shell or configure the values in CI. Defaults include workspace
`default`, inference route `inference.local`, and the versioned sandbox image;
`HARNESS_OS_IMAGE` overrides the image.

Direct OIDC target registration in a workflow is in-memory for that invocation.
The OIDC client secret is read from `OPENSHELL_OIDC_CLIENT_SECRET` and is never
part of the workflow document.

## Provider lifecycle

Harness does not create, update, or delete providers or credentials. A platform
administrator or trusted OpenShell bootstrap provisions them in the target
HyperShell workspace, for example with the native `openshell provider create`
flow. A workflow names providers where they are used: `inference.provider`
selects the inference provider and `sandbox.providers` attaches masked provider
proxies to the new sandbox. `plan` and `apply` verify those references before
execution.

If a referenced provider is absent, `apply` fails before creating the sandbox.
The gateway keeps the provider credential and exposes only its masked proxy
interface inside the sandbox. The runner's own gateway credential—local
OpenShell login, OIDC service account, or mTLS—is separate and is used only to
connect and create the sandbox; it is not automatically a sandbox provider.

The PR-review demo currently has a trusted shell bootstrap that creates
temporary providers for its self-contained test path. That is adapter-specific
bootstrap, not Harness workflow behavior. A HyperShell deployment should move
those providers to platform bootstrap and let the workflow reference the
pre-provisioned names.

## Credentials and policy

Workflow files contain provider names, not credentials. OpenShell resolves the
provider and exposes a proxy-backed, masked interface to authorized sandbox
requests. Raw credentials must not appear in workflow YAML, `sandbox.env`,
payloads, agent arguments, logs, artifacts, prompts, or structured JSON/YAML
output. Use provider configuration and OpenShell policy to grant capabilities;
provider attachment alone does not authorize comments, pushes, labels, or merges.

For GitHub Actions, trusted host-side setup may use the automatic
`GITHUB_TOKEN` to register the native OpenShell GitHub provider. The token is
not placed in the sandbox environment or agent payload. See
[docs/ci.md](docs/ci.md) for the bootstrap and secret contract.

## GitHub Actions and local sessions

The reusable PR reviewer is the first supported workflow archetype:
`pr-reviewer-with-comments`. The consuming repository supplies its skill and
review criteria; Harness supplies the execution boundary. The same runner can
also execute repository maintenance, CI assistance, research, or interactive
developer workflows when those workflows define the appropriate policy and
provider boundary.

```yaml
jobs:
  ai-review:
    uses: stackrox/harness-openshell/.github/workflows/pr-review-reusable.yml@<harness-sha>
    with:
      harness-ref: <same-40-character-harness-sha>
      skill-path: .github/skills/pr-review/SKILL.md
      allow-draft-reviews: false
    secrets: inherit
```

Use `pull_request_target` when the workflow needs secrets or write permissions;
the called workflow reads trusted files from the caller’s default branch and
stages the pull-request diff as data. The `ai-review` label is explicit opt-in
and is not added automatically. A `pull_request` trigger is appropriate only
for a credential-free demonstration.

Future archetypes such as issue triage, issue-to-PR, security review, or
auto-merge require separate mutation and approval contracts; they are not
implicitly enabled by the runner.

## Commands

| Command | Purpose |
|---|---|
| `harness workflow plan FILE` | Render a read-only plan |
| `harness workflow apply FILE` | Run the workflow headlessly |
| `harness workflow apply FILE --attach` | Run it with an interactive terminal |
| `harness workflow apply FILE --setup-only` | Verify references and configure inference without running a sandbox |

Plan and dry-run output support `-o table`, `-o json`, and `-o yaml`; credential
values are never serialized. The Harness CLI deliberately has no `doctor`,
`init`, `delete`, `get`, or `describe` commands. Use native OpenShell commands
for gateway health, sandbox inspection, and retained-sandbox deletion. Normal
`apply` cleanup deletes a sandbox by default; set `sandbox.keep: true` only to
retain it for debugging.

## Documentation and validation

- [AGENTS.md](AGENTS.md) — architecture constraints and validation matrix
- [docs/workflow-format.md](docs/workflow-format.md) — version 1 workflow contract
- [docs/ci.md](docs/ci.md) — trusted CI bootstrap and credential contract
- [docs/compatibility.md](docs/compatibility.md) — tested OpenShell, ACP, and Go versions
- [examples/github-pr-reviewer/](examples/github-pr-reviewer/) — workflow inputs and policy

Fast checks:

```bash
make test
make test-suite
```

Gateway lifecycle checks are available through `make test-local`, `make test-kind`,
and `make test-remote`. Provider-capability checks require platform-provisioned
credentials.
