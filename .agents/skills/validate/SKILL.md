---
name: validate
description: Validate harness-openshell locally and report the matching GitHub CI state. Use when asked to validate, run tests, check everything, or before a commit, push, or PR.
---

# Validate

Run the fastest deterministic checks first. Run infrastructure-dependent checks
only when their prerequisites are available, and report every skip with its
reason. Do not report hard-coded test totals; use each command's observed
summary.

## Pull-request CI

GitHub Actions currently runs these validation layers on pull requests and
pushes to `main`:

| Workflow | Validation |
|----------|------------|
| `ci.yml` | vet, unit tests, offline config suite, golangci-lint |
| `integration.yml` | canonical SDK lifecycle on local and Kind gateways plus the live config suite |
| `images.yml` | multi-architecture sandbox image build when image inputs change; trusted events also push |

HyperShell does not run on GitHub-hosted runners: its OIDC issuer is reachable
only from the Red Hat network/VPN.

Normal branch pushes do not trigger these workflows. A branch without a pull
request can therefore have no CI runs.

## Local validation

### 1. Static and fast checks

```bash
go build ./...
go vet ./...
CGO_ENABLED=0 go test ./...
golangci-lint run ./...
actionlint
bash -n test/test-flow.sh test/kind-lifecycle.sh test/suite/run.sh
```

If an optional tool is not installed, report that check as skipped. Do not
silently substitute a different check.

### 2. Offline config suite

```bash
make test-suite
```

This includes config parsing and rendering, CLI behavior, structured output,
and v1alpha1 plan coverage. Some gateway-dependent checks are expected
to skip when no gateway is reachable.

### 3. Canonical Kind integration

Requires `kind`, Helm, an OpenShell CLI compatible with `.openshell-version`,
and a working Docker or Podman daemon.

```bash
CI=true CONTAINER_CLI=docker make test-kind
```

CI mode is the credential-free, canonical v1alpha1 SDK create/exec/delete
lifecycle used by pull-request CI. Substitute `podman` only when its machine is
running. Confirm the temporary cluster is removed unless `KEEP=1` was requested.

### 4. Local gateway integration

Requires a reachable local OpenShell gateway:

```bash
make cli
CI=true ./test/test-flow.sh local-container
make test-suite-live
```

Both commands cover the credential-free canonical SDK lifecycle. Provider
capability checks require providers provisioned by the platform and are reported
separately when available.

Run `make test-local` without `CI=true` only when evaluating configured provider
and inference capabilities; that target also pushes the development image to its
configured registry. Missing provider capabilities should be reported as skips.

### 5. OCP integration

Requires a non-empty `KUBECONFIG` and a reachable cluster:

```bash
make test-remote
```

This is an opt-in environment validation, not pull-request CI.

### 6. HyperShell integration

Run the canonical SDK lifecycle locally from the Red Hat network/VPN using a
git-excluded service-account environment file. Platform bootstrap grants that
account workspace membership once; no administrator token is used at runtime.

```bash
make test-hypershell HYPERSHELL_SA_ENV=path/to/user-sa.env
make test-hypershell-haiku HYPERSHELL_SA_ENV=path/to/user-sa.env
```

The second target requires the durable Vertex provider and inference route
documented in `docs/ci.md`. Report an unreachable OIDC issuer as a VPN/network
skip and a missing provider or route as a platform-bootstrap failure.

### 7. Documentation consistency

Build the CLI, compare its current top-level commands with `README.md`, and
search for references to removed commands. `SPEC.md` is intentionally not part
of the repository.

```bash
make cli   # the root ./harness binary is gitignored; build it first
./harness --help
rg -n 'harness (apply|get|describe|delete|doctor|init|plan)' README.md
rg -n 'harness (up|create|render|deploy)' README.md
```

Review matches in context; user instructions must use current commands.

### 8. CI status

```bash
branch=$(git branch --show-current)
gh run list --branch "$branch" --limit 10
```

If there are no runs, distinguish a normal branch push from a failed or missing
pull-request run. If GitHub authentication is unavailable, report CI status as
unverified.

## Report

Report `PASS`, `FAIL`, or `SKIP (reason)` for:

- build, vet, unit tests, lint, workflow/shell validation
- offline config suite
- canonical Kind integration
- local CI and live integration
- configured provider capabilities
- OCP integration
- HyperShell integration
- documentation consistency
- current-branch GitHub CI

Use observed test counts and identify whether a failure is in product behavior,
test orchestration, infrastructure, or credentials.
