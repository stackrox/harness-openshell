# Workloads

A workload is a repository-owned task bundle. It keeps the task contract and
the OpenShell inputs together without making either one depend on Harness.

```text
workloads/<name>/
  README.md
  openshell/       # native OpenShell policy and provider-profile inputs
  workflow/        # Harness adapter, agent instructions, and payloads
```

The `openshell/` directory contains no credential values. Provider profiles are
endpoint and credential metadata that a platform administrator imports into a
gateway; provider instances and their credentials remain gateway-owned. The
`workflow/` directory contains the task-specific agent behavior and the
optional version 1 Harness document.

OpenShell has no single native workload-file abstraction. A workload can run
without Harness by using the image, policy, provider, and agent command with
the native `openshell sandbox create` and upload commands. Harness is an
adapter that composes the same inputs and manages the one-shot lifecycle.

For GitHub Actions, a capability-specific reusable workflow may select a bundle
and provide its trusted host inputs. Keep that adapter narrow: review and merge,
for example, remain separate because they have different provider credentials
and allowed mutations. Do not turn a reusable workflow into a privileged
general-purpose entrypoint that accepts arbitrary images, policies, providers,
or commands from callers.

Workloads reference platform-owned providers; they do not provision them. A
self-contained demo may create ephemeral providers in its trusted wrapper, but
managed deployments should use pre-provisioned workspace membership, providers,
and inference routes. Switching from a local to a managed gateway must not
change the workload's task or security contract.

Every workload README must state its trigger contract, trusted and untrusted
inputs, provider instance names, allowed mutations, policy rendering steps,
and cleanup expectations. Examples are opt-in; this repository does not enable
their GitHub Actions triggers automatically.

The initial validated set is intentionally small:

- `github-pr-reviewer` — read a staged pull-request diff and optionally publish
  bounded inline comments.
- `github-pr-merger` — validate an explicitly authorized pull request and merge
  it with a separate merge-capable provider credential.

Pull-request creation, issue review, PR watching, issue-to-PR, repository
triage, and ACS-specific workflows are deferred until these two workload
contracts have been exercised by consuming repos.
