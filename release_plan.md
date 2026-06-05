# Release Plan: CI → Embed → GoReleaser

## Phase 0: CI (done)

GitHub Actions for every PR and push to main.

**`.github/workflows/ci.yml`** — two jobs:
- **test**: `go vet` + `go test ./...` for both modules (harness + launcher)
- **lint**: `golangci-lint` via the official action

**`.github/workflows/release.yml`** — stub, triggered on `v*` tags. Runs GoReleaser once embed is ready (Phase 2).

Go version synced from `go.mod` via `go-version-file`.

Integration tests (podman, OCP) are deferred — extensive unit mock coverage exists for all orchestration paths. When needed, ubuntu-latest runners have podman pre-installed; openshell CLI can be installed from GitHub releases.

---

## Phase 1: Embed files + `harness init`

**Goal:** Single binary that works without cloning the repo.

The CLI already resolves everything relative to `harnessDir`. The plan:

1. **Embed** all runtime files into the binary
2. **Add `harness init`** — extracts embedded files to `~/.openshell/harness/`
3. **Update `detectHarnessDir()`** — add `~/.openshell/harness/` as a fallback
4. **No other code changes** — all existing `filepath.Join(harnessDir, ...)` calls work as-is

### Step 1: Create the embed package

Create `internal/embed/embed.go`:

```go
package embed

import "embed"

//go:embed all:files
var Files embed.FS
```

Create directory `internal/embed/files/` populated by `make embed-sync`. Runtime files to embed:
- `sandbox/profiles/atlassian.yaml`
- `sandbox/CLAUDE.md`, `settings.json`, `mcp.json`, `policy.yaml`, `startup.sh`
- `profiles/default.toml`
- `values-ocp.yaml`

**Decision: copy, not symlink.** `go:embed` doesn't follow symlinks. `make embed-sync` copies originals into `internal/embed/files/`. GoReleaser hooks call this automatically.

### Step 2: Add `harness init` command

Create `cmd/init.go`:
- Walks embedded FS, writes to `~/.openshell/harness/` (or `--dir`)
- Preserves directory structure
- Skips existing files unless `--force`
- Makes `.sh` files executable

### Step 3: Update `detectHarnessDir()`

Add `~/.openshell/harness/` as fallback:

```
1. $HARNESS_DIR env var
2. Walk up from executable location
3. Walk up from cwd
4. ~/.openshell/harness/ (if profiles/default.toml exists there)
5. Error + exit
```

### Step 4: Add Makefile targets

- `make embed-sync` — copies runtime files into `internal/embed/files/`
- Update `make cli` to depend on `embed-sync`

---

## Phase 2: GoReleaser

**Goal:** `git tag v*` → GitHub Release with binaries for all platforms.

### `.goreleaser.yaml`

```yaml
before:
  hooks:
    - make embed-sync
builds:
  - env: [CGO_ENABLED=0]
    goos: [linux, darwin]
    goarch: [amd64, arm64]
archives:
  - format: tar.gz
```

GitHub Releases only (no Homebrew tap — private repo at Red Hat).

### Install experience

**One-liner (requires `gh` auth):**
```bash
gh release download --repo robbycochran/harness-openshell -p '*darwin_arm64*' -O- | tar xz
sudo mv harness /usr/local/bin/
harness init
```

**From source (unchanged):**
```bash
git clone ... && cd harness-openshell
make cli
./harness new --local
```

---

## Phase 3: Integration CI (future)

When needed, add a GHA workflow for podman-based integration tests:
- Ubuntu runners have podman pre-installed
- Install openshell CLI from GitHub releases in workflow
- Run `test/test-flow.sh podman` (no `--full` initially)
- OCP tests stay manual (requires a live cluster)

Not urgent — unit tests with mocked k8s.Runner and gateway.Gateway cover all orchestration paths.

---

## Files summary

| File | Phase | Action |
|------|-------|--------|
| `.github/workflows/ci.yml` | 0 | Create — unit tests + lint |
| `.github/workflows/release.yml` | 0 | Create — stub for GoReleaser |
| `internal/embed/embed.go` | 1 | Create — embed declaration |
| `internal/embed/files/` | 1 | Create — populated by `make embed-sync` |
| `cmd/init.go` | 1 | Create — `harness init` command |
| `main.go` | 1 | Modify — fallback + register init |
| `.goreleaser.yaml` | 2 | Create |
| `.gitignore` | 1 | Modify — ignore `internal/embed/files/` |
| `Makefile` | 1 | Modify — add `embed-sync` target |

## Verification

| Phase | Check |
|-------|-------|
| 0 | Push PR → CI runs, tests + lint pass |
| 1 | `make embed-sync && make cli` → binary builds with embedded files |
| 1 | `rm -rf ~/.openshell/harness && ./harness init` → files extracted |
| 1 | `cd /tmp && harness preflight` → detects `~/.openshell/harness/` |
| 2 | `goreleaser release --snapshot --clean` → binaries for all platforms |
