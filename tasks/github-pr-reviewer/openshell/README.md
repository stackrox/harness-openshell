# Native OpenShell inputs

This directory is the native security boundary for the reviewer. Render
`policy.yaml` with the trusted `REVIEW_REPOSITORY`, `REVIEW_PR`, and
`REVIEW_GITHUB_PROVIDER` values before creating the sandbox. The policy permits
only the pull-request metadata,
comments, reviews, and changed-file reads needed for review, plus inline comment
POSTs to that same pull request. It grants no Git transport, issue mutation,
label, approval, merge, or repository-settings access.

The sandbox must attach an existing `github-review` provider instance. The
endpointless profile in `providers/github-review.yaml` describes the credential
shape but contains no credential value. The OpenCode workflow expects the
gateway's `vertex-review` inference provider. The opt-in Codex workflow expects
an existing OpenAI-compatible provider, named `openai-inference` by default. Both
use the platform-owned `inference.local` route and neither provider is created
by this workload.

For a native run, import or adapt the profile, provision the provider through
trusted OpenShell administration, attach it to the sandbox, and upload the
workflow payloads. Harness composes these same inputs and cleans up the
one-shot sandbox; it does not own provider credentials.
