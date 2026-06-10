# TODO — Roadmap

## Architecture

### Direct gRPC (future)
- OpenShell gateway exposes 54 gRPC RPCs (proto files in NVIDIA/OpenShell repo)
- Generate Go stubs from proto files -> `gateway.GRPC` implementation
- Swap `gateway.NewCLI(cli)` -> `gateway.NewGRPC(conn)` -- one line change
- Eliminates: openshell CLI binary dependency, output parsing fragility
- Prerequisite: proto files stabilize (OpenShell is alpha)

### Image registry as gateway config vs env override
- gateway.toml `[images]` section sets sandbox/runner image refs
- `SANDBOX_IMAGE`/`RUNNER_IMAGE` env vars override config (for dev/CI)
- Two sources of truth: gateway.toml hardcodes a registry, env vars override it
- Consider: gateway.toml uses a `registry` field and images are relative to it

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

- [ ] Remove `providers.toml`; add provider profile validation in its place
- [ ] Convert gateway configs from TOML to YAML
- [ ] Specify/document the YAML formats (agent config, provider profiles)
- [ ] Document non-secret provider env vars (what `providers[].config` captures
      and why it exists alongside secret credentials)

## CLI

- [ ] Flows that support agent.yaml (`create`, `up`) should also support
      `--provider-profile` and provider config overrides

## Agent Config

### Future fields
- [ ] `description` -- one line of human-readable context per agent config
- [ ] `repo` -- git URL to clone into the sandbox at start
- [ ] `secrets` -- non-provider secrets to inject

## Testing

### Current coverage
- Go unit tests across cmd/ (including launch.go) and all internal/ packages
- 29 bats preflight tests (run in CI via `.github/workflows/ci.yml`)
- Integration: local + kind + OCP via `make test-all`

### Gaps
- [ ] Integration test for `providers --force`
- [ ] Unit test for the full `runLaunch` orchestration (currently only its
      helpers — configureGateway, checkProviders, launchCreateSandbox — are tested)

## Release

- [ ] Add CHANGELOG.md for 0.1
- [ ] Add LICENSE file
- [ ] `harness init` command for standalone binary distribution (no repo clone)

## Deferred (post-0.1)

- [ ] Rename K8s SA `openshell-launcher` -> `openshell-runner` (breaking for deployed OCP clusters)
- [ ] Rename `LauncherSection` -> `RunnerSection` in gateway config TOML
- [ ] Gateway-level LLM proxy/logging (gateway.toml `[proxy]` section)
- [ ] Multi-agent workflow support (fleet.yaml / workflow.yaml)
- [ ] `harness policy suggest` (DenialEvent stream -> policy proposals)
- [ ] Fleet management (multi-gateway kubectl-context style)
