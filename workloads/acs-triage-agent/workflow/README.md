# ACS triage workflow proof

This is a local-only Harness/OpenShell translation of the smallest useful ACS
triage slice: one read-only JIRA issue assignment.

The workflow keeps ACS ownership clear:

- `jira-issue-triager.md`, `teams.yaml`, and `jira-triage.schema.json` are read
  from the adjacent `../acs-triage-agent` checkout at run time.
- Harness only transfers those files, starts the agent, and downloads the JSON
  artifact.
- The pinned sandbox image carries the baseline OpenShell policy at
  `/etc/openshell/policy.yaml`; this proof does not override it with a second
  workflow policy.
- The Atlassian provider owns the JIRA credential. `JIRA_API_TOKEN` is not in
  this workflow.
- `READ_ONLY_MODE=true`, the MCP allowlist, and the Claude deny rules prevent
  this proof from becoming a write workflow.

Prerequisites:

- a reachable gateway with `atlassian` and `vertex-claude-haiku` already
  registered;
- `JIRA_URL` and `JIRA_EMAIL` in the calling environment; and
- an absolute path to the local ACS checkout.

The `atlassian` provider owns both the masked JIRA credential and the sandbox
network allowlist. `../openshell/providers/atlassian.yaml` is an external
bootstrap input; Harness does not load or apply it. ACS currently uses
`https://issues.redhat.com`, so the registered gateway provider must allow that
exact host before a live run. Do not solve this by adding unrestricted egress
to the workflow.

```bash
export ACS_TRIAGE_AGENT_DIR="$(cd ../acs-triage-agent && pwd)"
export JIRA_ISSUE_KEY=ROX-12345
export JIRA_URL=https://issues.redhat.com
export JIRA_EMAIL=triage-bot@example.com

./harness workflow apply workloads/acs-triage-agent/workflow/jira-issue-readonly.yaml \
  --output-dir ./acs-triage-artifacts
```

This is intentionally not the complete scheduled ACS workflow. CI failure
analysis, community triage, Slack publication, and JIRA updates remain owned by
the ACS repository and can be added as separate repository workflows after this
single-issue contract works.
