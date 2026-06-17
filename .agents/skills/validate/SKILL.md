---
name: validate
description: Run the full test matrix for harness-openshell. Use when asked to "validate", "run tests", "check everything", or before any commit/push/PR.
---

# Validate

Run the full test matrix. Skip steps that require unavailable infrastructure.

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

## Output

Report a summary table:

```
Validation Results
──────────────────
  Build:          PASS
  Unit tests:     PASS (6 packages)
  Vet:            PASS
  Local (full):   PASS (22/22)
  Local (CI):     PASS (14/14)
  OCP:            PASS (10/10)
  Kind:           SKIP (kind not installed)
  CI:             GREEN (3/3 workflows)
```
