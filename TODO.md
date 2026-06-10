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

### Consolidate internal/profile into internal/agent
- ✅ Removed dead code: `Parse()`, `ParseFile()`, `BuildSandboxEnv()`
- Only `Config` struct, `ValidateProviders`, and `StageHarnessDir` remain
- TODO: Move `ValidateProviders` to agent or gateway package, inline `Config` into sandbox.go
- TODO: Remove TOML dependency (github.com/BurntSushi/toml) if no longer used

## Agent Config

### Future fields
- [ ] `description` -- one line of human-readable context per agent config
- [ ] `repo` -- git URL to clone into the sandbox at start
- [ ] `secrets` -- non-provider secrets to inject

## Testing

### Current coverage
- Go unit tests across cmd/, internal/agent, internal/gateway, internal/k8s
- 29 bats preflight tests
- Integration: local + kind + OCP via `make dev-test-all`

### Gaps
- [ ] Tests for cmd/launch.go (configureGateway mTLS, in-cluster flow)
- [ ] Integration test for `providers --force`
- [ ] Preflight Go unit tests (internal/preflight/ has no _test.go)
- [ ] Add bats step to `.github/workflows/ci.yml` (Makefile ci runs bats, GHA doesn't)

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
