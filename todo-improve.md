# harness-openshell improvement backlog

Generated from 12-expert review (2 workflows, 6 agents each).
Triaged against design.md rearchitect plan.
Effort: S (<1hr), M (1-4hr), L (4hr+).

---

## Bugs

- [x] **quiet-mode kubectl never retries on transient errors** [S]
- [x] **non-seekable stdin retries replay empty input** [S]
- [x] **gateway timeout returns exit 0** [S]
- [x] **buggy contains()/indexOf() test helpers** [S]
- [x] **kubectl error message uses opts.Args not actual executed args** [S]
- [x] **runQuiet goroutine leak on timeout** [S]

## Done — safety and quality

- [x] **teardown no-args defaults to maximum destruction** [S]
- [x] **--local/--remote not mutually exclusive on new** [S]
- [x] **deployRemote prints no guidance on partial failure** [S]
- [x] **ProviderList/SandboxList dedup** — `parseFirstColumn` helper [S]
- [x] **RunHelm returns (string, error) but string is always empty** [S]
- [x] **runQuiet should use context.WithTimeout** [S]
- [x] **Merge preflight.go + check.go** [S]
- [x] **Merge parse.go into profile.go** [S]
- [x] **shared test mock in wrong file** → `cmd/helpers_test.go` [S]
- [x] **detectHarnessDir silently falls back to "."** [S]
- [x] **detectHarnessDir loop duplicated** [S]
- [x] **Makefile lacks lint/vet targets** [S]

## Done — dead code

- [x] `SandboxUpload` + `SandboxExec` — zero callers
- [x] `status.Warnf` — zero callers
- [x] `decodeBase64` — trivial wrapper, inlined
- [x] `cmd.Stderr = nil` in `runOutput` — no-op
- [x] `internal/util` package — inlined at call site
- [x] Four Gateway sub-interfaces — zero consumers

---

## Do now — quick independent fixes

These are safe to do before the rearchitect. Each is independent and
won't conflict with design.md changes.

- [ ] **error wrapping inconsistency** — ~15 instances of `%v` instead of `%w`. Audit and replace. `cmd/deploy.go` `cmd/new.go` `cmd/providers.go` [S]
- [ ] **env-var-with-fallback repeated 12+** — add `EnvOr(key, fallback)` helper. `cmd/deploy.go` `cmd/providers.go` `cmd/creds.go` `cmd/new.go` [S]
- [ ] **launcher binary swallows errors** — check cmd.Run(), json.Unmarshal, copyFile close error, mTLS dir 0755→0700. `sandbox/launcher/main.go` [S]
- [ ] **duplicated Config struct in launcher** — import `internal/profile` instead. Delete ~25 lines. `sandbox/launcher/main.go` [S]
- [ ] **test writes to hardcoded /tmp path** — use `t.TempDir()`. `internal/gateway/cli_test.go` [S]
- [ ] **secret name string literals scattered** — define constants. `cmd/creds.go` `cmd/teardown.go` `cmd/deploy.go` [S]
- [ ] **hardcoded sk-ant- placeholder and personal email in default.toml** — remove PII. `profiles/default.toml` [S]
- [ ] **CheckInput file case has 4 levels of nesting** — flatten with early returns. `internal/preflight/preflight.go` [S]
- [ ] **deployLocal unnecessary podmanPath variable** — inline. `cmd/deploy.go` [S]
- [ ] **force-deleting pods with --grace-period=0** — use `--grace-period=30`. `cmd/new.go` [S]
- [ ] **ConfigFile.ChartVersion alias** — consumer reads `cfg.Upstream.ChartVersion` directly. `internal/preflight/preflight.go` [S]
- [ ] **pickKeys/pickKeysExcept/formatMeta** — inline at call sites. `internal/preflight/preflight.go` [S]
- [ ] **extractYAMLID** — simplify with `strings.Cut`. `cmd/providers.go` [S]
- [ ] **unreachable return nil** — `cmd/new.go:323` [S]
- [ ] **swallowed errors in deploy.go and new.go** — check remaining discarded errors. `cmd/deploy.go` `cmd/new.go` [S]
- [ ] **sandbox CRD from unversioned latest URL** — pin version. `cmd/deploy.go` [S]
- [ ] **SCC grant/revoke lists duplicated** — shared slice. `cmd/deploy.go` `cmd/teardown.go` [S]
- [ ] **launcher image single-arch** — use `docker buildx` for launcher. `Makefile` [S]

## Defer to rearchitect (design.md)

These will be restructured or replaced during the command/code reorg.

- [ ] **Move orchestration out of cmd/** → the rearchitect IS this [L]
- [ ] **Table-driven provider registration** → redesigned with `providers register` [M]
- [ ] **Preflight subcommands** → redesigned in new command structure [S]
- [ ] **Cobra examples** → add after commands are renamed [S]
- [ ] **Hardcoded values → harness.toml config** → addressed by config redesign [M]
- [ ] **Context propagation + cancellation** → natural to add when splitting new→create/up [L]
- [ ] **Gateway CLI timeout** → comes with context propagation [M]
- [ ] **kubectl log tailing bypasses k8s.Runner** → restructured code [S]
- [ ] **Configmap creation dedup** → restructured code [S]
- [ ] **No test coverage for registerProviders/RunCheck** → rewritten functions [M]
- [ ] **ProviderGet → ProviderExists rename** → interface redesign [S]
- [ ] **CLIVersion removal** → interface cleanup [S]
- [ ] **Inline Job spec → YAML template** → move to deploy/ [M]
- [ ] **Gateway CLI output parsing** → depends on openshell `--output json` [M]
- [ ] **GWS credential export flow duplicated** → restructured code [M]
- [ ] **Provider registration order** → `inference_provider` config field [S]

## Critical — address during rearchitect

- [ ] **cluster-admin ClusterRoleBinding + privileged SCC on default SA** — scoped ClusterRole in `deploy/`, use `kubectl apply`, remove `default` from SCC grants [M]
- [ ] **credentials visible in ps aux and verbose logs** — pass via subprocess env vars or stdin, add credential redaction to `status.Cmd` [M]
