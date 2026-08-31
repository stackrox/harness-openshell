# Project status snapshot

Snapshot date: 2026-08-31

Reviewed revision: `250cf35e5d50443c0437dd4417b34f304ab8aea1`

Source inspection included the unmerged `rc-pr6-source-hardening` branch at
`dacc054`. This is a review snapshot, not a test report. No build, unit test, or
live gateway operation was run.

## Changes since the previous snapshot

| Change | Status | Assessment |
|---|---|---|
| Sandbox reads moved to the OpenShell SDK | Merged in PR7a | Removes CLI-output parsing from `get`, `describe`, and `delete` |
| Gateway provisioning removed | Merged in PR7b | Establishes the correct bring-your-own-gateway boundary |
| Apply target resolved once | Merged in PR7b | Provider/inference reconciliation and sandbox creation now use the same gateway |
| URL-hashed repository mirrors and per-run checkouts | PR6 branch | Addresses basename collisions and concurrent-run corruption; still needs merge |

## Current assessment

The project has moved beyond being only a wrapper around `openshell sandbox
create`. Its shared plan/reconcile decisions, provider ownership and adoption
rules, credential-preserving SDK boundary, policy-at-create behavior, and single
sandbox lifecycle are legitimate controller foundations.

Removing gateway provisioning materially improves the product boundary:

| Layer | Responsibility |
|---|---|
| OpenShell | Sandbox isolation, policy enforcement, provider proxying, and native runtime lifecycle |
| HyperShell | Gateway fleet, clusters, tenancy, identity, service accounts, and routing |
| OpenShell operator | Persistent OpenShell infrastructure and resource reconciliation |
| harness-openshell | Workflow intent, execution portability, promotion, and evidence |

The durable product should not be the reconciliation mechanism itself. A future
OpenShell operator may replace persistent-resource writes. The durable opportunity
is a portable, policy-constrained workflow contract that consumes centrally
governed capabilities and returns auditable results.

## Implemented strengths

- The harness no longer owns gateway or Kubernetes provisioning.
- Apply resolves one gateway target and threads it through reconciliation and run.
- `internal/config` defines a strict versioned desired-resource model.
- Provider planning and reconciliation share action rules.
- Managed and referenced providers have explicit ownership semantics.
- Provider updates are designed to preserve credential material.
- Policy is supplied when the sandbox is created.
- `internal/run` owns sandbox creation, retry, retention, and cleanup.
- CI exercises local and Kind OpenShell deployments on Linux.

## Architectural transition still in progress

There are still two configuration paths:

- `harness apply` consumes the legacy `agent.AgentConfig` model.
- `harness plan` consumes `harness.openshell.dev/v1alpha1`.
- `cmd/desired.go` translates only part of the legacy model into the new model.

Consequently, plan and apply do not operate on the same resolved object. The
strict schema's workspace, referenced-provider, source, agent-argument, and
retention concepts are not yet the ordinary execution contract.

This is especially important for a shared HyperShell gateway. Legacy apply treats
Vertex AI and Google Workspace providers as harness-managed and eligible for
adoption. A repository CI job must instead reference platform-managed capabilities
and fail if they are unavailable; it must not mutate shared providers or inference
routes.

## Product readiness

| Area | Status |
|---|---|
| Runtime and policy foundation | Credible |
| Bring-your-own-gateway boundary | Established |
| Source isolation | Implemented on an unmerged branch |
| Canonical workflow API | Incomplete: plan/apply split |
| Shared-gateway workspace isolation | Not available in apply |
| Strict capability verification | Incomplete: reconciliation degrades to warnings |
| Agent portability | Incomplete: Codex still inherits Claude command semantics |
| Skill packaging | Task-file upload only |
| Structured run result/evidence | Not implemented |
| Reusable HyperShell GitHub workflow | Not implemented |

The repository is now a credible workflow-runner foundation. Its next risk is not
excessive gateway machinery; it is spending effort on convenience commands before
finishing the centrally governed workflow contract that would make it reusable
across ACS.
