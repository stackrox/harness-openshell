# harness

> **Experimental.** Harness runs trusted repository workflows in isolated
> [OpenShell](https://github.com/NVIDIA/OpenShell) sandboxes.

OpenShell is alpha software and both projects may change quickly. The binary
is still named `harness`; the product direction is a small workflow bridge,
not a second OpenShell implementation.

## The product boundary

The first supported workflow archetype is `pr-reviewer-with-comments`: review
one exact pull-request diff in an isolated sandbox and optionally publish
inline comments that the workflow skill has validated. The repository that uses
the workflow supplies the skill and review criteria. Harness supplies the
trusted execution contract.

Harness earns its place when it removes repeated credential, lifecycle, and CI
integration code. If a repository can run a native OpenShell workflow with the
same safety and less bookkeeping, use the native workflow instead.

### What belongs where

| Concern | Owner |
|---|---|
| Gateway provisioning, sandbox isolation, policy enforcement, provider proxying and credential masking | OpenShell or HyperShell |
| Provider registration and platform bootstrap | OpenShell/platform integration; a trusted adapter may create an ephemeral provider |
| Event, label, draft, permissions, trusted checkout, concurrency, approvals, and branch protection | GitHub Actions |
| Workflow loading, target resolution, source/payload staging, bounded execution, freshness checks, output validation, and cleanup | Harness and the workflow adapter |
| Task behavior, review criteria, trusted skills, and what to do with the result | Consuming repository |
| Coding agent and inference model | Workflow configuration and the consuming repository |

Harness is not a credential store, provider manager, policy language, scheduler,
or general-purpose StackRox automation suite. OpenShell remains authoritative
for gateways, policies, providers, and sandbox enforcement.

### The access-pattern model

Use these terms consistently when adding workflows:

```text
workflow archetype = what the agent may access or mutate
skill              = task-specific behavior and judgment
agent configuration= coding agent and inference choice
policy/provider    = sandbox, network, and credential boundary
Harness            = trusted execution and lifecycle bridge
```

The name of an archetype describes its access and output contract, not the
selected coding agent. Codex, OpenCode, and Claude are replaceable runtime
choices.

Currently supported:

- `pr-reviewer-with-comments` — read a fixed PR and write only the explicitly
  allowed review comments.

Future archetypes are deliberately not promised yet: `repo-observer`,
`issue-triager`, `issue-to-pr-creator`, `pr-fixer`, `ci-watcher`,
`security-reviewer`, and `auto-merge-gate`. Each would need a separate
mutation contract and approval boundary.

## Use it from GitHub Actions

The reusable workflow is the intended cross-repository integration. Pin both
references to the same immutable 40-character Harness commit SHA:

```yaml
name: AI review

on:
  pull_request_target:
    types: [opened, labeled, unlabeled, synchronize, reopened,
            ready_for_review, converted_to_draft, closed]

permissions:
  contents: read
  pull-requests: write

jobs:
  ai-review:
    uses: stackrox/harness-openshell/.github/workflows/pr-review-reusable.yml@<40-character-harness-sha>
    with:
      harness-ref: <same-40-character-harness-sha>
      skill-path: .github/skills/pr-review/SKILL.md
      allow-draft-reviews: false
    secrets: inherit
```

The called workflow checks out the caller repository's default branch and reads
`skill-path` from that trusted checkout. The pull-request head is fetched as
data; it is never checked out as workflow code. The `ai-review` label is an
explicit opt-in and is not added automatically. Removing it prevents future
runs. Draft pull requests run only when the caller opts into
`allow-draft-reviews: true` and the label is present.

The caller repository must configure:

| Setting | Kind | Purpose |
|---|---|---|
| `VERTEX_AI_PROJECT_ID` | Repository variable | Vertex project used by the inference provider |
| `VERTEX_AI_REGION` | Repository variable | Vertex region |
| `VERTEX_AI_SERVICE_ACCOUNT_KEY` | Repository secret | Trusted GitHub Actions bootstrap credential |

No manually created `GITHUB_TOKEN` secret is needed. GitHub's automatic token
is available only to trusted host-side bootstrap code, which registers the
native OpenShell GitHub provider. The token is not placed in the sandbox
environment or agent payload.

`pull_request_target` is the production trigger for a workflow that receives
secrets or write permissions. A `pull_request` trigger is suitable only for a
credential-free demonstration and must not be merged while it can execute
pull-request-controlled workflow code with gateway, Vertex, or GitHub write
credentials.

## Credential and policy model

Provider names in a workflow are references, not credential definitions or
permission grants. The attached OpenShell provider profile and policy determine
which endpoints and mutations are available.

The credential path is:

1. A trusted platform or workflow adapter registers a provider with the
   gateway. It may briefly read a host-side credential such as GitHub's
   automatic token or a short-lived Vertex token.
2. The sandbox attaches the named provider.
3. OpenShell exposes a proxy-backed, masked interface to authorized requests;
   the raw credential remains gateway/provider managed.

Raw credentials must never appear in workflow YAML, `spec.sandbox.env`,
payload files, agent arguments, logs, artifacts, structured `-o json`/`-o yaml`
output, or model prompts. Ordinary environment variables are for non-secret
workflow inputs only. Credential refresh material remains outside the sandbox.
See OpenShell's [provider and credential injection
documentation](https://docs.nvidia.com/openshell/sandboxes/manage-providers)
for the gateway-side masking model.

GitHub Actions owns event and permission checks. The review adapter rechecks the
PR label, base, and head immediately before execution and publication, stages
the diff as data, validates agent output, and cleans up the sandbox and
temporary workspace. OpenShell enforces the filesystem, process, network, and
provider credential boundary. These checks are duplicated only where a race can
occur between GitHub scheduling and sandbox execution.

## Workflow contract

The canonical `v1alpha1` document is intentionally small:

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
    image: quay.io/example/reviewer:v1
    providers: [github-read]
    keep: false
    tty: false
  payloads:
    - source: skills/review/SKILL.md
      destination: /sandbox/skills/review/SKILL.md
  agent:
    type: claude
    args: [--print, "Review the supplied repository input"]
  source:
    repo: https://github.com/stackrox/stackrox
    ref: main
    destination: /sandbox/stackrox
```

Providers are existing gateway capabilities. `management: referenced` does not
create or update a provider. The policy file, provider profile, and attached
provider names are resolved by OpenShell; Harness does not invent a second
policy schema.

Target resolution follows this order:

1. explicit flags (`--gateway`, `--workspace`);
2. `OPENSHELL_*` environment variables;
3. workflow configuration;
4. OpenShell's active gateway selection.

`plan` is read-only and may render desired state while the gateway is offline.
`apply` verifies the effective target and referenced providers before creating a
sandbox. Source repositories are prepared outside the sandbox and uploaded;
OpenShell sandboxes do not use host mounts by design.

The execution lifecycle is:

```text
load workflow
  -> resolve target and provider references
  -> prepare source, payloads, and policy
  -> create isolated sandbox
  -> run the selected agent under the workflow adapter's deadline
  -> validate result and recheck freshness
  -> return or publish the workflow result
  -> clean up sandbox and temporary resources
```

For a machine-readable completion record, use:

```bash
harness apply -f workflow.yaml --result-file result.json
```

The result records lifecycle completion, status, phase, timing, and the
prepared source commit. It is not a review-quality assertion or an independent
security audit; authorization comes from OpenShell policy and provider scope.

## Run locally

Install the OpenShell version pinned in `.openshell-version`, then register and
select a gateway:

```bash
make openshell
openshell gateway add https://127.0.0.1:17670 --local --name openshell
openshell gateway select openshell
```

Or target a configured HyperShell gateway through the normal OpenShell target
and OIDC environment variables. The core Harness CLI does not discover or
manage local provider credentials; configure providers through OpenShell or a
platform bootstrap path.

The basic local loop is:

```bash
harness init
harness doctor -f harness.yaml
harness plan -f harness.yaml
harness apply -f harness.yaml
harness apply -f harness.yaml --attach
```

For retained sandboxes, use OpenShell directly:

```bash
openshell sandbox connect <name>
openshell sandbox exec <name> -- <command>
openshell sandbox logs <name>
openshell policy get <name>
openshell term
```

`openshell term` shows policy decisions while an agent is running. Provider
references do not imply that an agent can push, comment, label, or merge; those
mutations must be allowed by the provider profile and OpenShell policy.

## Commands

| Command | Purpose |
|---|---|
| `harness init` | Generate a starter workflow |
| `harness doctor` | Check target reachability and referenced providers |
| `harness plan -f FILE` | Render a read-only reconciliation plan |
| `harness apply -f FILE` | Run the workflow |
| `harness apply -f FILE --setup-only` | Verify references and configure inference without running a sandbox |
| `harness get gateways\|agents\|providers` | Inspect identity-only resources (`-o table\|json\|yaml`) |
| `harness describe NAME` | Inspect a sandbox |
| `harness delete NAME` | Delete a sandbox |

Structured list/get output supports `-o table`, `-o json`, and `-o yaml`.
Credential values are never serialized in JSON or YAML output; only provider
identity and key names may be shown.

## Testing and development

Fast, credential-free checks:

```bash
make test
make test-suite
```

Gateway lifecycle checks are available with `make test-local`, `make test-kind`,
and `make test-remote`. CI uses the credential-free mode for local and Kind
lifecycles; provider capability checks require platform-provisioned credentials.
See [AGENTS.md](AGENTS.md) for the complete validation matrix and contribution
rules.

## Repository documentation

- [AGENTS.md](AGENTS.md) — coding rules, architecture constraints, and validation
- [docs/ci.md](docs/ci.md) — trusted CI bootstrap and credential contract
- [docs/compatibility.md](docs/compatibility.md) — tested OpenShell, ACP, and Go versions
- [profiles/README.md](profiles/README.md) — profile layout and examples
- [examples/github-pr-reviewer/](examples/github-pr-reviewer/) — the current
  `pr-reviewer-with-comments` workflow inputs and policy
