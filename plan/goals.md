# Goals and execution plan

Updated: 2026-09-08. Execution order approved by the user: **4 → 1 → 2**.
Goal 3 remains gated on execution and attribution evidence.

## North star

Make a bounded repository task portable, attributable, safe to run, and
demonstrably useful—with less custom machinery than maintaining equivalent
scripts independently. Keep platform provisioning outside the harness and
remove custom adapters when OpenShell makes them redundant.

## Decisions

- Start with locally installed OpenShell on the developer machine and GitHub
  runner. HyperShell is an additional target, not a prerequisite.
- Use Vertex service-account credentials first, with existing GitHub secrets.
  Bootstrap outside Harness documents; keep credentials out of sandboxes and
  artifacts. Scope keys and establish rotation ownership. WIF is deferred.
- Separate runtime smoke, attributable results, and useful AI judgment.
- Begin artifact-only. Any later publication uses trusted deterministic code
  with write authority outside the reviewing model's sandbox.
- Treat repository content as untrusted data. Prompts do not enforce permissions.
- Merged implementation and green credential-free CI do not prove live inference.

## Baseline

At review, main `cf9fc1f` included #138 and #140, with CI, Integration, and Images
green. The SDK runner already propagates cleanup errors and attempts cleanup
after cancellation. HyperShell M1/M2 have recorded successful validation; M3 is
deferred. The reviewer fixture is merged but lacks accepted live execution and
skips in CI. Vertex bootstrap tooling exists; the inspected manual workflow had
no run history. Dependency PRs #125/#126 fail image publication authentication.

## Goal 4 — Maintenance and planning truth (first)

Status: in progress. Implementation: Codex; acceptance review: repository owner.

- Make this the current execution roadmap; mark older directions as superseded
  without erasing historical validation evidence.
- Build images on PRs without registry publication credentials. Publish and
  export registry caches only from trusted main/tag pushes.
- Revalidate dependency PRs #125/#126 after the workflow correction lands.
- Report ordinary PR validation separately from credentialed live validation.
- Establish a release/compatibility checkpoint after Goal 1 is accepted.

Acceptance: workflow checks pass; PR builds cannot log in, publish images, or
write shared registry caches; dependency checks are rerun; current planning has
clear status and evidence. Release preparation remains gated on Goal 1.

## Goal 1 — Live local and GitHub workflow (second)

Status: queued. Implementation: Codex; credential ownership: repository owner.

- Complete #140 using the existing service-account bootstrap shape.
- First prove the existing OpenCode/Gemini smoke end to end. Gemini 3.8 Flash
  passed endpoint verification in `acs-ai-677887`; Claude Haiku did not. This
  runtime proof is not acceptance of the Claude reviewer or review quality.
- Verify and pin the image, installed agent/tools, and explicit inference route.
- Use isolated test-owned resources; never alter shared inference for a smoke.
- Run the same checked-in task locally and in trusted GitHub automation, changing
  only target and identity inputs. Missing required credentials must fail the
  credentialed test, not appear as a passing skip.
- Test success, nonzero agent exit, malformed output, and cancellation. Verify
  cleanup and expose failures rather than merely requesting deletion.

Acceptance: linked successful local/GitHub real-inference evidence, verified
runtime assumptions, negative-path coverage, no credential exposure, and no
unrequested resources left behind. Existing shared provider/route state is
preserved. Do not bypass endpoint verification to obtain a green result.

## Goal 2 — Exact source and trustworthy results (third)

Status: queued behind Goal 1.

- Supply immutable source inputs with actual base/head SHAs.
- Define a minimal machine-readable completion and artifact contract.
- Record provenance in trusted runner code: repository, revisions, workflow
  revision, image identity, model, policy identity, run ID, timing, and status.
- Bound task runtime; establish cost/resource limits where supported.
- Validate result shape and source association independently of model output.
- Verify private-source credential exclusion and enforce least-privilege tools
  and sandbox policy.

Acceptance: exact inputs are attributable; model-echoed markers cannot establish
provenance; failure, timeout, cancellation, and invalid output are distinguishable;
the sandbox has only the capabilities needed for the task.

## Goal 3 — Demonstrated review usefulness (later)

Status: deferred until Goals 1/2 pass.

Run an artifact-only pilot on approximately 10–20 representative historical PRs:
known defects, clean changes, context-dependent changes, and adversarial inputs.
Compare against existing tools and maintainer practice. Agree on thresholds
before evaluation for precision, missed important defects, duplicate findings,
human correction effort, time saved, latency, and cost.

Acceptance: a written proceed/change/stop decision supported by labeled results.
Only then add a deterministic publisher with stale-head checks, output validation,
duplicate prevention, and minimal GitHub authority outside the model sandbox.

## Delivery and evidence

Keep changes surgical and validate before commits, pushes, and PRs. Merge only
after required CI is green and review comments are resolved. Record evidence
below as it is obtained; do not infer acceptance from implementation status.

| Milestone | Evidence | State |
|---|---|---|
| Goal 4 planning and image boundary | Local build/vet/unit tests, actionlint, shell syntax, and offline suite (14/14; 1 live skip) passed; PR CI pending | In progress |
| Goal 4 dependency revalidation | Pending workflow landing | Pending |
| Goal 1 local/GitHub inference | Pending | Queued |
| Goal 2 provenance/results | Pending | Queued |
| Release/compatibility checkpoint | Gated on Goal 1 | Deferred |
| Goal 3 usefulness pilot | Gated on Goals 1/2 | Deferred |

Validation note (2026-09-08): installed golangci-lint cannot parse this repo's
configuration; local lint is an environment failure, not a passing check.
Infrastructure tests have not been rerun for this documentation/workflow change.
Goal 1 preflight found OpenShell 0.0.110 and the existing local service-account
file. User-authorized probes minted a token and successfully verified Gemini
3.8 Flash in `acs-ai-677887` at the global endpoint. Claude Haiku returned 404
there; this account returned prediction-permission 403 in `itpc-ca-b7242ff092`.
All probe providers/workspaces were deleted. Sandbox execution is still pending.
The current GitHub PAT cannot list Actions secrets or variables (403). Use the
existing CI bootstrap without exposing credential values in chat or logs.

## Deferred investments and stop criteria

Defer Context documents, provider/model aliases, HyperShell M3, WIF migration,
private-network CI expansion, ACP/backends, broad resource parity, general
orchestration, multi-repository rollout, autonomous repairs, and agent-controlled
merges. Reconsider only when repeated use demonstrates a concrete need.

Prefer direct OpenShell if it is simpler and equally reviewable. If the reviewer
does not justify cost and maintainer attention, change or stop the task before
investing in publication. Do not grow the harness to compensate for an unproven
AI use case.

## Background

- [CI credential and bootstrap contract](../docs/ci.md)

Local, gitignored background (not available in repository checkouts):

- `working-docs/active/product-vision-and-gap-analysis.md`
- `working-docs/active/hypershell-testing-plan.md`
- `working-docs/specs/done/single-execution-path/README.md`

This roadmap supersedes older next-step ordering, not historical validation
records. The local ignored `pr-reviewer-plan/` is supporting material, not the
authoritative status tracker.
