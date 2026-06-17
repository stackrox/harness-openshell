# TODO — Roadmap

## Next up

### `harness init` [DONE]
- [x] Generate a harness.yaml with interactive prompts (entrypoint, providers, gateway)
- [x] Discover providers from `openshell provider list-profiles`
- [x] Print next steps ("run `harness doctor` then `harness apply`")
- [x] `--non-interactive`, `--force`, `--output` flags

### `harness doctor` [DONE]
- [x] Check openshell installed and version
- [x] Check target-specific deps (podman/docker, kubectl, kind, kubeconfig)
- [x] Check provider credentials via `openshell provider profile export`
- [x] Online phase: check provider registration if gateway reachable
- [x] `-o table|json|yaml` output

### registerProviders should filter by agent's provider list
- `registerProviders()` in `cmd/providers.go` registers all providers regardless
  of what the agent needs. Fix: filter by `agentCfg.ProviderNames()`.

## CLI [DONE]

- [x] `harness apply` with `--dry-run`, `-o yaml|json`, `--attach`, `-f`, `--task`, `--entrypoint`
- [x] `harness get agents|providers|gateways` with `-o table|json|yaml`
- [x] `harness describe <name>` with `-o table|json|yaml`
- [x] `harness delete <name>` with `--all`, `--sandboxes`, `--providers`, `--k8s`
- [x] `harness deploy [local|ocp|kind]`
- [x] Headless task mode: `--task "text"` or `--task @file` runs agent with `--print`
- [x] `kind: policy` applied via `openshell policy set` after sandbox creation
- [x] `teardown` and `status` as hidden deprecated aliases
- [x] `up`, `create`, `render`, `start`, `stop` removed

## Agent Config [DONE]

- [x] Multi-document harness YAML (`kind: agent/provider/gateway/payload/policy`)
- [x] `kind: payload` with `sandbox_path`/`local_path`/`content` + multi-upload
- [x] Agent-level `payloads:` list merged with document-level payloads
- [x] `kind: config` kept as silent alias for backwards compat
- [x] Image defaults overridable via payloads (no image rebuild needed)

### Config reconciliation (`apply -o yaml`) -- future
- [ ] Show where each value came from (default, profile, harness file, env var)
- [ ] Credentials rendered as `${VAR}` placeholders
- [ ] Round-trip: `apply -o yaml > snapshot.yaml && apply -f snapshot.yaml`

### Future fields
- [ ] `description` -- one line of human-readable context per agent config
- [ ] `repo` -- git URL to clone into the sandbox at start

## Testing [DONE]

- [x] Config test suite: 37 tests across 7 categories
- [x] Agent integration: claude + opencode inference, gh cli, jira mcp, gws gmail
- [x] CI: config-suite + test-suite-live in workflows

## Architecture (future)

### Direct gRPC
- OpenShell gateway exposes 54 gRPC RPCs
- Would eliminate CLI binary dependency and output parsing fragility
- Prerequisite: proto files stabilize (OpenShell is alpha)

### Upstream issues to track
- #1719 -- K8s Operator design (providers as CRDs, gateway narrows to data-plane)
- #1851 -- Plugin system (affects binary naming)
- #1886 -- Declarative provider config in gateway.toml (core team rejected; redirected to #1719)
- #1520 -- Sandbox specs / apply -f (stale, no maintainer engagement)
- #1814 -- Named sandbox templates (no comments, blocked on #863)
- #1922 -- Portable sandbox log collection
- #1933 -- Centralized audit/event log

Upstream direction signal (as of 2026-06): the gateway stays a strict foundation
layer. Provider lifecycle and sandbox declaration are moving toward the operator/CRD
model for K8s. johntmyers mentioned hooks/middleware for API calls coming soon.
The harness's provider registration and multi-document YAML have no upstream
replacement on the current roadmap.

## Observability & Tracing

Langfuse hooks plugin working. MLflow spiked. SigNoz identified as strongest
OTel backend. Integration deferred until `init`/`doctor` ship.

## Release

- [x] CHANGELOG.md + LICENSE (Apache 2.0)
- [x] `harness init` for standalone binary distribution
