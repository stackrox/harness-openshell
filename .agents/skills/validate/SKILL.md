---
name: validate
description: Run the full test matrix for harness-openshell. Use when asked to "validate", "run tests", "check everything", or before any commit/push/PR.
---

# Validate

Run the full test and documentation matrix. Skip steps that require unavailable infrastructure.

## Steps

Run each step sequentially. Report pass/fail for each.

### 1. Build

```bash
go build ./...
```

### 2. Unit tests

```bash
CGO_ENABLED=0 go test ./...
```

### 3. Vet

```bash
go vet ./...
```

### 4. Local integration (full providers)

Requires: `openshell` running locally, provider credentials configured.

```bash
make test-local
```

Skip if `openshell` is not on PATH or the gateway is not running.

### 5. Local integration (CI mode)

Requires: `openshell` running locally. No credentials needed.

```bash
make test-local
```

Pass `--ci` to `test-flow.sh` (auto-detected when `CI=true`).

### 6. OCP integration

Requires: `KUBECONFIG` set, cluster accessible.

```bash
make test-remote
```

Skip if `KUBECONFIG` is not set or `kubectl cluster-info` fails.

### 7. Kind integration

Requires: `kind` on PATH.

```bash
make test-kind
```

Skip if `kind` is not on PATH.

### 8. CI status

Check if CI is green for the current branch:

```bash
gh run list --branch $(git branch --show-current) --limit 3
```

### 9. Config test suite

```bash
make test-suite
```

Runs 27+ config parsing, output format, env resolution, and CLI flag tests.
No gateway needed for most tests. Skip if `harness` binary not built.

### 10. Config test suite (live)

Requires: `openshell` running locally.

```bash
make test-suite-live
```

Adds live sandbox create/exec/delete tests. Skip if gateway not running.

### 11. Docs consistency

Check that README.md and SPEC.md accurately reference the commands
registered in main.go. Every primary command in main.go should appear
in both README.md and SPEC.md. No stale command references should exist.

```bash
# Commands registered in main.go (primary, not deprecated)
grep 'cmd.New.*Cmd' main.go | grep -v Hidden | grep -v Deprecated

# Check README references all primary commands
for cmd in apply get describe deploy stop start; do
  grep -q "harness $cmd" README.md && echo "README: $cmd OK" || echo "README: $cmd MISSING"
done

# Check SPEC references all primary commands
for cmd in apply get describe deploy stop start; do
  grep -q "harness $cmd" SPEC.md && echo "SPEC: $cmd OK" || echo "SPEC: $cmd MISSING"
done

# Check for stale references to removed commands
for cmd in "harness up" "harness create" "harness render"; do
  grep -c "$cmd" README.md SPEC.md 2>/dev/null
done
```

Report any docs/code mismatches.

## Output

Report a summary table:

```
Validation Results
------------------
  Build:          PASS
  Unit tests:     PASS (6 packages)
  Vet:            PASS
  Local (full):   PASS (22/22)
  Local (CI):     PASS (14/14)
  OCP:            PASS (10/10)
  Kind:           SKIP (kind not installed)
  CI:             GREEN (3/3 workflows)
  Config suite:   PASS (27/27, 3 skipped)
  Config live:    PASS (35/35, 3 skipped)
  Docs:           PASS (all commands documented, no stale refs)
```
