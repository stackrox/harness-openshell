# GitHub pull-request merger

This task bundle is intentionally separate from review. It uses a distinct
provider instance whose credential is allowed to merge pull requests. The
trusted caller must pass the exact repository, PR number, head SHA, merge
method, and `MERGE_ALLOWED=true` after applying its own approval rules.

The agent instructions require reading the PR and head commit checks,
verifying that the PR is open, non-draft, clean, and still at the expected head
SHA, then requesting one merge. The OpenShell REST policy restricts the
available read and merge endpoints. It does not itself enforce `MERGE_ALLOWED`,
validate the merge request body, or limit the number of requests. These task
checks are distinct from the proxy's method and path restrictions.

The policy grants no comment, label, Git push, or other-repository endpoint.
A merge-capable GitHub App should be installed only where this task is
explicitly intended to run. This repository includes the bundle, not a reusable
merge workflow; enabling it and verifying live behavior in a consuming
repository are separate steps. Sandbox cleanup does not undo a completed merge.

## Layout

- `workflow/` contains the harness workflow document, OpenCode configuration, and merge
  skill.
- `openshell/` contains the native policy and endpointless provider profile.

The policy contains no credential values. The caller renders its repository and
PR variables before applying it.
