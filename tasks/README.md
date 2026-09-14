# Task bundles

A task bundle combines agent instructions and the OpenShell inputs for a
defined operation. It keeps the
task contract usable with native OpenShell commands or the `harness` CLI.

```text
tasks/<name>/
  README.md
  openshell/       # native OpenShell policy and provider-profile inputs
  workflow/        # harness workflow document, agent instructions, and payloads
```

The `openshell/` directory contains no credential values. Provider profiles are
endpoint and credential metadata that a platform administrator imports into a
gateway; provider instances and their credentials remain gateway-owned. The
`workflow/` directory contains the task-specific agent behavior and the
optional version 1 harness workflow document.

OpenShell has no single native task-bundle file abstraction. A task can run
with native OpenShell by using the image, policy, provider, and agent command
with `openshell sandbox create` and upload commands. The `harness` CLI composes
the same inputs and manages one sandbox run with cleanup.

For GitHub Actions, a reusable workflow for a defined operation may select a
bundle and provide its trusted host inputs. Keep that adapter narrow: review and merge,
for example, remain separate because they have different provider credentials
and allowed mutations. Do not turn a reusable workflow into a privileged
general-purpose entrypoint that accepts arbitrary images, policies, providers,
or commands from callers.

Task bundles reference gateway provider instances; trusted setup or platform
administration provisions them. The current reviewer creates temporary
providers in [`scripts/pr-review-local.sh`](../scripts/pr-review-local.sh). A managed
deployment should establish workspace membership, provider credential
lifecycle, and matching inference routes centrally. Keeping provider names
stable still requires a way to mint or refresh short-lived credentials.
Preserve the task's allowed operations when moving to a managed gateway with
equivalent provider and policy support.

Agents may perform permitted GitHub operations during a run. Token permissions
and OpenShell REST policy bound those requests; task instructions describe the
desired behavior within that boundary. Downloaded `outputs` are files, and
sandbox deletion does not reverse external operations already completed.

Every task README must state its trigger contract, trusted and untrusted
inputs, provider instance names, allowed mutations, policy rendering steps,
and cleanup expectations. Examples are opt-in; this repository does not enable
their GitHub Actions triggers automatically.

The initial set is intentionally small:

- `github-pr-reviewer` — read a staged pull-request diff and optionally publish
  inline comments through the reusable reviewer. The instructions request at
  most three comments; the REST policy restricts endpoints, not comment count.
- `github-pr-merger` — validate an explicitly authorized pull request and merge
  it with a separate merge-capable provider credential. This is an opt-in
  bundle without a reusable merge workflow in this repository.

Bundle availability, configured integrations, and live validation in a
consuming repository are separate milestones. Gateway lifecycle tests alone
do not establish successful GitHub mutations or review quality.

Pull-request creation, issue review, PR watching, issue-to-PR, repository
triage, and ACS-specific workflows are deferred until these two task
contracts have been exercised by consuming repos.
