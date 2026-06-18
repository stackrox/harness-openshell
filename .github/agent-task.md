You are reviewing the harness-openshell repository at /sandbox/harness-openshell.

Your job: identify exactly ONE improvement and implement it. Not a list — one concrete change with a clear diff.

Priority order (pick the first that applies):

1. **Spec drift** — a field, command, or behavior documented in SPEC.md that doesn't match the Go code, or code behavior not reflected in the spec
2. **Test gap** — a code path in cmd/ or internal/ that has no test coverage and is easy to test (one test function, not a test suite)
3. **Doc inaccuracy** — something in README.md or profiles/README.md that is wrong or misleading given the current code
4. **Code simplification** — dead code, redundant checks, or unnecessarily complex logic that can be simplified without changing behavior

Do NOT:
- Touch .github/workflows/ or .goreleaser.yaml
- Add new features or change behavior
- Refactor for style preferences
- Make multiple unrelated changes

After making the change, run `go build ./...` to verify compilation. If you added or changed Go code, run `go test ./...` to verify tests pass.

Output the change as a git diff:
```
cd /sandbox/harness-openshell
git diff
```

The last thing you output should be the diff and a one-line summary of what you changed and why.
