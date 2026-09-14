# GitHub pull-request watcher

This workload is an explicit comment-driven fixer. A trusted caller should
invoke it only for a same-repository open PR when a comment contains the
approved command (for example, `/ai-fix`). It reads PR discussion, edits the
checked-out source, pushes a fix to the existing PR branch, and posts a status
comment.

It cannot merge, change labels, edit repository settings, or push the default
branch. Branch protection remains required because GitHub network policy cannot
express a branch name inside Git Smart HTTP.

The caller must provide `REVIEW_REPOSITORY`, `REVIEW_PR`, `REVIEW_HEAD_REF`, and
a rendered `REVIEW_POLICY`. The provider instance must be named
`github-pr-watcher`. Fork PRs are intentionally out of scope for this first
workflow because the trusted provider must not gain arbitrary fork write access.
