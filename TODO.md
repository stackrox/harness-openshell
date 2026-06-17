# TODO — Roadmap

## Architecture

### Direct gRPC (future)
- OpenShell gateway exposes 54 gRPC RPCs (proto files in NVIDIA/OpenShell repo)
- Generate Go stubs from proto files -> `gateway.GRPC` implementation
- Swap `gateway.NewCLI(cli)` -> `gateway.NewGRPC(conn)` -- one line change
- Eliminates: openshell CLI binary dependency, output parsing fragility
- Prerequisite: proto files stabilize (OpenShell is alpha)

### registerProviders should filter by agent's provider list
- `registerProviders()` in `cmd/providers.go` uses the gateway config's provider
  list, not the agent config's. When `gwCfg` is nil (common case), it tries to
  register all providers regardless of what the agent needs.
- Fix: pass the agent's provider names to `registerProviders` and use them as
  a filter alongside (or instead of) the gateway config's list.
- Files: `cmd/providers.go` (registerProviders signature), `cmd/executor.go` (call site)

## CLI [DONE]

kubectl-style refactor complete (PRs #67-#70):
- [x] `harness apply` with `--dry-run`, `-o yaml|json`, `--attach`, `-f`
- [x] `harness get agents|providers|gateways` with `-o table|json|yaml`
- [x] `harness describe <name>`
- [x] `harness delete <name>` with `--all`, `--sandboxes`, `--providers`, `--k8s`
- [x] `harness deploy`, `start`, `stop` unchanged
- [x] `teardown` and `status` as hidden deprecated aliases
- [x] Old commands (`up`, `create`, `render`) removed (PR #68)

## Agent Config

### Multi-document harness YAML [DONE]
- [x] `kind: agent/provider/gateway/policy` dispatch via `yaml.Decoder` loop
- [x] `Harness` type with `ParseHarness`/`ParseHarnessFile`
- [x] `RenderHarness` with built-in vs custom provider labeling
- [x] Resolution: harness-local definitions > profiles/ tree > embedded defaults

### `kind: config` — embed sandbox files in harness YAML
- [ ] `kind: config` documents for `claude.json`, `CLAUDE.md`, `mcp.json`,
      `opencode.json`, `settings.json`, `policy.yaml`
- [ ] Parsed by `ParseHarness` and stored as `Harness.Configs map[string][]byte`
- [ ] Rendered to payload directory by `RenderPayload` instead of baking into image
- [ ] Keeps sandbox image minimal — all agent-specific config in the harness YAML
- [ ] Example:
  ```yaml
  ---
  kind: config
  name: claude.json
  content: |
    {"mcpServers": {"atlassian": {"command": "mcp-atlassian"}}}
  ---
  kind: config
  name: CLAUDE.md
  content: |
    You are working inside an OpenShell sandbox.
  ```

### Config reconciliation (`apply -o yaml`)
- [ ] Resolves agent YAML against profiles/, defaults, and running gateway
- [ ] Shows where each value came from (default, profile, harness file, env var)
- [ ] Credentials rendered as `${VAR}` placeholders — shareable, replayable
- [ ] Round-trip: `apply -o yaml > snapshot.yaml && apply -f snapshot.yaml`

### Provider abstraction layer
- [ ] `kind: provider` targets `openshell provider create` today (imperative)
- [ ] Abstraction supports future backends: gateway.toml (#1886), K8s CRDs (#1719)

### Future fields
- [ ] `description` — one line of human-readable context per agent config
- [ ] `repo` — git URL to clone into the sandbox at start
- [ ] `secrets` — non-provider secrets to inject

## Testing [DONE]

Config test suite (PR #71): 37 tests across 7 categories.
- [x] Config parsing, output formats, env resolution, CLI flags
- [x] Live sandbox lifecycle (create, describe, exec, env injection, delete)
- [x] Provider registration (github, atlassian, vertex, gws, all-providers)
- [x] Agent integration (claude inference, opencode inference, gh cli, jira mcp, gws gmail)
- [x] CI: config-suite in CI workflow, test-suite-live in integration workflow

### Known gaps
- [ ] OpenCode + Vertex inference in sandbox blocked by policy fields not supported in 0.0.59
      (`allow_encoded_slash`, custom policy sections crash supervisor)
- [ ] Local `podman build` on macOS produces images that behave differently than CI `docker build`

## Upstream alignment

### Plugin compatibility (#1851)
- [ ] Binary naming: plan for `openshell-harness` (PATH-based plugin discovery)
- [ ] Status: #1851 is `question` label, not accepted. Design standalone first.

### Image building delegation
- [ ] Evaluate delegating to `openshell-image-builder` for advanced image composition
- [ ] Layered policy composition: base + agent-specific + user overlay

### Upstream issues to track
- #1719 — K8s Operator design (affects provider CRDs, declarative config)
- #1851 — Plugin system (affects binary naming, env var contract)
- #1886 — Declarative provider config in gateway.toml (affects `kind: provider`)
- #1922 — Portable sandbox log collection (affects observability)
- #1933 — Centralized audit/event log (affects run recorder)

## Release

- [x] Add CHANGELOG.md
- [x] Add LICENSE file (Apache 2.0)
- [ ] `harness init` command for standalone binary distribution (no repo clone)

## Observability & Tracing

Investigation (Jun 2026) validated paths for capturing agent session data.
Langfuse hooks plugin installed and working. MLflow spiked. SigNoz identified
as strongest OTel backend for full signal coverage.

### Harness integration (future)
- [ ] `harness apply` auto-injects OTel env vars or configures Langfuse hooks
- [ ] `harness runs list/show` queries traces from the backend
- [ ] Headless mode (`harness run --task '...'`) records automatically

## Deferred (post-0.1)

- [ ] Multi-agent workflow support (fleet.yaml / workflow.yaml)
- [ ] `harness policy suggest` (DenialEvent stream -> policy proposals)
- [ ] Fleet management (multi-gateway kubectl-context style)
