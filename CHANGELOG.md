# Changelog

## [Unreleased]

### Changed
- Cloned repos now use URL-hashed bare mirrors (`~/.cache/harness-openshell/mirrors/`)
  plus per-run, self-contained checkouts (`~/.cache/harness-openshell/checkouts/`)
  instead of the basename-keyed `repos/` cache. Distinct repositories that share a
  basename no longer collide, and concurrent runs of the same repository no longer
  share a working tree. Each checkout is a real repository with its own `.git`, so
  git keeps working inside the sandbox after upload. The old
  `~/.cache/harness-openshell/repos/` directory is orphaned and safe to delete
  manually.

## [0.3.0] - 2026-06-17

### Added
- Kubectl-style CLI: `harness apply` with `--dry-run` and `-o yaml`, plus `get`, `describe`, and `delete` commands
- Multi-document harness YAML (`---` separated agent/provider/gateway/policy docs)
- `harness init` and `harness doctor` commands
- `kind: config` embeds sandbox files directly in harness YAML
- Repository checkout support and an inference warning
- Cloned repos cached in `~/.cache/harness-openshell/repos/`
- Configuration test suite with multi-config and free-API support
- Headless tasks, policy applied via the CLI, and multi-upload payloads
- Gateway profile auto-discovery and a configurable profile directory

### Changed
- Deprecated commands removed; docs rewritten around the apply-first CLI
- Gateway profiles renamed
- Container registry switched to `quay.io/rcochran/openshell`
- Repository moved to `stackrox/harness-openshell`; CodeRabbit review config added

### Fixed
- OpenCode config (`ANTHROPIC_BASE_URL`, MCP format, policy)

## [0.2.0] - 2026-06-13

### Added
- `--gateway NAME` and `--gateway-profile FILE` flags on `harness up` for gateway selection
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
- Shared config resolution was consolidated
- Environment variables renamed: `SANDBOX_IMAGE` → `HARNESS_OS_IMAGE`, `HARNESS_DIR` → `HARNESS_OS_DIR`, `GATEWAY_NAME` → `HARNESS_OS_GATEWAY`, `PULL_SECRET` → `HARNESS_OS_PULL_SECRET`

### Removed
- `harness connect` and `harness logs` commands (use `openshell sandbox connect/logs` directly)
- Claude wrapper script (`sandbox/bin/claude`) and `claude-real` binary rename
- `startup.sh` from sandbox image (env vars injected via `--env`, no startup script needed)
- Dead code: `InferenceModel`, `BuildEnvSh`, `HasProviders`, `AllProviders`, `RunKubectlPassthrough`, `ShowEquivalentCmd`, `Detailf`
- Gateway interface reduced from 28 to 24 methods
- `docs/proto-migration.md` (stale, never executed)
- Stale TOML references and completed TODO items

## [0.1.2] - 2026-06-08

Initial Go rewrite release with full CLI, provider registration, and multi-target deployment.
