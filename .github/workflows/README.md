# GitHub workflows

This directory contains repository CI and capability-specific reusable
workflows. The reusable workflow is the cross-repository GitHub Actions
interface: it fixes the job permissions, trusted checkout, concurrency, and
host bootstrap for one workload instead of exposing the runner as an arbitrary
privileged action.

`pr-review-reusable.yml` currently provides the pull-request review capability.
It checks out trusted workflow code from the caller's default branch, treats the
pull-request diff as data, obtains narrowly scoped credentials, and invokes the
`github-pr-reviewer` workload. Consumers should pin both the reusable workflow
reference and its `harness-ref` input to the same immutable commit SHA.

## Gateway transition

Today the PR-review workflow installs a local OpenShell gateway, and its trusted
wrapper creates an ephemeral workspace and providers for the run. That makes
the integration self-contained while the managed service contract is being
established; it is not behavior of the Harness runner.

The intended StackRox arrangement authenticates this job to a managed gateway
whose workspace membership, providers, and inference route are provisioned by
the platform. At that point the local gateway installation and ephemeral
provider bootstrap can be removed while the selected workload and runner stay
the same.

Add another reusable workflow only when the capability has a distinct trigger,
permission, or trust contract. Keep review and merge separate, and keep
repository-specific task behavior in `workloads/` rather than growing a single
workflow with general-purpose image, policy, provider, or command inputs.
