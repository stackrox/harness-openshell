# GitHub issue reviewer

This workload is activated by a trusted caller when an issue has the
`needs-ai-review` label. It reads one issue and may post one comment. It cannot
push code, change labels, edit milestones, or create a pull request.

The caller must validate the label and pass `REVIEW_REPOSITORY`, `REVIEW_ISSUE`,
and a rendered absolute `REVIEW_POLICY` path. Render the policy template with
the exact repository and issue before calling Harness; policy contents are not
implicitly interpolated by the runner.

The provider profile is endpointless and the policy binds the provider instance
`github-issue-reviewer` to only the selected issue's read and comment paths.
Import the profile and create that provider instance in OpenShell; never put a
GitHub token in this directory.
