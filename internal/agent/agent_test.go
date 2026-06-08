package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParse_Valid(t *testing.T) {
	data := []byte(`
name: daily-standup
providers:
  - type: atlassian
    config:
      JIRA_USERNAME: alice@example.com
      JIRA_URL: https://issues.redhat.com
  - type: github
task: tasks/daily-standup.md
entrypoint: claude --bare
`)
	cfg, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.Name != "daily-standup" {
		t.Errorf("Name = %q, want daily-standup", cfg.Name)
	}
	if len(cfg.Providers) != 2 {
		t.Fatalf("Providers = %d, want 2", len(cfg.Providers))
	}
	if cfg.Providers[0].Type != "atlassian" {
		t.Errorf("Providers[0].Type = %q, want atlassian", cfg.Providers[0].Type)
	}
	if cfg.Providers[0].Config["JIRA_USERNAME"] != "alice@example.com" {
		t.Errorf("JIRA_USERNAME = %q", cfg.Providers[0].Config["JIRA_USERNAME"])
	}
	if cfg.Task != "tasks/daily-standup.md" {
		t.Errorf("Task = %q", cfg.Task)
	}
}

func TestParse_MissingName(t *testing.T) {
	data := []byte(`providers: [{type: github}]`)
	_, err := Parse(data)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
	if !strings.Contains(err.Error(), "name is required") {
		t.Errorf("error = %q, want 'name is required'", err)
	}
}

func TestParse_MissingProviderType(t *testing.T) {
	data := []byte(`
name: test
providers:
  - config:
      FOO: bar
`)
	_, err := Parse(data)
	if err == nil {
		t.Fatal("expected error for missing provider type")
	}
	if !strings.Contains(err.Error(), "type is required") {
		t.Errorf("error = %q, want 'type is required'", err)
	}
}

func TestParse_InvalidYAML(t *testing.T) {
	data := []byte(`name: [invalid yaml`)
	_, err := Parse(data)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
	if !strings.Contains(err.Error(), "parsing agent config") {
		t.Errorf("error = %q, want 'parsing agent config'", err)
	}
}

func TestParseFile_NotFound(t *testing.T) {
	_, err := ParseFile("/nonexistent/agent.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestParse_EmptyProviders(t *testing.T) {
	data := []byte(`
name: minimal
providers: []
`)
	cfg, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(cfg.Providers) != 0 {
		t.Errorf("Providers = %d, want 0", len(cfg.Providers))
	}
}

func TestProviderNames(t *testing.T) {
	cfg := &AgentConfig{
		Providers: []ProviderRef{
			{Type: "github"},
			{Type: "atlassian"},
			{Type: "gws"},
		},
	}
	names := cfg.ProviderNames()
	if len(names) != 3 {
		t.Fatalf("len = %d, want 3", len(names))
	}
	if names[0] != "github" || names[1] != "atlassian" || names[2] != "gws" {
		t.Errorf("names = %v", names)
	}
}

func TestEffectiveEntrypoint(t *testing.T) {
	cfg := &AgentConfig{}
	if ep := cfg.EffectiveEntrypoint(); ep != "claude --bare" {
		t.Errorf("default = %q, want 'claude --bare'", ep)
	}
	cfg.Entrypoint = "codex"
	if ep := cfg.EffectiveEntrypoint(); ep != "codex" {
		t.Errorf("custom = %q, want 'codex'", ep)
	}
}

func TestNoTTY(t *testing.T) {
	cfg := &AgentConfig{}
	if cfg.NoTTY() {
		t.Error("nil TTY should default to false (interactive)")
	}
	f := false
	cfg.TTY = &f
	if !cfg.NoTTY() {
		t.Error("TTY=false should return NoTTY=true")
	}
	tr := true
	cfg.TTY = &tr
	if cfg.NoTTY() {
		t.Error("TTY=true should return NoTTY=false")
	}
}

func TestBuildEnvSh(t *testing.T) {
	cfg := &AgentConfig{
		Providers: []ProviderRef{
			{Type: "atlassian", Config: map[string]string{
				"JIRA_URL":      "https://issues.redhat.com",
				"JIRA_USERNAME": "alice",
			}},
		},
	}
	env := cfg.BuildEnvSh()
	if !strings.Contains(env, `export JIRA_URL="https://issues.redhat.com"`) {
		t.Errorf("missing JIRA_URL in:\n%s", env)
	}
	if !strings.Contains(env, `export JIRA_USERNAME="alice"`) {
		t.Errorf("missing JIRA_USERNAME in:\n%s", env)
	}
}

func TestBuildEnvSh_Empty(t *testing.T) {
	cfg := &AgentConfig{Providers: []ProviderRef{{Type: "github"}}}
	if env := cfg.BuildEnvSh(); env != "" {
		t.Errorf("expected empty env.sh, got:\n%s", env)
	}
}

func TestBuildEnvSh_TopLevelEnv(t *testing.T) {
	cfg := &AgentConfig{
		Env: map[string]string{
			"ANTHROPIC_BASE_URL": "https://inference.local",
			"ANTHROPIC_API_KEY":  "sk-proxy",
		},
		Providers: []ProviderRef{
			{Type: "atlassian", Config: map[string]string{"JIRA_URL": "https://jira.example.com"}},
		},
	}
	env := cfg.BuildEnvSh()
	if !strings.Contains(env, `export ANTHROPIC_BASE_URL="https://inference.local"`) {
		t.Errorf("missing top-level env var in:\n%s", env)
	}
	if !strings.Contains(env, `export JIRA_URL="https://jira.example.com"`) {
		t.Errorf("missing provider config var in:\n%s", env)
	}
}

func TestBuildEnvSh_ProviderOverridesTopLevel(t *testing.T) {
	cfg := &AgentConfig{
		Env:       map[string]string{"FOO": "from-top"},
		Providers: []ProviderRef{{Type: "test", Config: map[string]string{"FOO": "from-provider"}}},
	}
	env := cfg.BuildEnvSh()
	if !strings.Contains(env, `"from-provider"`) {
		t.Errorf("provider config should override top-level env:\n%s", env)
	}
}

func TestParseFile_AgentYAML(t *testing.T) {
	dir := t.TempDir()
	yamlContent := `name: test-agent
image: ghcr.io/test:latest
entrypoint: claude --bare
tty: true
providers:
  - type: github
  - type: atlassian
    config:
      JIRA_URL: https://jira.example.com
env:
  ANTHROPIC_BASE_URL: https://inference.local
`
	os.WriteFile(filepath.Join(dir, "agent.yaml"), []byte(yamlContent), 0o644)

	cfg, err := ParseFile(filepath.Join(dir, "agent.yaml"))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if cfg.Name != "test-agent" {
		t.Errorf("Name = %q", cfg.Name)
	}
	if cfg.Image != "ghcr.io/test:latest" {
		t.Errorf("Image = %q", cfg.Image)
	}
	if cfg.Env["ANTHROPIC_BASE_URL"] != "https://inference.local" {
		t.Errorf("Env ANTHROPIC_BASE_URL = %q", cfg.Env["ANTHROPIC_BASE_URL"])
	}
	if len(cfg.Providers) != 2 {
		t.Errorf("Providers = %d, want 2", len(cfg.Providers))
	}
}

func TestBuildEnvSh_Sorted(t *testing.T) {
	cfg := &AgentConfig{
		Providers: []ProviderRef{
			{Type: "test", Config: map[string]string{"Z_VAR": "z", "A_VAR": "a"}},
		},
	}
	env := cfg.BuildEnvSh()
	aIdx := strings.Index(env, "A_VAR")
	zIdx := strings.Index(env, "Z_VAR")
	if aIdx > zIdx {
		t.Error("env.sh should be sorted alphabetically")
	}
}

func TestBuildRunSh(t *testing.T) {
	cfg := &AgentConfig{
		Entrypoint: "claude --bare",
		Task:       "tasks/standup.md",
	}
	runSh := cfg.BuildRunSh()
	if !strings.Contains(runSh, "#!/usr/bin/env bash") {
		t.Error("missing shebang")
	}
	if !strings.Contains(runSh, ". \"$PAYLOAD_DIR/env.sh\"") {
		t.Error("missing env.sh source")
	}
	if !strings.Contains(runSh, "gh auth setup-git") {
		t.Error("missing gh auth setup-git")
	}
	if !strings.Contains(runSh, `command -v "claude"`) {
		t.Error("missing entrypoint validation")
	}
	if !strings.Contains(runSh, `exec claude --bare "$PAYLOAD_DIR/task.md"`) {
		t.Errorf("missing task exec in:\n%s", runSh)
	}
}

func TestBuildRunSh_NoTask(t *testing.T) {
	cfg := &AgentConfig{Entrypoint: "codex"}
	runSh := cfg.BuildRunSh()
	if !strings.Contains(runSh, "exec codex\n") {
		t.Errorf("expected bare exec, got:\n%s", runSh)
	}
	if strings.Contains(runSh, "task.md") {
		t.Error("should not reference task.md when no task set")
	}
}

func TestRenderPayload(t *testing.T) {
	baseDir := t.TempDir()
	os.WriteFile(filepath.Join(baseDir, "my-task.md"), []byte("Do the thing: ${USER}"), 0o644)

	cfg := &AgentConfig{
		Name: "test-agent",
		Providers: []ProviderRef{
			{Type: "atlassian", Config: map[string]string{"JIRA_URL": "https://jira.example.com"}},
		},
		Task:       "my-task.md",
		Entrypoint: "claude --bare",
	}

	destDir := filepath.Join(t.TempDir(), "payload")
	if err := RenderPayload(cfg, baseDir, destDir); err != nil {
		t.Fatalf("RenderPayload: %v", err)
	}

	if _, err := os.Stat(filepath.Join(destDir, "env.sh")); err != nil {
		t.Error("missing env.sh")
	}
	if _, err := os.Stat(filepath.Join(destDir, "run.sh")); err != nil {
		t.Error("missing run.sh")
	}
	if _, err := os.Stat(filepath.Join(destDir, "task.md")); err != nil {
		t.Error("missing task.md")
	}
	if _, err := os.Stat(filepath.Join(destDir, "bin")); err != nil {
		t.Error("missing bin/ directory")
	}

	envData, _ := os.ReadFile(filepath.Join(destDir, "env.sh"))
	if !strings.Contains(string(envData), "JIRA_URL") {
		t.Errorf("env.sh missing JIRA_URL:\n%s", envData)
	}

	runData, _ := os.ReadFile(filepath.Join(destDir, "run.sh"))
	if !strings.Contains(string(runData), "exec claude") {
		t.Errorf("run.sh missing entrypoint:\n%s", runData)
	}

	taskData, _ := os.ReadFile(filepath.Join(destDir, "task.md"))
	if strings.Contains(string(taskData), "${USER}") {
		t.Error("task.md should have envsubst applied")
	}
}

func TestRenderPayload_NoEnv(t *testing.T) {
	cfg := &AgentConfig{
		Name:      "minimal",
		Providers: []ProviderRef{{Type: "github"}},
	}

	destDir := filepath.Join(t.TempDir(), "payload")
	if err := RenderPayload(cfg, t.TempDir(), destDir); err != nil {
		t.Fatalf("RenderPayload: %v", err)
	}

	if _, err := os.Stat(filepath.Join(destDir, "env.sh")); !os.IsNotExist(err) {
		t.Error("env.sh should not exist when no config vars")
	}
	if _, err := os.Stat(filepath.Join(destDir, "run.sh")); err != nil {
		t.Error("run.sh should always be created")
	}
}

func TestRenderPayload_Include(t *testing.T) {
	baseDir := t.TempDir()
	os.WriteFile(filepath.Join(baseDir, "helper.sh"), []byte("echo hi"), 0o644)

	cfg := &AgentConfig{
		Name:      "with-include",
		Providers: []ProviderRef{{Type: "github"}},
		Include:   []string{"helper.sh"},
	}

	destDir := filepath.Join(t.TempDir(), "payload")
	if err := RenderPayload(cfg, baseDir, destDir); err != nil {
		t.Fatalf("RenderPayload: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(destDir, "helper.sh"))
	if err != nil {
		t.Fatal("missing included file")
	}
	if string(data) != "echo hi" {
		t.Errorf("include content = %q", data)
	}
}

func TestRenderPayload_IncludePathTraversal(t *testing.T) {
	baseDir := t.TempDir()
	cfg := &AgentConfig{
		Name:      "evil",
		Providers: []ProviderRef{{Type: "github"}},
		Include:   []string{"../../etc/passwd"},
	}

	destDir := filepath.Join(t.TempDir(), "payload")
	err := RenderPayload(cfg, baseDir, destDir)
	if err == nil {
		t.Fatal("expected error for path traversal")
	}
	if !strings.Contains(err.Error(), "escapes base directory") {
		t.Errorf("error = %q, want 'escapes base directory'", err)
	}
}
