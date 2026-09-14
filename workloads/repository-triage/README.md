# Read-only repository triage

This workload gathers context for one repository issue or incident and writes
one report artifact. It can inspect GitHub CI, repository documentation and
history, and a Jira issue through read-only provider boundaries. It cannot
comment, label, push, edit Jira, create a pull request, or change files outside
the report directory.

The caller supplies `TRIAGE_REPOSITORY`, `TRIAGE_REF`, `TRIAGE_ISSUE`,
`JIRA_ISSUE_KEY`, `JIRA_URL`, `JIRA_USERNAME`, and a rendered absolute
`TRIAGE_POLICY` path. The host prepares the repository checkout; the sandbox
receives it as read-only data. The output is downloaded with `--output-dir`.

The provider instances must be named `github-readonly` and
`atlassian-readonly`. Their profiles are in `openshell/providers/`; they hold
no credential values. The policies intentionally allow only GitHub GETs and
read-only Jira operations. Provider attachment is not a write grant.
