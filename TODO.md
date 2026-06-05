# TODO — Go Migration & Roadmap

## Migration Status

| Command | Go Status | Notes |
|---------|-----------|-------|
| `new --local` | Native | Profile parsing, provider validation, sandbox create with retry |
| `new --remote` | Native | K8s Job YAML via internal/k8s, prerequisite chain (deploy+providers+creds) |
| `connect` | Native | exec into `openshell sandbox connect` |
| `deploy --local` | Native | Podman check, gateway find/select/verify |
| `deploy --remote` | Native | Helm install, Route, mTLS, RBAC, SCCs via internal/k8s |
| `teardown --sandboxes` | Native | SandboxList + SandboxDelete via Gateway |
| `teardown --providers` | Native | ProviderList + ProviderDelete + InferenceRemove |
| `teardown --k8s` | Native | Helm uninstall, CRDs, SCCs, secrets, namespace via internal/k8s |
| `preflight` | Native | All 29 bats tests pass against Go |
| `providers` | Native | Eliminates jq dependency |
| `test` | Bash | test-flow.sh orchestration (intentionally stays bash) |
| **Launcher** | Native | In-cluster Go binary, UBI9 + openssh |

**Score: 11/12 paths native Go.** Only `test` stays bash (test orchestration, not a user command).

## Architecture Improvements

### Image registry as gateway config vs env override
- gateway.toml `[images]` section sets sandbox/launcher image refs
- `SANDBOX_IMAGE`/`LAUNCHER_IMAGE` env vars override config (for dev/CI)
- Two sources of truth: gateway.toml hardcodes a registry, env vars override it
- Consider: gateway.toml uses a `registry` field and images are relative to it,
  or gateway.toml supports variable expansion (`${REGISTRY}:sandbox`)
- Not urgent — env override approach works as a bridge

### Direct gRPC (future)
- OpenShell gateway exposes 54 gRPC RPCs (proto files in NVIDIA/OpenShell repo)
- Generate Go stubs from proto files → `gateway.GRPC` implementation
- Swap `gateway.NewCLI(cli)` → `gateway.NewGRPC(conn)` — one line change
- Eliminates: openshell CLI binary dependency, output parsing fragility
- Prerequisite: proto files stabilize (OpenShell is alpha)

### Remove Python dependency
- `providers.py` and `parse-profile.py` still in repo, called by bash path (`bin/harness`)
- Go implementations exist: `internal/preflight/` and `internal/profile/`
- Remove when: bash path is no longer needed for dual-testing
- Blocked by: decision to stop maintaining bash path

### Launcher consolidation
- `sandbox/launcher/` is a separate Go module (runs in-cluster)
- Has its own `parseConfig` duplicating `internal/profile/`
- Can't share code (different execution context, separate binary)
- Consider: extract shared types to a third module, or accept duplication

## Profile Schema — Cross-Project Alignment

Analysis of field naming and structure against OpenShell provider profiles (#896)
and Kaiden projects (#1272). See `profile.md` for full comparison.

### Current schema (Config struct)

```
name, image, command, keep, providers, [env]
```

### Changes

- [ ] Add `description` field — one line of human-readable context per profile.
      Makes multi-profile use cases (`harness new --profile research`) self-documenting.
      Both OpenShell and Kaiden include this.

- [ ] Split `[env]` by purpose — the current `[env]` map mixes inference config
      (`ANTHROPIC_*`), agent config (`CLAUDE_CODE_*`), provider metadata
      (`JIRA_URL`, `JIRA_USERNAME`), and custom provider workaround paths
      (`GOOGLE_WORKSPACE_*`). At minimum, add grouping comments in `default.toml`.
      Longer term, consider a `[provider-config]` section for non-secret provider
      metadata that belongs with the provider, not the sandbox. The `ANTHROPIC_*`
      vars should eventually drop entirely when OpenShell inference automation ships.

### No changes needed

- **`name`** — correct term, maps to `openshell sandbox create --name`
- **`image`** — harness-openshell differentiator (Kaiden is folder-first, we're image-first)
- **`command`** — maps to `openshell sandbox create --command`
- **`providers`** — correct term, matches OpenShell's provider naming throughout
- **`keep`** — unique to harness-openshell lifecycle, neither upstream has it, that's fine
- **TOML format** — trivially convertible to YAML (OpenShell) or JSON (Kaiden), no reason to change

### Future fields (not now)

- `repo` — git URL to clone into the sandbox at start (Kaiden supports this via `folder`)
- `secrets` — non-provider secrets to inject, cleaner than stuffing credentials into `[env]`

## Low-Priority Cleanup (from audit)

- [ ] `copyFile` in launcher: check Close error (`defer out.Close()` discards it)
- [ ] Launcher `configureGateway`: route stderr to `os.Stderr` not `os.Stdout` (line 61)
- [ ] Launcher `run()` helper: log errors instead of silently discarding
- [ ] Unexport internal-only functions in `internal/preflight/` and `internal/profile/`
- [ ] Stale comment in `providers.sh` line 18: says "default: us-east5", actual default is "global"
- [ ] `SandboxExec`/`SandboxUpload` on Gateway interface — no callers yet (premature abstraction, but harmless)

## Testing

### Current coverage
- 38 Go unit tests (gateway, profile, cmd)
- 7 launcher tests
- 29 bats preflight tests (Python + Go paths)
- Integration: `{bash, go}` × `{podman, ocp}` via `make validate`
- `--reuse-gateway` for fast OCP cycles (49s vs 137s)

### Gaps to fill
- [ ] Integration test for `providers --force` (currently no test exercises force mode)
- [ ] Go + OCP integration (`test-flow.sh ocp --full --go`) — not yet validated
- [ ] Preflight Go unit tests (internal/preflight/ has no _test.go)
- [ ] Kind gateway integration — `gateways/kind/gateway.toml` exists with full config (direct mode, nodeport) but is not exercised by test-flow.sh or CI. Add `test-flow.sh kind` and a GHA workflow with `kind create cluster`.
