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

The reviewer invokes [`setup-openshell`](../actions/setup-openshell/action.yml)
to install the pinned OpenShell CLI and wait for the local CI gateway. The
[`scripts/pr-review-local.sh`](../../scripts/pr-review-local.sh) wrapper creates
the temporary workspace/providers and configures inference. It calls
[`pr-review.sh run`](../../scripts/pr-review.sh) and tears down its setup
afterward. The review script stages the diff in `prepare`, checks eligibility,
renders the PR-specific policy, invokes the CLI, and validates output. The CLI
composes the task and manages its sandbox lifecycle.

The CLI already supports a direct managed-gateway connection. Moving this
review job to the intended managed StackRox deployment still requires platform
ownership of workspace membership, provider credentials and their refresh or
expiry, matching inference routes, and CI network access. A pre-provisioned
provider name does not by itself keep a short-lived GitHub token usable.
See [managed reviewer requirements](../../docs/ci.md#managed-reviewer-transition).

Once that contract is established, replace the job's local setup and temporary
provider bootstrap with managed authentication and `pr-review.sh run`. Preserve the task's allowed
operations and equivalent OpenShell policy and provider boundaries.

Comments may be posted during agent execution. Artifacts retain diagnostics;
cleanup or cancellation does not undo GitHub operations that already succeeded.

Add another reusable workflow only when the capability has a distinct trigger,
permission, or trust contract. Keep review and merge separate, and keep
repository-specific task behavior in [tasks/](../../tasks/) rather than
growing a single workflow with general-purpose image, policy, provider, or
command inputs.
