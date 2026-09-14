# GitHub pull-request merger

This workload is intentionally separate from review. It uses a distinct
provider instance whose credential is allowed to merge pull requests. The
trusted caller must pass the exact repository, PR number, head SHA, merge
method, and `MERGE_ALLOWED=true` after applying its own approval rules.

The agent reads the PR and the head commit checks, verifies that the PR is open,
non-draft, clean, and still at the expected head SHA, then performs one merge.
It never comments, changes labels, pushes Git refs, or accesses another
repository. A merge-capable GitHub App should be installed only where this
workload is explicitly intended to run.

## Layout

- `workflow/` contains the Harness adapter, OpenCode configuration, and merge
  skill.
- `openshell/` contains the native policy and endpointless provider profile.

The policy contains no credential values. The caller renders its repository and
PR variables before applying it.
