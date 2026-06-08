package profile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mockProviderChecker implements profile.ProviderChecker for testing.
type mockProviderChecker struct {
	providers map[string]bool
}

func (m *mockProviderChecker) ProviderGet(name string) error {
	if m.providers[name] {
		return nil
	}
	return fmt.Errorf("not found")
}

func TestParseFile_Full(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	os.WriteFile(path, []byte(`
name = "research"
from = "quay.io/test/sandbox:latest"
command = "claude --bare --model opus"
keep = false
providers = ["github", "vertex-local"]

[env]
ANTHROPIC_BASE_URL = "https://inference.local"
JIRA_URL = "https://example.atlassian.net"
`), 0o644)

	cfg, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if cfg.Name != "research" {
		t.Errorf("Name = %q, want %q", cfg.Name, "research")
	}
	if cfg.From != "quay.io/test/sandbox:latest" {
		t.Errorf("From = %q", cfg.From)
	}
	if cfg.Command != "claude --bare --model opus" {
		t.Errorf("Command = %q", cfg.Command)
	}
	if cfg.KeepSandbox() {
		t.Error("KeepSandbox() = true, want false")
	}
	if len(cfg.Providers) != 2 || cfg.Providers[0] != "github" {
		t.Errorf("Providers = %v", cfg.Providers)
	}
	if cfg.Env["JIRA_URL"] != "https://example.atlassian.net" {
		t.Errorf("Env[JIRA_URL] = %q", cfg.Env["JIRA_URL"])
	}
}

func TestParseFile_Defaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	os.WriteFile(path, []byte(`image = "quay.io/test:latest"`), 0o644)

	cfg, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if cfg.Name != "agent" {
		t.Errorf("Name = %q, want default 'agent'", cfg.Name)
	}
	if cfg.Command != "claude" {
		t.Errorf("Command = %q, want default", cfg.Command)
	}
	if !cfg.KeepSandbox() {
		t.Error("KeepSandbox() = false, want true (default)")
	}
}

func TestParseFile_Missing(t *testing.T) {
	_, err := ParseFile("/nonexistent.toml")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestParse_ByName(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "profiles"), 0o755)
	os.WriteFile(filepath.Join(dir, "profiles", "test.toml"), []byte(`
name = "test-agent"
image = "test:latest"
`), 0o644)

	cfg, err := Parse(dir, "test")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.Name != "test-agent" {
		t.Errorf("Name = %q", cfg.Name)
	}
}

func TestBuildSandboxEnv(t *testing.T) {
	cfg := &Config{
		Env: map[string]string{
			"ZEBRA": "z",
			"APPLE": "a",
		},
	}
	env := cfg.BuildSandboxEnv()
	lines := strings.Split(strings.TrimSpace(env), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %q", len(lines), env)
	}
	if lines[0] != `export APPLE="a"` {
		t.Errorf("first line = %q (should be sorted and quoted)", lines[0])
	}
	if lines[1] != `export ZEBRA="z"` {
		t.Errorf("second line = %q", lines[1])
	}
}

func TestBuildSandboxEnv_Empty(t *testing.T) {
	cfg := &Config{}
	if env := cfg.BuildSandboxEnv(); env != "" {
		t.Errorf("expected empty, got %q", env)
	}
}

func TestStageHarnessDir(t *testing.T) {
	dir := t.TempDir()
	harnessDir := filepath.Join(dir, "harness")

	cfg := &Config{
		Env: map[string]string{"FOO": "bar"},
	}
	if err := StageHarnessDir(cfg, harnessDir); err != nil {
		t.Fatalf("StageHarnessDir: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(harnessDir, "sandbox.env"))
	if err != nil {
		t.Fatalf("reading sandbox.env: %v", err)
	}
	if !strings.Contains(string(data), `export FOO="bar"`) {
		t.Errorf("sandbox.env = %q", string(data))
	}
}

func TestValidateProviders_AllRegistered(t *testing.T) {
	gw := &mockProviderChecker{providers: map[string]bool{"github": true, "vertex-local": true}}
	reg, missing := ValidateProviders([]string{"github", "vertex-local"}, gw)
	if len(reg) != 2 {
		t.Errorf("registered = %v, want 2", reg)
	}
	if len(missing) != 0 {
		t.Errorf("missing = %v, want empty", missing)
	}
}

func TestValidateProviders_SomeMissing(t *testing.T) {
	gw := &mockProviderChecker{providers: map[string]bool{"github": true}}
	reg, missing := ValidateProviders([]string{"github", "vertex-local", "atlassian"}, gw)
	if len(reg) != 1 || reg[0] != "github" {
		t.Errorf("registered = %v", reg)
	}
	if len(missing) != 2 {
		t.Errorf("missing = %v, want 2 items", missing)
	}
}

func TestValidateProviders_NoneRegistered(t *testing.T) {
	gw := &mockProviderChecker{providers: map[string]bool{}}
	reg, missing := ValidateProviders([]string{"github", "vertex-local"}, gw)
	if len(reg) != 0 {
		t.Errorf("registered = %v, want empty", reg)
	}
	if len(missing) != 2 {
		t.Errorf("missing = %v, want 2 items", missing)
	}
}
