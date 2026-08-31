# Implementation plan

This plan turns harness-openshell into a repository workflow runner for centrally
managed HyperShell/OpenShell gateways. Each phase should be independently
reviewable and leave the repository in a working state.

## Success definition

A repository owner can add a short security-review declaration and reusable
GitHub workflow caller. The job authenticates to the ACS gateway without carrying
inference credentials, runs against the repository's authorized workspace, fails
if a declared capability is missing, and publishes structured findings plus an
evidence manifest.

## Phase 0: merge source isolation

Land `rc-pr6-source-hardening` after final review; it is already based on the
reviewed main revision.

Acceptance criteria:

- Distinct repositories with the same basename use distinct mirrors.
- Concurrent runs never share a mutable checkout.
- The uploaded checkout has no credential-bearing remote URL.
- Submodule configuration does not persist credentials.
- Checkouts are removed after success, failure, and cancellation.
- The resolved source commit can be returned to the run layer.

## Phase 1: one canonical execution API

Make `harness.openshell.dev/v1alpha1` drive both plan and execution. Introduce one
resolved, immutable run object and one action graph:

```text
parse → resolve → validate → plan → execute → result
```

Acceptance criteria:

- `harness plan -f workflow.yaml` and `harness apply -f workflow.yaml` consume the
  same parsed and resolved structure.
- Apply executes the exact action graph rendered by plan.
- Gateway, workspace, source, providers, sandbox, agent arguments, payloads, and
  retention are all honored.
- `harness migrate` output can be supplied directly to apply.
- Legacy input has a documented removal window and no new features are added to
  it.
- `desiredFromAgent` is removed when the migration window closes.

## Phase 2: shared-gateway authority and capabilities

Separate administrator reconciliation from ordinary workflow execution.

Acceptance criteria:

- Run mode accepts referenced providers/capabilities only.
- Run credentials cannot create, adopt, update, or delete providers or inference
  routes.
- Gateway and workspace are resolved once and passed to every SDK and CLI call.
- Missing capabilities fail before sandbox creation.
- OpenShell automatic provider discovery is disabled for declared workflows.
- A separate, clearly named administrator path owns any remaining managed-resource
  reconciliation, or that work is delegated entirely to HyperShell/operator APIs.

Initially, logical capabilities may map to provider names through a small
workspace-owned configuration. Do not build a new catalog service before the
pilot proves that a static mapping is insufficient.

## Phase 3: deterministic agent execution

Complete the adapter boundary and OpenShell compatibility contract.

Acceptance criteria:

- Claude, Codex, and OpenCode have native argv construction and focused tests.
- Codex headless mode uses `codex exec`, not Claude's `--print` convention.
- `agent.args` is passed as argv without shell interpolation.
- Only transient sandbox-creation failures are retried.
- CI tests the pinned OpenShell release and newest supported release.
- Upload-plus-command behavior is compatible with each supported OpenShell
  version or implemented as an explicit multi-step lifecycle.

## Phase 4: structured run results

Define a stable `RunResult` envelope and a security-review finding schema.

The result should include:

- run and workflow identifiers;
- repository and resolved commit;
- workflow, skill, policy, and image digests;
- gateway/workspace and OpenShell versions;
- resolved capability/provider names without secrets;
- agent type and model;
- timestamps, exit status, and cleanup status; and
- typed artifacts such as SARIF, Markdown summary, patch, or test report.

Acceptance criteria:

- Results can be emitted as JSON without scraping terminal output.
- Agent failure, policy denial, infrastructure failure, and invalid workflow are
  distinguishable outcomes.
- Secrets cannot be serialized into results.
- Rerunning the same immutable inputs produces an equivalent resolved-run record.

## Phase 5: packaged skills

Treat `SKILL.md` inputs as versioned packages rather than prompt files.

Acceptance criteria:

- The selected skill directory and required referenced assets are uploaded.
- A deterministic package digest is recorded.
- The package declares required capabilities, tools, and network access.
- Catalog references resolve to an immutable version or digest.
- Local paths remain available for skill development.

## Phase 6: ACS reusable GitHub workflow

Publish a centrally versioned reusable workflow for a read-only security review.

Acceptance criteria:

- GitHub OIDC or another short-lived exchange authenticates the job to the ACS
  gateway; inference credentials never enter repository secrets.
- Repository identity is authorized to exactly one intended workspace.
- GitHub access is repository-scoped and read-only except for publishing the
  review result.
- Findings appear as a check, PR summary, or SARIF result.
- The complete evidence manifest is retained as an artifact.
- A repository can adopt it with a small workflow caller and one harness
  declaration.

Do not start with automatic code changes. Write-capable remediation is a separate
workflow, identity, and approval decision.

## Phase 7: pilot and one-file adoption

Pilot in one ACS repository, then expand to two repositories with different build
systems and separate workspaces.

Measure:

- time from copied example to first successful run;
- successful-run and infrastructure-failure rates;
- useful findings and false positives;
- runtime and inference cost;
- rerun determinism; and
- opt-out or suppression reasons.

After the workflow earns trust, use an organization-required workflow to discover
the repository declaration automatically. That reduces adoption from two files to
one without hiding the repository's declared capabilities and policy.

## Deferred deliberately

- A new hosted workflow/catalog control plane
- Write-capable autonomous remediation
- Scheduled self-improvement agents
- Broad observability integrations beyond the run-result contract
- OpenShell plugin packaging
- Convenience wrappers around additional native OpenShell commands

These may become useful, but none is required to prove the ACS security-review
adoption loop.
