# Changelog

## [0.2.0] - 2026-06-13

### Added
- `--gateway NAME` and `--gateway-profile FILE` flags on `harness up` for gateway selection
- `--agent-profile` (`-f`) flag replaces `--file` on `harness up` and `harness create`
- Gateway profiles support inline Helm values and addon manifests (single self-contained YAML per target)
- Gateway profiles are embedded in the binary with fallback: `profiles/gateways/` → `gateways/` → embedded
- `LoadConfigFromBytes` and `LoadProfile` for flexible gateway config loading
- `status.Warnf` for formatted warning output
- `make tag` shows the current version from git describe
- CI artifacts: verbose test-flow logs uploaded as GitHub Actions artifacts
- `HARNESS_OS_` prefix for all harness-specific environment variables
- Apache 2.0 license

### Changed
- Gateway configs moved from `gateways/<name>/` directories to `profiles/gateways/<name>.yaml` flat files
- Provider profiles moved from `agents/providers/profiles/` to `profiles/providers/`
- `--local`/`--remote` flags replaced by `--gateway local`/`--gateway ocp` on `harness up`
- Image tags use `git describe` output instead of bare short SHAs
- Verbose output is now the default in test-flow.sh
- Sandbox image preloaded into kind on CI (eliminates slow registry pulls)
- Claude runs directly in sandbox — no wrapper script needed
- `gh auth setup-git` moved from startup script to CLAUDE.md instructions
- CLAUDE.md moved to `~/.claude/CLAUDE.md` (auto-read by Claude Code)
- Provider registration messages standardized to `%s: registered`
- All sandbox headers use noun form (`Sandbox`, not `Creating sandbox`)
- `ensureProviders` helper deduplicates validate-register-revalidate pattern
- Shared resolve functions moved to `cmd/resolve.go`
- Environment variables renamed: `SANDBOX_IMAGE` → `HARNESS_OS_IMAGE`, `HARNESS_DIR` → `HARNESS_OS_DIR`, `GATEWAY_NAME` → `HARNESS_OS_GATEWAY`, `PULL_SECRET` → `HARNESS_OS_PULL_SECRET`

### Removed
- `harness connect` and `harness logs` commands (use `openshell sandbox connect/logs` directly)
- Claude wrapper script (`sandbox/bin/claude`) and `claude-real` binary rename
- `startup.sh` from sandbox image (env vars injected via `--env`, no startup script needed)
- Dead code: `InferenceModel`, `BuildEnvSh`, `HasProviders`, `AllProviders`, `RunKubectlPassthrough`, `ShowEquivalentCmd`, `Detailf`
- Gateway interface reduced from 28 to 24 methods
- `docs/proto-migration.md` (stale, never executed)
- Stale TOML references and completed TODO items

## [0.1.2] - 2026-06-09

Initial Go rewrite release with full CLI, provider registration, and multi-target deployment.
