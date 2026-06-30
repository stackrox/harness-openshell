package orchestrator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type SessionRecord struct {
	Timestamp   time.Time    `json:"timestamp"`
	Cycle       int          `json:"cycle"`
	Entrypoint  string       `json:"entrypoint"`
	Mode        string       `json:"mode"`
	DurationSec float64      `json:"duration_sec"`
	Result      *CycleResult `json:"result"`
}

type SessionWriter struct {
	path string
	file *os.File
}

func NewSessionWriter(dir string) (*SessionWriter, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating session dir: %w", err)
	}
	path := filepath.Join(dir, "sessions.jsonl")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("opening session file: %w", err)
	}
	return &SessionWriter{path: path, file: f}, nil
}

func (w *SessionWriter) Write(record SessionRecord) error {
	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshaling session record: %w", err)
	}
	data = append(data, '\n')
	if _, err := w.file.Write(data); err != nil {
		return fmt.Errorf("writing session record: %w", err)
	}
	return nil
}

func (w *SessionWriter) Close() error {
	return w.file.Close()
}
