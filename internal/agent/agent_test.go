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
  - profile: atlassian
    env:
      JIRA_USERNAME: alice@example.com
      JIRA_URL: https://issues.redhat.com
  - profile: github
task: tasks/daily-standup.md
entrypoint: claude
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
	if cfg.Providers[0].Profile != "atlassian" {
		t.Errorf("Providers[0].Profile = %q, want atlassian", cfg.Providers[0].Profile)
	}
	if cfg.Providers[0].Env["JIRA_USERNAME"] != "alice@example.com" {
		t.Errorf("JIRA_USERNAME = %q", cfg.Providers[0].Env["JIRA_USERNAME"])
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

func TestParse_MissingProviderProfile(t *testing.T) {
	data := []byte(`
name: test
providers:
  - env:
      FOO: bar
`)
	_, err := Parse(data)
	if err == nil {
		t.Fatal("expected error for missing provider profile")
	}
	if !strings.Contains(err.Error(), "profile is required") {
		t.Errorf("error = %q, want 'profile is required'", err)
	}
}

func TestParse_InvalidYAML(t *testing.T) {
	data := []byte(`name: [invalid yaml`)
	_, err := Parse(data)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
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
			{Profile: "github"},
			{Profile: "atlassian"},
			{Profile: "gws"},
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
	if ep := cfg.EffectiveEntrypoint(); ep != "claude" {
		t.Errorf("default = %q, want 'claude'", ep)
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

func TestBuildEnvMap_IncludesProviderEnv(t *testing.T) {
	t.Setenv("JIRA_URL", "https://test.atlassian.net")
	cfg := &AgentConfig{
		Env: map[string]string{"TOP": "top-val"},
		Providers: []ProviderRef{
			{Profile: "atlassian", Env: map[string]string{
				"JIRA_URL":      "${JIRA_URL}",
				"JIRA_USERNAME": "alice",
			}},
		},
	}
	env := cfg.BuildEnvMap()
	if env["TOP"] != "top-val" {
		t.Errorf("TOP = %q", env["TOP"])
	}
	if env["JIRA_URL"] != "https://test.atlassian.net" {
		t.Errorf("JIRA_URL = %q, want expanded value", env["JIRA_URL"])
	}
	if env["JIRA_USERNAME"] != "alice" {
		t.Errorf("JIRA_USERNAME = %q", env["JIRA_USERNAME"])
	}
}

func TestBuildEnvMap_ProviderEnvOverridesTopLevel(t *testing.T) {
	cfg := &AgentConfig{
		Env:       map[string]string{"SHARED": "from-top"},
		Providers: []ProviderRef{{Profile: "test", Env: map[string]string{"SHARED": "from-provider"}}},
	}
	env := cfg.BuildEnvMap()
	if env["SHARED"] != "from-provider" {
		t.Errorf("SHARED = %q, want from-provider (provider env should override top-level)", env["SHARED"])
	}
}

func TestParseFile_AgentYAML(t *testing.T) {
	dir := t.TempDir()
	yamlContent := `name: test-agent
image: ghcr.io/test:latest
entrypoint: claude
tty: true
providers:
  - profile: github
  - profile: atlassian
    env:
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

func TestBuildEnvMap(t *testing.T) {
	cfg := &AgentConfig{
		Env: map[string]string{
			"TOP_VAR": "top-val",
			"ANOTHER": "another-val",
		},
		Providers: []ProviderRef{
			{Profile: "atlassian", Env: map[string]string{
				"JIRA_URL": "https://issues.redhat.com",
			}},
		},
	}
	env := cfg.BuildEnvMap()
	if env["TOP_VAR"] != "top-val" {
		t.Errorf("TOP_VAR = %q, want top-val", env["TOP_VAR"])
	}
	if env["ANOTHER"] != "another-val" {
		t.Errorf("ANOTHER = %q", env["ANOTHER"])
	}
	if env["JIRA_URL"] != "https://issues.redhat.com" {
		t.Errorf("JIRA_URL = %q, want provider env included", env["JIRA_URL"])
	}
}

func TestBuildEnvMap_EmptyValueReadsFromHost(t *testing.T) {
	t.Setenv("MY_HOST_VAR", "from-host")
	cfg := &AgentConfig{
		Env: map[string]string{
			"MY_HOST_VAR": "",
		},
	}
	env := cfg.BuildEnvMap()
	if env["MY_HOST_VAR"] != "from-host" {
		t.Errorf("MY_HOST_VAR = %q, want from-host", env["MY_HOST_VAR"])
	}
}

func TestBuildEnvMap_EmptyValueNotInHost(t *testing.T) {
	cfg := &AgentConfig{
		Env: map[string]string{
			"NONEXISTENT_VAR_12345": "",
		},
	}
	env := cfg.BuildEnvMap()
	if _, ok := env["NONEXISTENT_VAR_12345"]; ok {
		t.Error("empty env var not in host should be omitted from map")
	}
}

func TestBuildEnvMap_Empty(t *testing.T) {
	cfg := &AgentConfig{Providers: []ProviderRef{{Profile: "github"}}}
	env := cfg.BuildEnvMap()
	if len(env) != 0 {
		t.Errorf("expected empty map, got %v", env)
	}
}

func TestBuildRunSh(t *testing.T) {
	cfg := &AgentConfig{
		Entrypoint: "claude",
		Task:       "tasks/standup.md",
	}
	runSh := cfg.BuildRunSh()
	if !strings.Contains(runSh, "#!/usr/bin/env bash") {
		t.Error("missing shebang")
	}
	if strings.Contains(runSh, "env.sh") {
		t.Error("run.sh should not source env.sh — env vars are injected via --env")
	}
	if !strings.Contains(runSh, `command -v "claude"`) {
		t.Error("missing entrypoint validation")
	}
	if !strings.Contains(runSh, `exec claude -p "$(cat "$PAYLOAD_DIR/task.md")"`) {
		t.Errorf("missing task exec with -p in:\n%s", runSh)
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
			{Profile: "atlassian", Env: map[string]string{"JIRA_URL": "https://jira.example.com"}},
		},
		Task:       "my-task.md",
		Entrypoint: "claude",
	}

	destDir := filepath.Join(t.TempDir(), "payload")
	if err := RenderPayload(cfg, baseDir, destDir); err != nil {
		t.Fatalf("RenderPayload: %v", err)
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
	if _, err := os.Stat(filepath.Join(destDir, "env.sh")); !os.IsNotExist(err) {
		t.Error("env.sh should not be created — env vars are injected via --env")
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
		Providers: []ProviderRef{{Profile: "github"}},
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
		Providers: []ProviderRef{{Profile: "github"}},
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
		Providers: []ProviderRef{{Profile: "github"}},
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
