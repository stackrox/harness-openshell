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

Every workload README must state its trigger contract, trusted and untrusted
inputs, provider instance names, allowed mutations, policy rendering steps,
and cleanup expectations. Examples are opt-in; this repository does not enable
their GitHub Actions triggers automatically.
