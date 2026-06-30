package orchestrator

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestRunOnceSuccess(t *testing.T) {
	dir := t.TempDir()
	cfg := &OrchestratorConfig{
		Mode:       "once",
		Entrypoint: "claude",
		SessionDir: dir,
	}
	cfg.ApplyDefaults()

	orch := &Orchestrator{
		config:    cfg,
		adapter:   &mockAdapter{script: "true"},
		configDir: dir,
	}
	sw, _ := NewSessionWriter(dir)
	orch.session = sw

	err := orch.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	records := readRecords(t, dir)
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}
	if records[0].Result.Status != "complete" {
		t.Errorf("status = %q, want complete", records[0].Result.Status)
	}
}

func TestRunOnceFailure(t *testing.T) {
	dir := t.TempDir()
	cfg := &OrchestratorConfig{
		Mode:       "once",
		Entrypoint: "claude",
		SessionDir: dir,
	}
	cfg.ApplyDefaults()

	orch := &Orchestrator{
		config:    cfg,
		adapter:   &mockAdapter{script: "exit 1"},
		configDir: dir,
	}
	sw, _ := NewSessionWriter(dir)
	orch.session = sw

	err := orch.Run(context.Background())
	if err == nil {
		t.Fatal("expected error for exit code 1")
	}
}

func TestRunOnceSentinel(t *testing.T) {
	dir := t.TempDir()
	cfg := &OrchestratorConfig{
		Mode:       "once",
		Entrypoint: "claude",
		Sentinel:   true,
		SessionDir: dir,
	}
	cfg.ApplyDefaults()

	sentinel := `OPENSHELL_AGENT_RESULT {"status":"complete","reason":"merged"}`
	orch := &Orchestrator{
		config:    cfg,
		adapter:   &mockAdapter{script: fmt.Sprintf("echo 'working...'; echo '%s'", sentinel)},
		configDir: dir,
	}
	sw, _ := NewSessionWriter(dir)
	orch.session = sw

	err := orch.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	records := readRecords(t, dir)
	if records[0].Result.Reason != "merged" {
		t.Errorf("reason = %q, want merged", records[0].Result.Reason)
	}
}

func TestRunWatchStopsOnComplete(t *testing.T) {
	dir := t.TempDir()
	cfg := &OrchestratorConfig{
		Mode:         "watch",
		Entrypoint:   "claude",
		Sentinel:     true,
		PollInterval: 1,
		Heartbeat:    -1,
		SessionDir:   dir,
	}

	sentinels := []string{
		`OPENSHELL_AGENT_RESULT {"status":"waiting","reason":"pending","next_poll_seconds":1}`,
		`OPENSHELL_AGENT_RESULT {"status":"complete","reason":"done"}`,
	}

	orch := &Orchestrator{
		config:    cfg,
		adapter:   &multiMockAdapter{sentinels: sentinels},
		configDir: dir,
	}
	sw, _ := NewSessionWriter(dir)
	orch.session = sw

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := orch.Run(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	records := readRecords(t, dir)
	if len(records) != 2 {
		t.Fatalf("records = %d, want 2", len(records))
	}
	if records[0].Result.Status != "waiting" {
		t.Errorf("cycle 1 status = %q, want waiting", records[0].Result.Status)
	}
	if records[1].Result.Status != "complete" {
		t.Errorf("cycle 2 status = %q, want complete", records[1].Result.Status)
	}
}

func TestRunWatchContextCancel(t *testing.T) {
	dir := t.TempDir()
	cfg := &OrchestratorConfig{
		Mode:         "watch",
		Entrypoint:   "claude",
		Sentinel:     true,
		PollInterval: 300,
		Heartbeat:    -1,
		SessionDir:   dir,
	}

	sentinel := `OPENSHELL_AGENT_RESULT {"status":"waiting","reason":"forever","next_poll_seconds":300}`
	orch := &Orchestrator{
		config:    cfg,
		adapter:   &mockAdapter{script: fmt.Sprintf("echo '%s'", sentinel)},
		configDir: dir,
	}
	sw, _ := NewSessionWriter(dir)
	orch.session = sw

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := orch.Run(ctx)
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}

type mockAdapter struct {
	script string
}

func (m *mockAdapter) Name() string { return "mock" }

func (m *mockAdapter) BuildCommand(_ string, _ bool) *exec.Cmd {
	return exec.Command("sh", "-c", m.script)
}

type multiMockAdapter struct {
	sentinels []string
	idx       int
}

func (m *multiMockAdapter) Name() string { return "mock" }

func (m *multiMockAdapter) BuildCommand(_ string, _ bool) *exec.Cmd {
	var script string
	if m.idx < len(m.sentinels) {
		script = fmt.Sprintf("echo '%s'", m.sentinels[m.idx])
		m.idx++
	} else {
		script = `echo 'OPENSHELL_AGENT_RESULT {"status":"terminal_failure","reason":"out_of_results"}'`
	}
	return exec.Command("sh", "-c", script)
}

func readRecords(t *testing.T, dir string) []SessionRecord {
	t.Helper()
	f, err := os.Open(filepath.Join(dir, "sessions.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	var records []SessionRecord
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var r SessionRecord
		if err := json.Unmarshal(scanner.Bytes(), &r); err != nil {
			t.Fatal(err)
		}
		records = append(records, r)
	}
	return records
}
