# harness

Harness is a declarative runner for [OpenShell](https://github.com/NVIDIA/OpenShell).
A repository checks in a workflow describing one trusted task; Harness resolves
that document, runs it in an isolated OpenShell sandbox, returns the result, and
cleans up the run.

Its purpose is to remove repeated gateway, credential, sandbox-lifecycle, and CI
plumbing from repository workflows. The repository still owns the task behavior:
skills, prompts, review criteria, source checkout, and result handling.

Harness is not a second OpenShell, provider manager, credential store, policy
language, scheduler, controller, or release manager. The closest operational
model is submitting a Kubernetes `Job`: `plan` previews a run and `apply` runs
one. There is no Harness database, release history, rollback, or watch loop.

## The workflow model

The workflow file is a desired input document, not a stored Harness resource:

```yaml
apiVersion: harness.openshell.dev/v1alpha1
kind: Harness
metadata:
  name: pr-review
spec:
  target:
    gateway: acs
    workspace: stackrox
  providers:
    - name: github-review
      management: referenced
  sandbox:
    image: quay.io/example/reviewer:v1
    providers: [github-review]
    policy:
      file: review-policy.yaml
    keep: false
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

The document can declare a gateway/workspace target, references to existing
providers, an inference route, sandbox image/policy/environment, agent command,
source checkout, and payload files. `providers` are references; provider
credentials and permissions remain OpenShell-owned.

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
For post-run debugging, set `spec.sandbox.keep: true` and use native OpenShell
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

## GitHub Actions

The reusable PR reviewer is the first supported workflow archetype:
`pr-reviewer-with-comments`. The consuming repository supplies its skill and
review criteria; Harness supplies the execution boundary.

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
`apply` cleanup still deletes a sandbox when `spec.sandbox.keep` is false.

## Documentation and validation

- [AGENTS.md](AGENTS.md) — architecture constraints and validation matrix
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
