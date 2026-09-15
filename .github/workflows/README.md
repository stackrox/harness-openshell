# GitHub workflows

This directory contains repository CI and reusable GitHub Actions workflows
for defined operations, such as PR review. Each reusable workflow fixes the
job permissions, trusted checkout, concurrency, and setup for its task bundle.
It is the cross-repository interface supplied by `harness-openshell`.

[`pr-review-reusable.yml`](pr-review-reusable.yml) currently provides PR review.
It checks out trusted workflow code from the caller's default branch, treats the
pull-request diff as data, obtains a repository-scoped GitHub App token, and
invokes the `github-pr-reviewer` task bundle. Its OpenShell REST policy permits
the sandboxed agent to read the selected PR and post inline comments to it.
The token's repository permissions and the policy's PR-specific HTTP methods
and paths are separate restrictions. Consumers should pin both the workflow
reference and its `harness-ref` input to the same immutable commit SHA.

## Gateway setup: local CI and managed deployment

For local CI, the reviewer invokes [`setup-openshell`](../actions/setup-openshell/action.yml)
to install the pinned OpenShell CLI and wait for the local CI gateway. The
[`scripts/pr-review-local.sh`](../../scripts/pr-review-local.sh) wrapper creates
the temporary workspace/providers and configures inference. It calls
[`pr-review.sh run`](../../scripts/pr-review.sh) and tears down its setup
afterward. The review script stages the diff in `prepare`, checks eligibility,
renders the PR-specific policy, invokes the CLI, and validates output. The CLI
composes the task and manages its sandbox lifecycle.

Set the caller repository's `OPENSHELL_GATEWAY_ENDPOINT` and complete the
[managed connection configuration](../../docs/ci.md#managed-reviewer-transition)
to run the same review against HyperShell. This path calls `pr-review.sh run`
directly with OIDC connection metadata and a gateway service-account secret.
It skips local OpenShell installation, Google authentication, and temporary
provider setup. Partial managed configuration fails before the task runs.

The platform supplies workspace membership, the `github-review` provider and
credential refresh, and the Gemini 2.5 Pro `inference.local` route. The host's
GitHub App token still serves PR metadata checks; it does not update the managed
provider. `OPENSHELL_RUNNER` selects a Linux runner with access to the gateway
and issuer. The default remains `ubuntu-latest`.

Comments may be posted during agent execution. Artifacts retain diagnostics;
cleanup or cancellation does not undo GitHub operations that already succeeded.

Add another reusable workflow only when the capability has a distinct trigger,
permission, or trust contract. Keep review and merge separate, and keep
repository-specific task behavior in [tasks/](../../tasks/) rather than
growing a single workflow with general-purpose image, policy, provider, or
command inputs.
