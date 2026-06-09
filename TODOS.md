# TODOs

## registerProviders should filter by agent's provider list

**What:** `registerProviders()` in `cmd/providers.go` uses the gateway config's provider
list, not the agent config's. When `gwCfg` is nil (common case), it tries to register
all providers regardless of what the agent needs.

**Why:** Confusing output -- users see "skipped" messages for providers their agent
doesn't reference. No functional impact (missing credentials are silently handled).

**Fix:** Pass the agent's provider names to `registerProviders` and use them as a
filter alongside (or instead of) the gateway config's list.

**Files:** `cmd/providers.go` (registerProviders signature), `cmd/up.go` (call site)

**Depends on:** Nothing. Can be done independently.
