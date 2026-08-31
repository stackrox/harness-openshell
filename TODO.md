# Roadmap

The harness is a workflow layer for OpenShell gateways that already exist. Its
next milestone is a repository-declared workflow that can run unchanged against a
local gateway or a centrally managed HyperShell gateway.

Completed work belongs in `CHANGELOG.md`; this file describes active and planned
work only.

## P0: source isolation

- [ ] Complete final review and merge `rc-pr6-source-hardening`.
- [ ] Verify uploaded Git metadata contains no credential-bearing remote.
- [ ] Verify submodule authentication cannot persist credentials in the upload.
- [ ] Return the resolved source commit to the run-result layer.
- [ ] Clean per-run checkouts after success, failure, and cancellation.

## P0: canonical workflow execution

`harness plan` currently consumes `harness.openshell.dev/v1alpha1`, while
`harness apply` consumes the legacy `agent.AgentConfig` model. Until both commands
share one resolved object and action graph, plan is not a reliable preview of
apply and migration output is not an executable workflow.

- [ ] Make v1alpha1 the input to plan and apply.
- [ ] Resolve defaults, environment references, gateway, workspace, and CLI
      overrides once.
- [ ] Build one action graph; render it in plan and execute it in apply.
- [ ] Honor v1alpha1 source, sandbox, agent arguments, payloads, and retention.
- [ ] Ensure `harness migrate` output can be passed directly to apply.
- [ ] Publish a bounded legacy-config removal window.
- [ ] Remove `desiredFromAgent` after migration.

## P0: shared-gateway authority

A repository workflow running on a centrally managed gateway must consume
platform-owned capabilities. It must not create, adopt, update, or delete shared
providers or inference routes.

- [ ] Separate administrator reconciliation from ordinary workflow execution.
- [ ] Support explicit gateway and workspace selection in apply.
- [ ] Resolve the target once and pass it to every SDK and CLI operation.
- [ ] Resolve logical capabilities such as `github-read` and
      `inference-default` to referenced providers in the selected workspace.
- [ ] Fail before sandbox creation when a required capability is unavailable.
- [ ] Disable OpenShell automatic provider discovery for declared workflows.
- [ ] Make best-effort behavior explicit rather than the default.
- [ ] Make `--setup-only` fail when desired state was not achieved.

## P1: deterministic execution

- [ ] Give Claude, Codex, OpenCode, and custom agents separate argv builders.
- [ ] Use native `codex exec` for headless Codex execution.
- [ ] Honor `agent.args` without shell interpolation.
- [ ] Retry only transient sandbox-creation failures.
- [ ] Define and test an OpenShell compatibility window.
- [ ] Test both the pinned and newest supported OpenShell release in CI.
- [ ] Adapt the upload/run lifecycle to OpenShell versions where `--upload`
      conflicts with a trailing command.
- [ ] Move secret provider-refresh material from process arguments to
      environment-backed OpenShell options.

## P1: structured run results

- [ ] Define a stable JSON `RunResult` envelope.
- [ ] Record source, workflow, skill, policy, image, agent, model, target, and
      OpenShell version provenance without secrets.
- [ ] Distinguish invalid workflow, missing capability, policy denial, agent
      failure, and infrastructure failure.
- [ ] Define typed artifact references for SARIF, Markdown summaries, patches,
      and test reports.
- [ ] Make cleanup and retention outcomes visible in the result.

## P1: packaged skills

- [ ] Treat a `SKILL.md` input as a package with referenced scripts, templates,
      instructions, and assets rather than only task text.
- [ ] Record a deterministic skill-package digest.
- [ ] Declare required capabilities, tools, and network access.
- [ ] Resolve catalog references to immutable versions or digests.
- [ ] Retain local-path support for skill development.

## P1: ACS HyperShell pilot

- [ ] Publish a reusable read-only security-review GitHub workflow.
- [ ] Authenticate GitHub Actions to the gateway with short-lived identity.
- [ ] Map repository identity to an authorized workspace.
- [ ] Keep inference credentials out of repository and organization secrets.
- [ ] Publish findings on the pull request and retain a complete evidence
      artifact.
- [ ] Pilot in one repository, then two repositories with different build
      systems and separate workspaces.
- [ ] Measure setup time, reliability, finding usefulness, false positives,
      runtime, and inference cost.
- [ ] After the pilot earns trust, use an organization-required workflow to
      reduce repository adoption from two files to one declaration.

## Upstream issues to track

- #1719 -- Kubernetes Operator design
- #1851 -- Plugin system and host contract
- #1922 -- Portable sandbox log collection
- #1933 -- Centralized audit/event log

Provider lifecycle and sandbox declaration may move into upstream operator
resources. Keep reconciliation behind replaceable interfaces; the durable harness
contract is workflow intent, capability verification, promotion, and evidence.

## Deliberately deferred

- Broad observability integrations beyond the run-result contract
- A new hosted catalog/control plane
- Write-capable autonomous remediation
- Scheduled self-improvement agents
- OpenShell plugin packaging
- Additional wrappers around native OpenShell commands
