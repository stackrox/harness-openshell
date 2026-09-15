# harness-openshell

Repository automation through OpenShell sandboxes.

`harness-openshell` is a helper repository for connecting repository workflows
to [OpenShell](https://github.com/NVIDIA/OpenShell) gateways. It provides
reusable GitHub Actions workflows, task bundles, sandbox images, and a small
`harness` CLI that assembles and runs a task in an isolated sandbox.

Agents can produce artifacts and perform authorized GitHub operations from
inside the sandbox. Trusted setup supplies a GitHub App token scoped to the
target repository and required permissions. OpenShell holds the provider
credential and mediates GitHub REST requests using a task-specific policy.

Repository workflows can use a HyperShell-managed gateway through the existing
OpenShell SDK connection. The reusable reviewer selects that path when the
caller configures `OPENSHELL_GATEWAY_ENDPOINT`; platform setup supplies workspace
access, providers, and the v0.0.109 `inference.local` route. Without managed
connection settings, [`setup-openshell`](.github/actions/setup-openshell/action.yml)
and the [local wrapper](scripts/pr-review-local.sh) prepare temporary CI resources.
[`pr-review.sh`](scripts/pr-review.sh) prepares and runs the same review task.

## What an agent can do

| Task bundle | Allowed GitHub operation | Integration status |
|---|---|---|
| [PR reviewer](tasks/github-pr-reviewer/) | Read the selected PR and post inline comments to it | Used by the reusable review workflow |
| [PR merger](tasks/github-pr-merger/) | Read the selected PR and head checks, then request a merge under a separate approval and credential contract | Opt-in bundle; consumer enablement and live validation are separate steps |

For review, trusted setup obtains a repository-scoped GitHub App installation
token with `Contents: read` and `Pull requests: write`. The
[OpenShell REST policy](tasks/github-pr-reviewer/openshell/policy.yaml)
further restricts sandbox requests to the chosen PR's read endpoints and inline
comment `POST` endpoint. The agent uses `gh api`; OpenShell's network proxy
enforces the permitted host, HTTP methods, and paths.

The token is scoped to a repository and permissions, while the policy narrows
requests to a specific PR. The instruction to post at most three comments is
agent behavior; the REST policy does not enforce a comment quota or finding
quality. Review and merge use separate provider and policy boundaries.

External actions happen during execution. Artifacts are files and diagnostics
retained from the run; the workflow's `outputs` field declares sandbox paths to
download. Sandbox cleanup does not undo a posted comment or a completed merge,
and a failed or cancelled run may already have performed permitted actions.

## How a repository run works

`harness-openshell` packages the integration around OpenShell. The `harness`
CLI is its generic composition and sandbox lifecycle component. A **task
bundle** combines agent instructions,
an image, native OpenShell policy, provider references, and payloads. A
**harness workflow document** declares a run; a **GitHub Actions workflow**
supplies its CI trigger and trusted host setup.

```text
Repository workflow or local caller
  selects trusted task inputs and allowed operation
                    |
                    v
               harness CLI
  composes inputs and manages sandbox lifecycle
                    |
                    v
          OpenShell-managed sandbox
             agent runs the task
               /           \
              v             v
       files and logs     GitHub REST request
              |             |
              v             v
      retained artifacts  OpenShell network proxy
                          provider credential + REST policy
                              |
                              v
                         GitHub API
                    permitted read or mutation
```

Trusted setup establishes gateway access and provider credentials before the
task runs. The diagram shows the request flow; OpenShell owns sandbox isolation,
inference routing, credential handling, and network policy enforcement.

## Use the reusable PR reviewer

The cross-repository interface is a reusable GitHub Actions workflow for a
defined operation with fixed permissions and trusted inputs. The reviewer
selects the `github-pr-reviewer` task bundle; it does not accept arbitrary
image, policy, provider, or command inputs.

Configure the GitHub App installation and Vertex credentials described in
[docs/ci.md](docs/ci.md#label-driven-pr-review), then call the reviewer from a
trusted `pull_request_target` workflow:

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

This is the job portion of the caller workflow. Pin both references to the same
immutable commit. The reviewer checks out trusted workflow code from the
caller's default branch and stages the pull-request diff only as untrusted
data. The `ai-review` label is explicit opt-in. See
[the reusable workflow guidance](.github/workflows/README.md) for its boundaries.

## Gateway setup: local CI and managed deployment

| Component | Current reviewer responsibility |
|---|---|
| [Reusable workflow](.github/workflows/pr-review-reusable.yml) | Trusted checkout, job permissions, App token, and setup/execution steps |
| [`setup-openshell`](.github/actions/setup-openshell/action.yml) | Invoke the installer for the pinned OpenShell CLI release and wait for gateway readiness |
| [`scripts/pr-review-local.sh`](scripts/pr-review-local.sh) | Create temporary workspace/providers, configure inference, run the review, and remove its setup resources |
| [`scripts/pr-review.sh`](scripts/pr-review.sh) | Stage the diff, check PR eligibility, render the PR policy, invoke the CLI, and validate the output |
| [`harness` CLI](runner/) | Compose the task and manage its sandbox lifecycle |

The managed path authenticates to an existing gateway with a service account
and creates only the task sandbox. The platform owns workspace membership,
provider credentials and refresh, and the matching inference route. The local
path uses the setup action and wrapper above.

Configure the caller's connection variables and secret using the
[managed reviewer instructions](docs/ci.md#managed-reviewer-transition).
The selected Linux runner must reach both the gateway and its OIDC issuer;
use a runner on the Red Hat network when the issuer is private. The integration
target is HyperShell's OpenShell v0.0.109 deployment. Local CLI and SDK pins
remain unchanged; the exact SDK/server combination still requires live validation.

## Run a task locally

Install the pinned OpenShell CLI and build the `harness` CLI:

```bash
make openshell
make cli
```

Before applying a task, ensure its gateway is reachable, its provider instances
exist, and its inference route and policy are configured. Select an existing
local gateway with the native CLI, or configure a direct managed target as
described in [docs/ci.md](docs/ci.md#workflow-contract).

For an existing local gateway listening at this address:

```bash
openshell gateway add https://127.0.0.1:17670 --local --name openshell
openshell gateway select openshell
```

Choose a task from [tasks/](tasks/) and prepare its documented inputs.
The [PR reviewer](tasks/github-pr-reviewer/) includes the native OpenShell
inputs; [docs/ci.md](docs/ci.md#label-driven-pr-review) gives the trusted wrapper
commands for running it locally. For your own prepared workflow document:

```bash
./harness workflow plan workflow.yaml
./harness workflow apply workflow.yaml
```

`workflow.yaml` is the document you supply. The
[workflow format reference](docs/workflow-format.md) shows the schema and
explains provider references, payloads, and downloaded outputs.

Use `--attach` for the same task with a connected terminal. Set
`sandbox.keep: true` only for post-run debugging with native
`openshell sandbox` commands. Normal execution creates one sandbox, runs the
agent, collects declared output files, and deletes the sandbox.

## Ownership and security boundaries

| Component | Owns |
|---|---|
| Consuming repository | Opt-in triggers, trusted task inputs, review criteria, and approval rules |
| [.github/workflows/](.github/workflows/) | Repository CI and reusable jobs with fixed permissions, trusted checkout, concurrency, and task selection |
| [.github/actions/setup-openshell/](.github/actions/setup-openshell/action.yml) | OpenShell installation and gateway readiness for the current local CI path |
| [scripts/pr-review.sh](scripts/pr-review.sh) | Trusted review preparation, execution, and output validation |
| [scripts/pr-review-local.sh](scripts/pr-review-local.sh) | Temporary workspace/provider bootstrap and teardown for local CI |
| [tasks/](tasks/) | Task instructions, policy, provider references, image selection, payloads, and outputs |
| [runner/](runner/) | Generic `plan`/`apply` composition and sandbox lifecycle |
| [images/](images/) | Reusable runtime toolchains |
| Platform administration | Managed gateway access, workspace membership, provider credential lifecycle, and inference configuration |
| OpenShell | Gateway resources, credential-backed proxies, inference routing, policy enforcement, and sandbox isolation |

The `harness` CLI verifies provider references and asks OpenShell to attach
their masked proxy interfaces. Provider provisioning belongs to trusted setup
or platform administration. The CLI has no provider-management service,
durable workflow database, scheduler, release history, or rollback mechanism.

- Workflow files, policies, payload declarations, and reusable-workflow code
  are trusted host-side inputs. Never execute versions supplied by an
  untrusted pull request in a credentialed context.
- Provider fields contain names only. Raw credentials must not appear in
  workflow YAML, sandbox environment values, payloads, agent arguments,
  logs, prompts, or artifacts.
- Provider attachment supplies credential-backed proxy access; native OpenShell
  policy separately controls which requests the sandbox may make.
- GitHub Actions owns run state, labels, artifacts, concurrency, and approvals.
  OpenShell and the platform own gateway runtime resources.

The PR review task consumes `inference.local`; setup owns its configuration.
The local wrapper configures the route in its temporary workspace, while a
managed platform supplies the matching route before review execution. Other
workflow documents can still explicitly request inference reconciliation. See
[docs/ci.md](docs/ci.md) for the credential and setup contract.

## CLI

| Command | Purpose |
|---|---|
| `harness workflow plan FILE` | Preview the configured run |
| `harness workflow apply FILE` | Execute one task headlessly |
| `harness workflow apply FILE --attach` | Execute with an attached terminal |
| `harness workflow apply FILE --output-dir DIR` | Download declared outputs below `DIR` |
| `harness workflow apply FILE --setup-only` | Verify references and reconcile inference without starting a sandbox |

`plan` and dry-run output support `-o table|json|yaml`. Interpolated values are
redacted from display output; authors must keep credential values out of
workflow inputs. Use `--result-file` for a host-derived execution result.
Use the native OpenShell CLI for provider administration, gateway operations,
and sandbox inspection.

## Documentation and validation

- [runner/](runner/) — implementation and lifecycle of the `harness` CLI
- [docs/workflow-format.md](docs/workflow-format.md) — version 1 workflow contract
- [docs/ci.md](docs/ci.md) — trusted CI setup and managed deployment requirements
- [docs/compatibility.md](docs/compatibility.md) — tested dependency versions
- [AGENTS.md](AGENTS.md) — contribution and upstream-alignment rules

```bash
make test
make test-suite
```

Gateway lifecycle checks are available through `make test-local`,
`make test-kind`, and `make test-remote`. Provider-capability checks require
configured credentials. A passing lifecycle check establishes execution and
cleanup behavior; live GitHub effects, inference, and consumer integrations
need their corresponding validation.
