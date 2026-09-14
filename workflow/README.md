# Workflow runner

This directory contains the Go implementation of the Harness OpenShell workflow
runner. It is the generic composition and lifecycle layer, not a GitHub Actions
framework or a provider-management service.

The runner:

- loads and validates a versioned workflow document;
- resolves a local or managed OpenShell target;
- verifies referenced providers and reconciles the declared inference route;
- creates a sandbox with the selected image, policy, provider attachments, and
  command;
- uploads source and payloads, observes execution, downloads outputs, and
  deletes the sandbox by default.

It does not start an OpenShell gateway, create workspaces or providers, handle
provider credentials, or choose task-specific permissions. Platform bootstrap
owns those resources, while workload bundles and their CI adapters own task
behavior and trust decisions.

## Layout

- `main.go` is the `harness` CLI entrypoint.
- `cmd/` contains the CLI commands.
- `internal/` contains the workflow parser, planner, OpenShell adapter, and
  execution support.

The repository keeps `go.mod` at its root so the runner, integration tests, and
repository tooling remain one Go module. Build the CLI with:

```bash
go build -o harness ./workflow
```

Workload bundles remain under `workloads/`; they are inputs to this runner, not
Go packages. Repository-level integration tests remain under `test/` because
they exercise shell workflows and gateway lifecycle behavior rather than the
runner packages themselves.

The same binary can connect to a developer's selected local gateway or directly
to a managed gateway. Moving CI to a managed gateway changes authentication and
platform bootstrap around the runner, not this package's responsibility.
