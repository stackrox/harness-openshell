# Workflow runner

This directory contains the Go implementation of the Harness OpenShell workflow
runner:

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
