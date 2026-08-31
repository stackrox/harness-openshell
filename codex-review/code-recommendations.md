# Code recommendations

## P0: establish one truthful workflow API

1. Make `harness.openshell.dev/v1alpha1` the input to both `plan` and `apply`.
2. Resolve defaults, environment references, target selection, and overrides once.
3. Build one action graph from that resolved object; let `plan` render it and
   `apply` execute it.
4. Retire `desiredFromAgent` and the legacy parser after a bounded migration
   period.
5. Consider naming the durable resource `AgentRun` or `WorkflowRun` rather than
   the generic `Harness`.

Until this is complete, a successful plan is not a reliable preview of apply.

## P0: separate platform authority from workflow authority

A shared HyperShell gateway changes the provider ownership model. Split operations
into two explicit authorities:

- Platform/bootstrap authority provisions gateways, workspaces, providers,
  inference routes, policy baselines, images, and service accounts.
- Workflow/run authority verifies referenced capabilities, creates a sandbox,
  executes the agent, and publishes results.

Ordinary repository execution must never create, adopt, update, or delete a shared
provider. The normal CI declaration should reference logical capability names and
fail when they cannot be resolved. Any administrator reconciliation command must
be distinct from `run` and require separate credentials.

## P0: make execution deterministic and fail closed

1. Treat missing required providers, provider reconciliation errors, and
   inference reconciliation failures as fatal before sandbox creation.
2. Make best-effort behavior an explicit `--best-effort` option.
3. Pass `NoAutoProviders: true` by default so the runtime cannot silently attach
   undeclared capabilities.
4. Make `--setup-only` return a failure when desired state was not achieved.
5. Retry only failures classified as transient. Do not retry invalid arguments,
   authentication failures, or unsupported CLI behavior five times.

## P0: make gateway and workspace identity unambiguous

PR7b removed gateway deployment and now resolves one selected gateway for apply.
Preserve this bring-your-own-gateway boundary; do not reintroduce deployment as a
harness backend.

Complete the work by resolving one `(gateway registration, workspace)` target and
passing it to every CLI and SDK operation. Workspace selection cannot remain the
ambient default when multiple repositories or trust zones share one logical ACS
gateway. HyperShell, Helm, a local process, and an operator should remain
interchangeable ways to supply the registered target.

## P1: complete real agent adapters

Adapters should produce argv directly and own the semantics of each supported
agent:

- Claude headless execution may use `--print`.
- Codex headless execution should use `codex exec`; it must not inherit Claude's
  `--print` behavior.
- OpenCode should use its native `run` form.
- Schema `agent.args` must be honored without embedding untrusted values in a
  shell string.

If an intentionally permissive agent mode is offered, express it in the workflow
and adapter, and rely on OpenShell as the outer security boundary. Do not silently
enable it as an adapter default.

## P1: define an OpenShell compatibility contract

The current exact 0.0.110 pin protects the build but does not establish forward
compatibility. In newer OpenShell source, `sandbox create --upload` conflicts with
a trailing command, while the current harness lifecycle supplies both.

1. State an explicit tested compatibility window.
2. Test the pinned release and newest supported release in CI.
3. Prefer structured capability discovery where possible.
4. Keep CLI-dependent behavior behind a narrow backend adapter.
5. Use the Go SDK for structured state operations and the CLI only where it owns
   important native behavior such as builds, uploads, TTY, or authentication.

## P1: secure credential transport

Google Workspace refresh secrets are redacted from harness logging, but currently
travel in child-process arguments. Use OpenShell's environment-backed secret
material option when the supported version provides it. Continue keeping secrets
out of the desired-state and evidence models.

Review ordinary sandbox environment fields separately: configuration values that
look like credentials should normally become provider capabilities or secret
references rather than `--env` arguments.

## P1: make skills deployable packages

`--task @SKILL.md` should package the selected skill, not only copy its prompt
text. Resolve and upload the skill directory closure, including directly
referenced instructions, scripts, templates, and assets. Record a deterministic
digest and declare prerequisites such as providers, tools, and network policy.

Prefer the existing `SKILL.md` convention rather than introducing a competing
skill format.

## P1: add a shared-gateway capability contract

The workflow should request logical capabilities such as `github-read` and
`inference-default`, not provider implementation names or credential sources. A
platform-owned mapping resolves those names within the selected workspace.

The resolved run must record the concrete provider names and versions without
exposing credentials. Capability resolution should happen before sandbox creation
and should never silently reduce the declared set.

## P1: produce a run result and evidence manifest

Every run should produce a machine-readable result containing at least:

- run ID and workflow API version;
- repository, commit SHA, ref, and dirty-state status;
- workflow and skill digests;
- policy and image digests;
- OpenShell client and gateway versions;
- agent type and model;
- attached provider names, never secret values;
- start/end timestamps and exit status; and
- declared output artifacts such as patches, findings, or test reports.

This evidence is a major durable differentiator from invoking the OpenShell CLI
directly.

## P2: complete source and lifecycle hardening

PR6 implements URL-hashed bare mirrors, locks shared mirror updates, and creates
isolated self-contained checkouts for each run. Merge that work, then verify:

1. The uploaded `.git` directory has no credential-bearing remote URL.
2. Submodule authentication cannot persist secrets in uploaded configuration.
3. The resolved commit SHA is recorded in run evidence.
4. Per-run checkouts are removed after cancellation and failure as well as success.
5. Retention is configurable through the canonical schema rather than always
   retaining legacy apply sandboxes.
6. Cancellation and cleanup behavior is explicit in the run result.
7. Commodity `get`, `describe`, and `delete` wrappers remain thin where
   the native OpenShell CLI already offers the better interface.

## Suggested implementation order

1. Merge PR6 source hardening.
2. Unify plan/apply configuration and workspace-aware target resolution.
3. Separate platform reconciliation from referenced-capability execution.
4. Enforce strict capability verification and disable auto-providers.
5. Fix the agent adapters and OpenShell compatibility boundary.
6. Add a structured run result and security finding schema.
7. Add the reusable ACS/HyperShell GitHub workflow.
8. Add packaged skills and an immutable shared catalog.
