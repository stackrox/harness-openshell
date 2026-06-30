package orchestrator

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "orchestrator.yaml")
	os.WriteFile(path, []byte(`
mode: watch
entrypoint: claude
task: task.md
sentinel: true
poll_interval: 600
max_failures: 3
heartbeat: 30
`), 0o644)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Mode != "watch" {
		t.Errorf("mode = %q, want watch", cfg.Mode)
	}
	if cfg.Entrypoint != "claude" {
		t.Errorf("entrypoint = %q, want claude", cfg.Entrypoint)
	}
	if cfg.PollInterval != 600 {
		t.Errorf("poll_interval = %d, want 600", cfg.PollInterval)
	}
	if cfg.MaxFailures != 3 {
		t.Errorf("max_failures = %d, want 3", cfg.MaxFailures)
	}
	if cfg.Heartbeat != 30 {
		t.Errorf("heartbeat = %d, want 30", cfg.Heartbeat)
	}
	if !cfg.Sentinel {
		t.Error("sentinel = false, want true")
	}
}

func TestLoadConfigDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "orchestrator.yaml")
	os.WriteFile(path, []byte("entrypoint: claude\n"), 0o644)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Mode != "once" {
		t.Errorf("default mode = %q, want once", cfg.Mode)
	}
	if cfg.PollInterval != 300 {
		t.Errorf("default poll_interval = %d, want 300", cfg.PollInterval)
	}
	if cfg.MaxFailures != 5 {
		t.Errorf("default max_failures = %d, want 5", cfg.MaxFailures)
	}
	if cfg.Heartbeat != 60 {
		t.Errorf("default heartbeat = %d, want 60", cfg.Heartbeat)
	}
	if cfg.SessionDir != "/sandbox/.harness" {
		t.Errorf("default session_dir = %q, want /sandbox/.harness", cfg.SessionDir)
	}
}

func TestValidateInvalidMode(t *testing.T) {
	cfg := &OrchestratorConfig{Mode: "invalid", Entrypoint: "claude"}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for invalid mode")
	}
}

func TestValidateInvalidEntrypoint(t *testing.T) {
	cfg := &OrchestratorConfig{Mode: "once", Entrypoint: "unknown"}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for invalid entrypoint")
	}
}

func TestValidateValidEntrypoints(t *testing.T) {
	for _, ep := range []string{"claude", "codex", "opencode"} {
		cfg := &OrchestratorConfig{Mode: "once", Entrypoint: ep}
		if err := cfg.Validate(); err != nil {
			t.Errorf("entrypoint %q: unexpected error: %v", ep, err)
		}
	}
}
