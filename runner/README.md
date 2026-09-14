# Runner

This directory contains the Go implementation of the `harness` CLI, the small
composition and sandbox lifecycle component of `harness-openshell`. A harness
workflow document declares one run; a GitHub Actions workflow supplies the CI
trigger and trusted setup around that run.

The runner:

- loads and validates a versioned workflow document;
- resolves a local or managed OpenShell target;
- verifies referenced providers and reconciles the declared inference route,
  or requires it to match without writes under `--require-existing-inference`;
- creates a sandbox with the selected image, policy, provider attachments, and
  command;
- uploads source and payloads, observes execution, downloads outputs, and
  deletes the sandbox by default.

It does not start an OpenShell gateway, create workspaces or providers, handle
provider credentials, or choose task-specific permissions. Trusted setup or
platform administration owns those resources, while task bundles and their CI
adapters own task behavior and trust decisions.

The sandboxed agent can perform external operations through OpenShell's
credential-backed network proxy when its token permissions and task policy
allow them. Downloaded output files are separate from those operations.
Deleting a sandbox does not reverse a posted comment or completed merge, and
an execution failure does not prove that no external operation occurred.

## Layout

- `main.go` is the `harness` CLI entrypoint.
- `cmd/` contains CLI commands and the thin wiring for repository integrations.
- `../integrations/github/review/` owns GitHub review behavior and calls the
  existing apply service through a single execution function.
- `internal/` contains the workflow parser, planner, OpenShell adapter, and
  execution support.

The repository keeps `go.mod` at its root so the runner, integration tests, and
repository tooling remain one Go module. Build the CLI with:

```bash
go build -o harness ./runner
```

Task bundles remain under `tasks/`; they are inputs to this runner, not
Go packages. Repository-level integration tests remain under `test/` because
they exercise shell workflows and gateway lifecycle behavior rather than the
runner packages themselves.

The same binary connects to a selected local gateway or directly to a managed
gateway. `harness github review` uses existing resources and shares this
runner's execution service. Temporary local CI setup lives in
[`pr-review-local.sh`](../scripts/pr-review-local.sh); managed setup belongs to
the platform. See the [adapter architecture](../integrations/github/review/)
and [managed setup](../docs/ci.md#managed-reviewer-transition).
