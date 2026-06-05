# TODO — Go Migration & Roadmap

## Migration Status

| Command | Go Status | Bash stays? | Notes |
|---------|-----------|-------------|-------|
| `new --local` | Native | Reference only | Profile parsing, provider validation, sandbox create with retry |
| `new --remote` | Bash wrapper | Yes | K8s Job YAML generation, kubectl apply |
| `connect` | Native | Reference only | exec into `openshell sandbox connect` |
| `deploy --local` | Native | Reference only | Podman check, gateway find/select/verify |
| `deploy --remote` | Bash wrapper | Yes | Helm install, route, mTLS, RBAC, SCCs |
| `teardown --sandboxes` | Native | Reference only | SandboxList + SandboxDelete via Gateway |
| `teardown --providers` | Native | Reference only | ProviderList + ProviderDelete + InferenceRemove |
| `teardown --k8s` | Bash wrapper | Yes | Helm uninstall, CRDs, SCCs, secrets, namespace |
| `preflight` | Native | Reference only | All 29 bats tests pass against Go |
| `providers` | Native | Reference only | Eliminates jq dependency |
| `test` | Bash wrapper | Yes | test-flow.sh orchestration |
| **Launcher** | Native | Reference only | In-cluster Go binary, UBI9 + openssh |

**Score: 8/12 paths native Go.** Remaining 4 are kubectl/helm-heavy operations.

## Remaining Bash Wrappers

### `new --remote` — Medium lift
- Generates K8s Job YAML inline (ConfigMap, volumes, env vars)
- Applies via `kubectl apply`, watches Job, tails logs
- Could use Go `text/template` for YAML + `k8s.io/client-go` or `os/exec kubectl`
- Prerequisite: decide whether to depend on client-go or keep shelling out to kubectl

### `deploy --remote` — Largest lift
- ~160 lines: namespace, CRDs, SCCs (oc adm), Helm install, route, mTLS cert copy
- Uses: kubectl, helm, oc (OpenShift-specific)
- Consider: keep as bash or accept client-go + helm SDK dependency
- Low priority — rarely changes, works reliably

### `teardown --k8s` — Medium lift
- Helm uninstall, delete CRDs/SCCs/secrets/namespace, gateway config cleanup
- Mirror of deploy --remote in reverse
- Port together with deploy --remote or not at all

### `test` — Low priority
- test-flow.sh is a bash test harness, not a harness subcommand
- Porting to Go would mean rewriting the test framework
- No value — bash is the right tool for test orchestration

## Architecture Improvements

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
