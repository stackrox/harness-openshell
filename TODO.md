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

## CLI

- [ ] Flows that support agent.yaml (`create`, `up`) should also support
      `--provider-profile` and provider config overrides

## Agent Config

### Self-contained agent YAML (multi-document, k8s-style)
- [ ] Support multi-document YAML (`---` separated) where all objects live in one file
- Agent still references everything by name (`profile: github`), but provider/gateway/policy
  definitions can be co-located in the same file instead of separate files in profiles/
- Parser reads all documents via `yaml.Decoder` loop, indexes by `kind`+`name`, resolves
  references against local set first, falls back to profiles/ tree
- Composes naturally: split the file up and drop objects into `profiles/` to share across agents
- Example:
  ```yaml
  ---
  kind: agent
  name: my-agent
  entrypoint: claude
  gateway: local
  providers:
    - profile: github
    - profile: vertex
  env:
    ANTHROPIC_BASE_URL: https://inference.local
  ---
  kind: provider
  name: github
  type: github
  credentials: [GITHUB_TOKEN]
  endpoints:
    - { host: "api.github.com", port: 443 }
  ---
  kind: provider
  name: vertex
  type: google-vertex-ai
  credentials: [GOOGLE_APPLICATION_CREDENTIALS]
  ---
  kind: gateway
  name: local
  type: local
  insecure: true
  ---
  kind: policy
  network_policies:
    github:
      endpoints:
        - { host: "api.github.com", port: 443 }
  ```
- Goal: `harness up -f agent.yaml` with one file. Zero to working sandboxed agent.
- Existing single-document agent YAMLs (no `kind` field) continue to work unchanged

### `harness render` as live config snapshot
- [ ] `harness render` queries the running gateway for effective state, not just YAML files
- Outputs what is actually configured: registered providers, active gateway, inference
  config, sandbox policy, env structure
- Credentials replaced with `${VAR}` placeholders -- the snapshot is shareable
- Replay with different creds: `GITHUB_TOKEN=theirs harness up -f snapshot.yaml`
- Like `kubectl get -o yaml` -- captures the running shape, not the source config
- Round-trip: `harness render > snapshot.yaml && harness up -f snapshot.yaml` should
  reproduce the same agent setup (with different credentials from env)
- [ ] `harness render preview -f harness.yaml` -- dry-run that resolves all references
  (providers, gateway, env vars) and shows the fully resolved config without deploying.
  Like `terraform plan` or `helm template`. Shows what `harness up` would do.

### Future fields
- [ ] `description` -- one line of human-readable context per agent config
- [ ] `repo` -- git URL to clone into the sandbox at start
- [ ] `secrets` -- non-provider secrets to inject

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
