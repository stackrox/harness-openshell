# harness

Harness is a small declarative runner for
[OpenShell](https://github.com/NVIDIA/OpenShell). It turns a repository-owned
workflow document into one isolated sandbox run: compose the inputs, verify
OpenShell references, execute the agent command, return declared outputs, and
clean up.

The same runner works from a local terminal or a reusable GitHub workflow. The
gateway can run beside the caller today and move to a managed service later
without changing the workload contract.

## Why it exists

OpenShell owns the hard runtime boundary: gateways, workspaces, providers,
credentials, inference routing, policy enforcement, and sandbox isolation.
Repositories still need a consistent way to combine those primitives for a
specific task and manage a one-shot run.

Harness standardizes only that seam. It is not a general agentic CI platform, a
provider manager, a credential store, a scheduler, or a second policy language.
The operational model is closer to submitting a Kubernetes `Job`: `plan`
previews one run and `apply` performs it. Harness has no database, release
history, rollback, or watch loop.

This narrow boundary is intentional. New task behavior belongs in a workload;
new CI permissions belong in a capability-specific reusable workflow; new
provider and workspace setup belongs in trusted platform bootstrap.

## How the pieces fit

```text
local CLI or capability-specific reusable workflow
                      │
                      ▼
         workload bundle in workloads/
       image + policy + provider references
          + payloads + agent + outputs
                      │
                      ▼
       harness workflow plan / apply
       compose + create + observe + clean up
                      │
                      ▼
       OpenShell gateway: local or managed
                      │
                      ▼
               isolated sandbox
```

| Layer | Owns |
|---|---|
| `.github/workflows/` | CI trigger, permissions, trusted checkout, concurrency, and host bootstrap for one capability |
| `workloads/` | Task policy, provider references, image, agent behavior, payloads, and outputs |
| `workflow/` | Generic `plan`/`apply` composition and one-shot sandbox lifecycle |
| `images/` | Reusable runtime toolchains, without credentials or task behavior |
| OpenShell and platform bootstrap | Gateway access, workspaces, provider credentials, inference routes, and sandbox isolation |

The runner never creates, updates, deletes, stores, or serializes provider
credentials. It verifies that provider references exist and asks OpenShell to
attach their masked proxy interfaces to the sandbox.

## Run a workflow

Build Harness from this repository:

```bash
make cli
```

Select a local OpenShell gateway with the native CLI, or declare a direct
managed target in the workflow:

```bash
openshell gateway add https://127.0.0.1:17670 --local --name openshell
openshell gateway select openshell

./harness workflow plan workflow.yaml
./harness workflow apply workflow.yaml
```

A version 1 workflow composes native OpenShell inputs:

```yaml
version: 1
name: pr-review
sandbox:
  image: ghcr.io/nvidia/openshell-community/sandboxes/base@sha256:...
  providers: [github-review]
  policy:
    file: review-policy.yaml
payloads:
  - source: .github/skills/pr-review/SKILL.md
    destination: /sandbox/skills/pr-review/SKILL.md
outputs:
  - source: /sandbox/artifacts
    destination: artifacts
    required: false
agent:
  type: opencode
  args: [run, --format, json]
```

The lifecycle is deliberately one-shot:

```text
load and validate
  → resolve target and inputs
  → inspect gateway state and build a plan
  → verify provider references
  → create, run, and observe the sandbox
  → download outputs and delete the sandbox
```

Use `--attach` for the same workflow with a connected terminal. Set
`sandbox.keep: true` only for post-run debugging with native
`openshell sandbox` commands. The complete schema, target resolution order,
direct OIDC contract, redaction rules, and defaults are in
[docs/workflow-format.md](docs/workflow-format.md).

## Use it across StackRox

The cross-repository GitHub Actions interface is a capability-specific reusable
workflow, not an arbitrary privileged `action.yml`. This lets each integration
fix its permissions, trusted inputs, concurrency, and workload selection.

```yaml
jobs:
  ai-review:
    uses: stackrox/harness-openshell/.github/workflows/pr-review-reusable.yml@<harness-sha>
    with:
      harness-ref: <same-40-character-harness-sha>
      skill-path: .github/skills/pr-review/SKILL.md
      allow-draft-reviews: false
      openshell-github-app-client-id: ${{ vars.OPENSHELL_GITHUB_APP_CLIENT_ID }}
    secrets:
      VERTEX_AI_SERVICE_ACCOUNT_KEY: ${{ secrets.VERTEX_AI_SERVICE_ACCOUNT_KEY }}
      OPENSHELL_GITHUB_APP_PRIVATE_KEY: ${{ secrets.OPENSHELL_GITHUB_APP_PRIVATE_KEY }}
```

Pin both references to the same immutable commit. The current reviewer uses
`pull_request_target`, checks out trusted workflow code from the caller's
default branch, and stages the pull-request diff only as untrusted data. The
`ai-review` label is explicit opt-in.

Review and merge remain separate capabilities because they require different
provider credentials and allowed mutations. Do not replace them with a single
workflow that accepts arbitrary image, policy, provider, or command inputs.

### Gateway transition

Today the reusable reviewer installs a local OpenShell gateway, then its trusted
wrapper creates an ephemeral workspace and providers before invoking Harness.
That bootstrap makes the current integration self-contained; it is not runner
behavior.

The intended managed arrangement removes those steps. The GitHub job
authenticates to the managed gateway, while platform bootstrap owns workspace
membership, pre-provisioned providers, and matching inference routes. The
reusable workflow still selects the workload, and Harness still performs the
same one-shot sandbox lifecycle.

## Security and ownership

- Workflow files, policies, payload declarations, and reusable-workflow code
  are trusted host-side inputs. Never execute versions supplied by an
  untrusted pull request in a credentialed context.
- Workflow provider fields contain names only. Raw credentials must not appear
  in workflow YAML, sandbox environment values, payloads, agent arguments,
  logs, prompts, or artifacts.
- Provider attachment supplies a masked proxy interface; native OpenShell
  policy separately controls which requests the sandbox may make.
- Harness owns no durable runtime state. OpenShell or platform bootstrap owns
  gateway resources; GitHub Actions owns run state, labels, artifacts,
  concurrency, and approvals.
- Sandboxes are deleted by default. Review and merge use separate provider and
  policy boundaries.

The current inference-route write is a compatibility bridge for isolated or
explicitly administered workspaces. Shared managed workspaces should have a
matching route provisioned by the platform so ordinary runs remain
reference-only. See [docs/ci.md](docs/ci.md) for the CI bootstrap and credential
contract.

## CLI

| Command | Purpose |
|---|---|
| `harness workflow plan FILE` | Read-only reconciliation plan |
| `harness workflow apply FILE` | Execute one workflow headlessly |
| `harness workflow apply FILE --attach` | Execute with an attached terminal |
| `harness workflow apply FILE --output-dir DIR` | Download declared outputs below `DIR` |
| `harness workflow apply FILE --setup-only` | Verify references and reconcile inference without starting a sandbox |

`plan` and dry-run output support `-o table|json|yaml`; credential values are
never serialized. Use `--result-file` for a host-derived execution result.
Harness deliberately has no provider-management, sandbox-inspection, or
deployment commands—use the native OpenShell CLI for those operations.

## Repository map

- [.github/workflows/](.github/workflows/) — capability-specific reusable workflows
- [workflow/](workflow/) — Go implementation of the runner
- [workloads/](workloads/) — native OpenShell task bundles and optional Harness adapters
- [images/](images/) — reusable sandbox image build contexts
- [docs/workflow-format.md](docs/workflow-format.md) — version 1 workflow contract
- [docs/ci.md](docs/ci.md) — trusted CI and managed-gateway bootstrap
- [docs/compatibility.md](docs/compatibility.md) — tested dependency versions
- [AGENTS.md](AGENTS.md) — contribution and upstream-alignment rules

## Validate

```bash
make test
make test-suite
```

Gateway lifecycle checks are available through `make test-local`,
`make test-kind`, and `make test-remote`. Provider-capability checks require
platform-provisioned credentials.
