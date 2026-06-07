package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseConfig_Full(t *testing.T) {
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

	cfg, err := parseConfig(path)
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if cfg.Name != "research" {
		t.Errorf("Name = %q, want %q", cfg.Name, "research")
	}
	if cfg.From != "quay.io/test/sandbox:latest" {
		t.Errorf("From = %q, want %q", cfg.From, "quay.io/test/sandbox:latest")
	}
	if cfg.Command != "claude --bare --model opus" {
		t.Errorf("Command = %q, want %q", cfg.Command, "claude --bare --model opus")
	}
	if cfg.KeepSandbox() {
		t.Error("KeepSandbox() = true, want false")
	}
	if len(cfg.Providers) != 2 {
		t.Fatalf("Providers len = %d, want 2", len(cfg.Providers))
	}
	if cfg.Providers[0] != "github" || cfg.Providers[1] != "vertex-local" {
		t.Errorf("Providers = %v", cfg.Providers)
	}
	if cfg.Env["ANTHROPIC_BASE_URL"] != "https://inference.local" {
		t.Errorf("Env[ANTHROPIC_BASE_URL] = %q", cfg.Env["ANTHROPIC_BASE_URL"])
	}
	if cfg.Env["JIRA_URL"] != "https://example.atlassian.net" {
		t.Errorf("Env[JIRA_URL] = %q", cfg.Env["JIRA_URL"])
	}
}

func TestParseConfig_Defaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	os.WriteFile(path, []byte(`
from = "quay.io/test/sandbox:latest"
`), 0o644)

	cfg, err := parseConfig(path)
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if cfg.Name != "agent" {
		t.Errorf("Name = %q, want %q (default)", cfg.Name, "agent")
	}
	if cfg.Command != "claude --bare" {
		t.Errorf("Command = %q, want %q (default)", cfg.Command, "claude --bare")
	}
	if !cfg.KeepSandbox() {
		t.Error("KeepSandbox() = false, want true (default)")
	}
	if len(cfg.Providers) != 0 {
		t.Errorf("Providers = %v, want empty", cfg.Providers)
	}
}

func TestParseConfig_Missing(t *testing.T) {
	_, err := parseConfig("/nonexistent/config.toml")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestParseConfig_Invalid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	os.WriteFile(path, []byte(`not valid toml {{{{`), 0o644)

	_, err := parseConfig(path)
	if err == nil {
		t.Error("expected error for invalid TOML")
	}
}

func TestStageFiles_EnvFromFile(t *testing.T) {
	dir := t.TempDir()
	harnessDir := filepath.Join(dir, "harness")
	envDir := filepath.Join(dir, "env")

	os.MkdirAll(envDir, 0o755)

	envContent := "export FOO=bar\nexport BAZ=qux\n"
	envFile := filepath.Join(envDir, "sandbox.env")
	os.WriteFile(envFile, []byte(envContent), 0o644)

	origStageFiles := stageFilesFrom
	stageFilesFrom = envFile
	defer func() { stageFilesFrom = origStageFiles }()

	if err := stageFiles(harnessDir); err != nil {
		t.Fatalf("stageFiles: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(harnessDir, "sandbox.env"))
	if err != nil {
		t.Fatalf("reading sandbox.env: %v", err)
	}
	if string(data) != envContent {
		t.Errorf("sandbox.env = %q, want %q", string(data), envContent)
	}
}

func TestStageFiles_NoEnv(t *testing.T) {
	dir := t.TempDir()
	harnessDir := filepath.Join(dir, "harness")

	origStageFiles := stageFilesFrom
	stageFilesFrom = "/nonexistent/sandbox.env"
	defer func() { stageFilesFrom = origStageFiles }()

	if err := stageFiles(harnessDir); err != nil {
		t.Fatalf("stageFiles: %v", err)
	}

	if _, err := os.Stat(filepath.Join(harnessDir, "sandbox.env")); err == nil {
		t.Error("sandbox.env should not exist when env file not present")
	}
}
