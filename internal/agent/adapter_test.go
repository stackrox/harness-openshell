package agent

import (
	"strings"
	"testing"
)

// TestAdapterForDispatch verifies the AdapterFor function routes correctly.
func TestAdapterForDispatch(t *testing.T) {
	tests := []struct {
		name       string
		entrypoint string
		wantType   string
	}{
		{"claude explicit", "claude", "claudeAdapter"},
		{"claude implicit (empty)", "", "claudeAdapter"},
		{"codex", "codex", "codexAdapter"},
		{"opencode", "opencode", "opencodeAdapter"},
		{"custom", "myagent", "customAdapter"},
		{"custom with args", "myagent --flag", "customAdapter"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := AdapterFor(tt.entrypoint)
			// Verify by checking the type name.
			gotType := typeOf(adapter)
			if gotType != tt.wantType {
				t.Errorf("AdapterFor(%q) returned %s, want %s", tt.entrypoint, gotType, tt.wantType)
			}
		})
	}
}

// TestEnvironmentAlwaysEmpty verifies all adapters return empty maps.
func TestEnvironmentAlwaysEmpty(t *testing.T) {
	adapters := map[string]AgentAdapter{
		"claude":   AdapterFor("claude"),
		"codex":    AdapterFor("codex"),
		"opencode": AdapterFor("opencode"),
		"custom":   AdapterFor("custom"),
	}

	cfg := &AgentConfig{Name: "test"}

	for name, adapter := range adapters {
		t.Run(name, func(t *testing.T) {
			env := adapter.Environment(cfg)
			if env == nil {
				t.Error("Environment() returned nil, want empty map")
			}
			if len(env) != 0 {
				t.Errorf("Environment() returned non-empty map: %v, want empty", env)
			}
		})
	}
}

// TestClaudeHeadlessWithTask verifies claude adapter emits --print form for headless+task.
func TestClaudeHeadlessWithTask(t *testing.T) {
	cfg := &AgentConfig{
		Name:       "test",
		Entrypoint: "claude",
		Task:       "task.md",
		TTY:        boolPtr(false), // headless
	}

	adapter := AdapterFor("claude")
	cmd, err := adapter.Command(cfg, SandboxTaskPath)
	if err != nil {
		t.Fatalf("Command() returned error: %v", err)
	}

	expected := []string{
		"bash",
		"-lc",
		`export PATH="/sandbox/.config/openshell/bin:$PATH"; if ! command -v claude >/dev/null 2>&1; then echo "ERROR: entrypoint claude not found in PATH" >&2; exit 1; fi; exec claude --print "$(cat /sandbox/.config/openshell/task.md)"`,
	}

	if !cmdEqual(cmd, expected) {
		t.Errorf("Command() mismatch\ngot:  %v\nwant: %v", cmd, expected)
	}
}

// TestClaudeInteractiveWithTask verifies claude adapter emits -p form for interactive+task.
func TestClaudeInteractiveWithTask(t *testing.T) {
	cfg := &AgentConfig{
		Name:       "test",
		Entrypoint: "claude",
		Task:       "task.md",
		TTY:        boolPtr(true), // interactive
	}

	adapter := AdapterFor("claude")
	cmd, err := adapter.Command(cfg, SandboxTaskPath)
	if err != nil {
		t.Fatalf("Command() returned error: %v", err)
	}

	expected := []string{
		"bash",
		"-lc",
		`export PATH="/sandbox/.config/openshell/bin:$PATH"; if ! command -v claude >/dev/null 2>&1; then echo "ERROR: entrypoint claude not found in PATH" >&2; exit 1; fi; exec claude -p "$(cat /sandbox/.config/openshell/task.md)"`,
	}

	if !cmdEqual(cmd, expected) {
		t.Errorf("Command() mismatch\ngot:  %v\nwant: %v", cmd, expected)
	}
}

// TestClaudeNoTask verifies claude adapter emits bare entrypoint when no task.
func TestClaudeNoTask(t *testing.T) {
	cfg := &AgentConfig{
		Name:       "test",
		Entrypoint: "claude",
		TTY:        boolPtr(false),
	}

	adapter := AdapterFor("claude")
	cmd, err := adapter.Command(cfg, "")
	if err != nil {
		t.Fatalf("Command() returned error: %v", err)
	}

	expected := []string{
		"bash",
		"-lc",
		`export PATH="/sandbox/.config/openshell/bin:$PATH"; if ! command -v claude >/dev/null 2>&1; then echo "ERROR: entrypoint claude not found in PATH" >&2; exit 1; fi; exec claude`,
	}

	if !cmdEqual(cmd, expected) {
		t.Errorf("Command() mismatch\ngot:  %v\nwant: %v", cmd, expected)
	}
}

// TestClaudeImplicitEntrypoint verifies implicit entrypoint (empty) defaults to claude.
func TestClaudeImplicitEntrypoint(t *testing.T) {
	cfg := &AgentConfig{
		Name:       "test",
		Entrypoint: "", // implicit
		Task:       "task.md",
		TTY:        boolPtr(false),
	}

	adapter := AdapterFor("") // "" dispatches to claude
	cmd, err := adapter.Command(cfg, SandboxTaskPath)
	if err != nil {
		t.Fatalf("Command() returned error: %v", err)
	}

	expected := []string{
		"bash",
		"-lc",
		`export PATH="/sandbox/.config/openshell/bin:$PATH"; if ! command -v claude >/dev/null 2>&1; then echo "ERROR: entrypoint claude not found in PATH" >&2; exit 1; fi; exec claude --print "$(cat /sandbox/.config/openshell/task.md)"`,
	}

	if !cmdEqual(cmd, expected) {
		t.Errorf("Command() mismatch\ngot:  %v\nwant: %v", cmd, expected)
	}
}

// TestOpenCodeHeadlessWithTask verifies opencode adapter emits run form for headless+task.
func TestOpenCodeHeadlessWithTask(t *testing.T) {
	cfg := &AgentConfig{
		Name:       "test",
		Entrypoint: "opencode",
		Task:       "task.md",
		TTY:        boolPtr(false), // headless
	}

	adapter := AdapterFor("opencode")
	cmd, err := adapter.Command(cfg, SandboxTaskPath)
	if err != nil {
		t.Fatalf("Command() returned error: %v", err)
	}

	expected := []string{
		"bash",
		"-lc",
		`export PATH="/sandbox/.config/openshell/bin:$PATH"; if ! command -v opencode >/dev/null 2>&1; then echo "ERROR: entrypoint opencode not found in PATH" >&2; exit 1; fi; exec opencode run "$(cat /sandbox/.config/openshell/task.md)"`,
	}

	if !cmdEqual(cmd, expected) {
		t.Errorf("Command() mismatch\ngot:  %v\nwant: %v", cmd, expected)
	}
}

// TestOpenCodeInteractiveWithTask verifies opencode uses -p for interactive mode.
func TestOpenCodeInteractiveWithTask(t *testing.T) {
	cfg := &AgentConfig{
		Name:       "test",
		Entrypoint: "opencode",
		Task:       "task.md",
		TTY:        boolPtr(true), // interactive
	}

	adapter := AdapterFor("opencode")
	cmd, err := adapter.Command(cfg, SandboxTaskPath)
	if err != nil {
		t.Fatalf("Command() returned error: %v", err)
	}

	expected := []string{
		"bash",
		"-lc",
		`export PATH="/sandbox/.config/openshell/bin:$PATH"; if ! command -v opencode >/dev/null 2>&1; then echo "ERROR: entrypoint opencode not found in PATH" >&2; exit 1; fi; exec opencode -p "$(cat /sandbox/.config/openshell/task.md)"`,
	}

	if !cmdEqual(cmd, expected) {
		t.Errorf("Command() mismatch\ngot:  %v\nwant: %v", cmd, expected)
	}
}

// TestCodexHeadlessWithTask verifies codex adapter emits --print form for headless+task.
func TestCodexHeadlessWithTask(t *testing.T) {
	cfg := &AgentConfig{
		Name:       "test",
		Entrypoint: "codex",
		Task:       "task.md",
		TTY:        boolPtr(false), // headless
	}

	adapter := AdapterFor("codex")
	cmd, err := adapter.Command(cfg, SandboxTaskPath)
	if err != nil {
		t.Fatalf("Command() returned error: %v", err)
	}

	expected := []string{
		"bash",
		"-lc",
		`export PATH="/sandbox/.config/openshell/bin:$PATH"; if ! command -v codex >/dev/null 2>&1; then echo "ERROR: entrypoint codex not found in PATH" >&2; exit 1; fi; exec codex --print "$(cat /sandbox/.config/openshell/task.md)"`,
	}

	if !cmdEqual(cmd, expected) {
		t.Errorf("Command() mismatch\ngot:  %v\nwant: %v", cmd, expected)
	}
}

// TestCustomHeadlessWithTask verifies custom adapter uses effective entrypoint.
func TestCustomHeadlessWithTask(t *testing.T) {
	cfg := &AgentConfig{
		Name:       "test",
		Entrypoint: "myagent",
		Task:       "task.md",
		TTY:        boolPtr(false), // headless
	}

	adapter := AdapterFor("myagent")
	cmd, err := adapter.Command(cfg, SandboxTaskPath)
	if err != nil {
		t.Fatalf("Command() returned error: %v", err)
	}

	expected := []string{
		"bash",
		"-lc",
		`export PATH="/sandbox/.config/openshell/bin:$PATH"; if ! command -v myagent >/dev/null 2>&1; then echo "ERROR: entrypoint myagent not found in PATH" >&2; exit 1; fi; exec myagent --print "$(cat /sandbox/.config/openshell/task.md)"`,
	}

	if !cmdEqual(cmd, expected) {
		t.Errorf("Command() mismatch\ngot:  %v\nwant: %v", cmd, expected)
	}
}

// TestCustomWithArgsHeadlessWithTask verifies custom adapter with args in entrypoint.
func TestCustomWithArgsHeadlessWithTask(t *testing.T) {
	cfg := &AgentConfig{
		Name:       "test",
		Entrypoint: "myagent --model=mini",
		Task:       "task.md",
		TTY:        boolPtr(false), // headless
	}

	adapter := AdapterFor("myagent --model=mini")
	cmd, err := adapter.Command(cfg, SandboxTaskPath)
	if err != nil {
		t.Fatalf("Command() returned error: %v", err)
	}

	expected := []string{
		"bash",
		"-lc",
		`export PATH="/sandbox/.config/openshell/bin:$PATH"; if ! command -v myagent >/dev/null 2>&1; then echo "ERROR: entrypoint myagent not found in PATH" >&2; exit 1; fi; exec myagent --model=mini --print "$(cat /sandbox/.config/openshell/task.md)"`,
	}

	if !cmdEqual(cmd, expected) {
		t.Errorf("Command() mismatch\ngot:  %v\nwant: %v", cmd, expected)
	}
}

// TestTaskPathUsesProvidedConstant verifies the command uses the passed taskPath.
func TestTaskPathUsesProvidedConstant(t *testing.T) {
	cfg := &AgentConfig{
		Name:       "test",
		Entrypoint: "claude",
		Task:       "task.md",
		TTY:        boolPtr(false),
	}

	adapter := AdapterFor("claude")
	cmd, err := adapter.Command(cfg, SandboxTaskPath)
	if err != nil {
		t.Fatalf("Command() returned error: %v", err)
	}

	// Verify the exact constant is used
	if len(cmd) != 3 || cmd[0] != "bash" || cmd[1] != "-lc" {
		t.Fatalf("Command structure wrong: %v", cmd)
	}
	cmdStr := cmd[2]

	if cmdStr != `export PATH="/sandbox/.config/openshell/bin:$PATH"; if ! command -v claude >/dev/null 2>&1; then echo "ERROR: entrypoint claude not found in PATH" >&2; exit 1; fi; exec claude --print "$(cat /sandbox/.config/openshell/task.md)"` {
		t.Errorf("Task path or command structure mismatch:\n%s", cmdStr)
	}
}

// TestEnvironmentReturnsNonNilMap verifies Environment() never returns nil.
func TestEnvironmentReturnsNonNilMap(t *testing.T) {
	adapters := []AgentAdapter{
		AdapterFor("claude"),
		AdapterFor("codex"),
		AdapterFor("opencode"),
		AdapterFor("custom"),
	}

	cfg := &AgentConfig{Name: "test"}

	for i, adapter := range adapters {
		env := adapter.Environment(cfg)
		if env == nil {
			t.Errorf("adapter %d returned nil from Environment(), want non-nil map", i)
		}
	}
}

// TestAnthropicBaseURLFromConfig verifies that ANTHROPIC_BASE_URL flows via BuildEnvMap.
// This test confirms the adapter does NOT add it, keeping env single-sourced.
func TestAnthropicBaseURLFromConfig(t *testing.T) {
	cfg := &AgentConfig{
		Name:       "test",
		Entrypoint: "claude",
		Env: map[string]string{
			"ANTHROPIC_BASE_URL": "http://inference.local",
		},
	}

	// BuildEnvMap should return it (config owned)
	envMap := cfg.BuildEnvMap()
	if envMap["ANTHROPIC_BASE_URL"] != "http://inference.local" {
		t.Errorf("BuildEnvMap() missing ANTHROPIC_BASE_URL, got: %v", envMap)
	}

	// Adapter should NOT add it (empty map)
	adapter := AdapterFor("claude")
	adapterEnv := adapter.Environment(cfg)
	if len(adapterEnv) > 0 {
		t.Errorf("adapter.Environment() should be empty but got: %v", adapterEnv)
	}
}

// TestCustomEntrypointRejectsShellMetacharacters verifies a custom entrypoint
// carrying shell metacharacters is rejected rather than embedded in the bash -lc
// script (the entrypoint reaches a shell, so it must be shell-safe).
func TestCustomEntrypointRejectsShellMetacharacters(t *testing.T) {
	for _, ep := range []string{
		"claude; rm -rf /",
		"claude && curl evil",
		"claude | tee /tmp/x",
		"claude $(whoami)",
		"claude `id`",
		"claude > /etc/passwd",
	} {
		cfg := &AgentConfig{Name: "test", Entrypoint: ep, Task: "task.md", TTY: boolPtr(false)}
		adapter := AdapterFor(cfg.EffectiveEntrypoint())
		if _, err := adapter.Command(cfg, SandboxTaskPath); err == nil {
			t.Errorf("entrypoint %q: expected error, got nil", ep)
		}
	}
}

// TestEntrypointEmptyOrWhitespaceRejected verifies buildCommand does not panic
// on a whitespace-only entrypoint and returns an error instead of indexing an
// empty strings.Fields result.
func TestEntrypointEmptyOrWhitespaceRejected(t *testing.T) {
	// A whitespace-only Entrypoint resolves to the default via EffectiveEntrypoint.
	cfg := &AgentConfig{Name: "test", Entrypoint: "   "}
	if got := cfg.EffectiveEntrypoint(); got != "claude" {
		t.Errorf("EffectiveEntrypoint() = %q, want claude for whitespace-only", got)
	}
	// buildCommand itself rejects an empty/whitespace entrypoint without panicking.
	if _, err := buildCommand("   ", cfg, ""); err == nil {
		t.Error("buildCommand(whitespace) = nil error, want error")
	}
}

// TestCustomEntrypointWithArgsAllowed verifies a legitimate command-plus-flags
// entrypoint (spaces and flag punctuation) is accepted.
func TestCustomEntrypointWithArgsAllowed(t *testing.T) {
	cfg := &AgentConfig{Name: "test", Entrypoint: "/usr/local/bin/my-agent --model gpt-4o", Task: "task.md", TTY: boolPtr(false)}
	adapter := AdapterFor(cfg.EffectiveEntrypoint())
	cmd, err := adapter.Command(cfg, SandboxTaskPath)
	if err != nil {
		t.Fatalf("Command() returned error for valid entrypoint: %v", err)
	}
	if len(cmd) != 3 || !strings.Contains(cmd[2], "command -v /usr/local/bin/my-agent") {
		t.Errorf("expected entrypoint bin validated in script, got: %v", cmd)
	}
	if !strings.Contains(cmd[2], "exec /usr/local/bin/my-agent --model gpt-4o --print") {
		t.Errorf("expected full entrypoint with args in exec, got: %s", cmd[2])
	}
}

// Helper functions

func boolPtr(b bool) *bool {
	return &b
}

func typeOf(a AgentAdapter) string {
	switch a.(type) {
	case *claudeAdapter:
		return "claudeAdapter"
	case *codexAdapter:
		return "codexAdapter"
	case *opencodeAdapter:
		return "opencodeAdapter"
	case *customAdapter:
		return "customAdapter"
	default:
		return "unknown"
	}
}

func cmdEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
