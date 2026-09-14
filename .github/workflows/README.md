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

The reviewer prepares the exact PR input through `harness github review`, then
chooses local or managed setup. Local is the default: the
[local review action](../actions/run-local-review/action.yml) calls
[`setup-openshell`](../actions/setup-openshell/action.yml), obtains local Google
credentials, and runs the temporary provisioning script around the review CLI.

`execution-target: managed` skips that action and executes the same CLI against
`managed-workflow`, a version 1 workflow from the trusted caller default branch.
The platform supplies existing resources and the credential lifecycle. The job
requests a read-only host GitHub token; sandbox writes use the existing provider.
`runner-label` must select a trusted runner with gateway/OIDC network access.
The review command does not write inference, provision providers, or delete
shared resources. See [managed setup](../../docs/ci.md#managed-reviewer-transition)
and the [adapter architecture](../../integrations/github/review/).

Comments may be posted during agent execution. Artifacts retain diagnostics;
cleanup or cancellation does not undo GitHub operations that already succeeded.

Add another reusable workflow only when the capability has a distinct trigger,
permission, or trust contract. Keep review and merge separate, and keep
repository-specific task behavior in [tasks/](../../tasks/) rather than
adding a general plugin or hook framework. Managed workflow configuration is
trusted host code and must preserve the review adapter protocol.
