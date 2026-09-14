# Native OpenShell inputs

This directory is the native security boundary for the reviewer. Render
`policy.yaml` with trusted `REVIEW_REPOSITORY`, `REVIEW_PR` and
`REVIEW_GITHUB_PROVIDER` values before
creating the sandbox. The policy permits only the pull-request metadata,
comments, reviews, and changed-file reads needed for review, plus inline comment
POSTs to that same pull request. It grants no Git transport, issue mutation,
label, approval, merge, or repository-settings access.

The sandbox must attach an existing endpointless GitHub provider instance
(`github-review` by default). The
endpointless profile in `providers/github-review.yaml` describes the credential
shape but contains no credential value. The workflow also expects the gateway's
`vertex-review` inference provider and `inference.local` route; those are
platform-owned and are not created by this task.

For a native run, import or adapt the profile, provision the provider through
trusted OpenShell administration, attach it to the sandbox, and upload the
workflow payloads. Harness composes these same inputs and cleans up the
one-shot sandbox; it does not own provider credentials.
