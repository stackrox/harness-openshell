# GitHub issue to pull request

This workload is activated by a trusted caller when an issue has an approved
automation label. It reads that issue, checks out the repository, makes a
bounded change, pushes one new branch, creates one pull request, and comments
with the link.

It cannot merge, change labels, edit settings, or push the default branch.
Repository branch protection and a narrowly scoped GitHub App installation are
still required because Git Smart HTTP policy cannot encode branch names.

The caller must validate the label and supply `REVIEW_REPOSITORY`,
`REVIEW_ISSUE`, `REVIEW_BASE_REF`, and a rendered `REVIEW_POLICY`. The host-side
source checkout must be authenticated separately; the sandbox receives only
the checkout and an endpoint-bound provider placeholder.
