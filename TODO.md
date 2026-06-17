# TODO — Roadmap

## Architecture

### Direct gRPC (future)
- OpenShell gateway exposes 54 gRPC RPCs (proto files in NVIDIA/OpenShell repo)
- Generate Go stubs from proto files -> `gateway.GRPC` implementation
- Swap `gateway.NewCLI(cli)` -> `gateway.NewGRPC(conn)` -- one line change
- Eliminates: openshell CLI binary dependency, output parsing fragility
- Prerequisite: proto files stabilize (OpenShell is alpha)

### Image registry as gateway config vs env override
- `HARNESS_OS_IMAGE` env var overrides the version-based image resolution (for dev/CI)
- Consider: gateway.yaml uses a `registry` field and images are relative to it

### registerProviders should filter by agent's provider list
- `registerProviders()` in `cmd/providers.go` uses the gateway config's provider
  list, not the agent config's. When `gwCfg` is nil (common case), it tries to
  register all providers regardless of what the agent needs.
- Why: confusing output — users see "skipped" messages for providers their
  agent doesn't reference. No functional impact (missing credentials are
  silently handled).
- Fix: pass the agent's provider names to `registerProviders` and use them as
  a filter alongside (or instead of) the gateway config's list.
- Files: `cmd/providers.go` (registerProviders signature), `cmd/up.go` (call site)

## Config Format

- [ ] Specify/document the YAML formats (agent config, provider profiles)
- [ ] Document non-secret provider env vars (what `providers[].env` captures
      and why it exists alongside secret credentials)

## CLI — kubectl-style refactor

Sequenced implementation plan. Each phase builds on the previous.

### Phase 1: `apply` command (replaces `up` and `create`)
- [ ] `cmd/apply.go` — unified deploy command, delegates to existing `upLocal()`
- [ ] `-f` flag as primary interface (replaces `--agent-profile`)
- [ ] `--attach` flag (default false) for interactive TTY (flips old `up` default)
- [ ] `--dry-run` — resolve everything, report pass/fail per step, don't deploy
- [ ] `-o yaml` — output fully resolved harness YAML with source annotations
- [ ] `-o json` — machine-readable resolved config
- [ ] Env var fallbacks: `OPENSHELL_GATEWAY`, `OPENSHELL_GATEWAY_ENDPOINT` (#1851)
- [ ] Bare `harness apply` uses default agent config (same as old `harness up`)
- [ ] Mark `up` and `create` as hidden deprecated aliases via cobra `Deprecated` field

### Phase 2: `get` and `describe` commands (replaces `status`)
- [ ] `cmd/get.go` — parent command with subcommands:
  - `harness get agents` (aliases: `sandboxes`) — list running sandboxes
  - `harness get providers` — list registered providers
  - `harness get gateways` — list gateways
- [ ] Shared `OutputFormat` type: `table|json|yaml` via `-o` flag on all `get` subcommands
- [ ] Credential exclusion: `-o json/yaml` never includes secret values (#1830 pattern)
- [ ] `cmd/describe.go` — `harness describe <name>` for detailed sandbox status
- [ ] Mark `status` as hidden deprecated alias

### Phase 3: `delete` command (replaces `teardown`)
- [ ] `cmd/delete.go` — targeted + bulk deletion:
  - `harness delete <name>` — delete specific sandbox
  - `harness delete --all` — full teardown (sandboxes + providers + k8s)
  - `harness delete --providers` — providers only
  - `harness delete --k8s` — k8s resources only
- [ ] Reuses existing `teardownSandboxes()`, `teardownProviders()`, `teardownK8s()`
- [ ] Mark `teardown` as hidden deprecated alias

### Phase 4: Integration + docs
- [ ] Update `test/test-flow.sh` to use new verbs
- [ ] Update SPEC.md command reference
- [ ] Update README.md command reference
- [ ] `render` becomes hidden alias for `apply -o yaml`
- [ ] `deploy` stays as-is (infrastructure action)
- [ ] `start`/`stop` stay as-is (lifecycle actions)

## Agent Config

### Multi-document harness YAML [DONE]
- [x] `kind: agent/provider/gateway/policy` dispatch via `yaml.Decoder` loop
- [x] `Harness` type with `ParseHarness`/`ParseHarnessFile`
- [x] `RenderHarness` with built-in vs custom provider labeling
- [x] Resolution: harness-local definitions > profiles/ tree > embedded defaults
- [x] Backwards compat: single-doc agent YAMLs without `kind` still work

### Config reconciliation (`apply -o yaml`)
- [ ] Resolves agent YAML against profiles/, defaults, and running gateway
- [ ] Shows where each value came from (default, profile, harness file, env var)
- [ ] Credentials rendered as `${VAR}` placeholders — shareable, replayable
- [ ] Round-trip: `apply -o yaml > snapshot.yaml && apply -f snapshot.yaml`
- [ ] `--dry-run` without `-o` reports pass/fail (gateway available? providers
      resolvable? image exists? env vars resolved?)

### `kind: config` — embed sandbox files in harness YAML (future)
- [ ] `kind: config` documents for `claude.json`, `CLAUDE.md`, `mcp.json`, etc.
- [ ] Rendered to payload directory instead of baking into sandbox image
- [ ] Keeps sandbox image minimal — all agent-specific config in the harness YAML

### Provider abstraction layer
- [ ] `kind: provider` targets `openshell provider create` today (imperative)
- [ ] Abstraction supports future backends: gateway.toml (#1886), K8s CRDs (#1719)
- [ ] Do not hard-code execution strategy — upstream is undecided

### Future fields
- [ ] `description` — one line of human-readable context per agent config
- [ ] `repo` — git URL to clone into the sandbox at start
- [ ] `secrets` — non-provider secrets to inject

## Upstream alignment

### Plugin compatibility (#1851)
- [ ] Binary naming: plan for `openshell-harness` (PATH-based plugin discovery)
- [ ] Dual invocation: standalone `openshell-harness` and plugin `openshell harness`
- [ ] Env var fallbacks for gateway/endpoint/verbosity when running as plugin
- [ ] Auth via `openshell-bootstrap` APIs (no token forwarding)
- [ ] Status: #1851 is `question` label, not accepted. Design standalone first.

### Image building delegation
- [ ] Evaluate delegating to `openshell-image-builder` for advanced image composition
- [ ] Harness generates config (`.kaiden/workspace.json` + `config.toml`), builder consumes
- [ ] Layered policy composition: base + agent-specific + user overlay
- [ ] Programmatic Containerfile generation from config (stop maintaining static Dockerfiles)

### Upstream issues to track
- #1719 — K8s Operator design (affects provider CRDs, declarative config)
- #1851 — Plugin system (affects binary naming, env var contract)
- #1886 — Declarative provider config in gateway.toml (affects `kind: provider`)
- #1922 — Portable sandbox log collection (affects observability)
- #1933 — Centralized audit/event log (affects run recorder)

## Testing

### Current coverage
- Go unit tests across cmd/ and all internal/ packages (run in CI via `.github/workflows/ci.yml`)
- Integration: local + kind + OCP via `make test-all`

### Gaps
- [ ] Integration test for `harness up --provider-refresh`

## Release

- [x] Add CHANGELOG.md
- [x] Add LICENSE file (Apache 2.0)
- [ ] `harness init` command for standalone binary distribution (no repo clone)

## Observability & Tracing

Investigation (Jun 2026) validated two paths for capturing full agent session
data (prompts, responses, tool calls, token counts, cost):

### What works today
- **OpenShell OCSF JSONL** (`ocsf_json_enabled` setting) captures network/process/policy
  events inside the sandbox. Structured, OCSF v1.7.0 compliant. No conversation content.
- **Claude Code OTel export** sends traces (span structure), logs (full API request/response
  bodies), and metrics (token counts, cost) via standard OTLP env vars.
- **Langfuse hooks plugin** (`langfuse-observability`) reads Claude Code transcript files
  directly and creates Langfuse traces with full input/output. Best LLM-specific UI.
  Setup: `docs/langfuse-setup.md`.
- **MLflow** accepts OTel traces at `/v1/traces` (not logs). Proven working with Claude Code
  via `OTEL_EXPORTER_OTLP_ENDPOINT`. Good for span structure + token counts but not
  conversation content (that lives in the OTel logs signal, which MLflow doesn't ingest).

### Integration options (pick one or combine)
- [ ] **Langfuse (hooks)** -- full conversation content, best UI, self-hosted Docker.
      No OTel plumbing needed. Plugin reads transcripts post-hoc.
- [ ] **Langfuse (OTel)** -- span structure via OTLP/HTTP at `/api/public/otel`.
      Prompts land in metadata (not input field) because Claude Code uses `user_prompt`
      attribute, not `gen_ai.prompt`. Response content not captured via this path.
- [ ] **MLflow (OTel)** -- traces only, no logs. Good for span structure + AI Gateway
      (inference routing with budget/rate limiting). Self-hosted SQLite or Postgres.
- [ ] **SigNoz (OTel)** -- accepts traces + logs + metrics on same OTLP endpoint.
      Self-hosted Docker, 4GB RAM. Only backend that ingests all three Claude Code signals.
- [ ] **OTel Collector fan-out** -- route traces to Langfuse/MLflow, logs to SigNoz/Loki,
      metrics to SigNoz/Prometheus. Best-of-breed but more infrastructure.

### Harness integration (future)
- [ ] `harness deploy` starts observability backend (Langfuse/MLflow/SigNoz) if not running
- [ ] `harness up` injects OTel env vars or configures hooks automatically
- [ ] `harness runs list/show` queries traces from the backend
- [ ] Headless mode (`harness run --task '...'`) records automatically

### Upstream to watch
- OpenShell portable sandbox log collection (#1922) and centralized audit/event log (#1933)
  are in early investigation. No concrete implementation yet -- this is where the harness
  recorder fits.
- AgentGateway (Linux Foundation) proposed for embedding in OpenShell (#998, rejected by
  NVIDIA). Could be deployed as a companion proxy for LLM traffic observability with
  token-level detail. See design doc.

### Design doc
`~/.gstack/projects/robbycochran-harness-openshell/rc-rc-nextnext-design-20260613-130837.md`

## Deferred (post-0.1)

- [ ] Multi-agent workflow support (fleet.yaml / workflow.yaml)
- [ ] `harness policy suggest` (DenialEvent stream -> policy proposals)
- [ ] Fleet management (multi-gateway kubectl-context style)
