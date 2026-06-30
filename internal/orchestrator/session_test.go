package orchestrator

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSessionWriterCreatesFile(t *testing.T) {
	dir := t.TempDir()
	w, err := NewSessionWriter(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	path := filepath.Join(dir, "sessions.jsonl")
	if _, err := os.Stat(path); err != nil {
		t.Errorf("session file not created: %v", err)
	}
}

func TestSessionWriterWriteAndRead(t *testing.T) {
	dir := t.TempDir()
	w, err := NewSessionWriter(dir)
	if err != nil {
		t.Fatal(err)
	}

	ts := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	record := SessionRecord{
		Timestamp:   ts,
		Cycle:       1,
		Entrypoint:  "claude",
		Mode:        "once",
		DurationSec: 42.5,
		Result:      &CycleResult{Status: "complete", Reason: "done"},
	}
	if err := w.Write(record); err != nil {
		t.Fatal(err)
	}
	w.Close()

	path := filepath.Join(dir, "sessions.jsonl")
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	if !scanner.Scan() {
		t.Fatal("no line in JSONL")
	}

	var got SessionRecord
	if err := json.Unmarshal(scanner.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Cycle != 1 {
		t.Errorf("cycle = %d, want 1", got.Cycle)
	}
	if got.Entrypoint != "claude" {
		t.Errorf("entrypoint = %q, want claude", got.Entrypoint)
	}
	if got.Result.Status != "complete" {
		t.Errorf("result.status = %q, want complete", got.Result.Status)
	}
}

func TestSessionWriterAppends(t *testing.T) {
	dir := t.TempDir()
	w, err := NewSessionWriter(dir)
	if err != nil {
		t.Fatal(err)
	}

	for i := 1; i <= 3; i++ {
		w.Write(SessionRecord{
			Timestamp:  time.Now(),
			Cycle:      i,
			Entrypoint: "claude",
			Mode:       "watch",
			Result:     &CycleResult{Status: "waiting"},
		})
	}
	w.Close()

	path := filepath.Join(dir, "sessions.jsonl")
	f, _ := os.Open(path)
	defer f.Close()

	count := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		count++
	}
	if count != 3 {
		t.Errorf("lines = %d, want 3", count)
	}
}

func TestSessionWriterCreatesSubdirs(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "dir")
	w, err := NewSessionWriter(dir)
	if err != nil {
		t.Fatal(err)
	}
	w.Close()

	if _, err := os.Stat(filepath.Join(dir, "sessions.jsonl")); err != nil {
		t.Errorf("file not created in nested dir: %v", err)
	}
}
