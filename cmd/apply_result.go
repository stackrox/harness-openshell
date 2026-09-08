package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/stackrox/harness-openshell/internal/source"
)

// applyResult deliberately excludes configuration, raw errors, and agent output:
// each can contain credentials. SourceCommit comes only from host preparation.
type applyResult struct {
	Version        int       `json:"version"`
	RunID          string    `json:"runId"`
	Status         string    `json:"status"`
	Phase          string    `json:"phase"`
	StartedAt      time.Time `json:"startedAt"`
	FinishedAt     time.Time `json:"finishedAt"`
	DurationMillis int64     `json:"durationMillis"`
	SourceCommit   string    `json:"sourceCommit,omitempty"`
	started        time.Time
}

func startApplyResult(path string) (*applyResult, *os.File, error) {
	id, err := source.NewRunID()
	if err != nil {
		return nil, nil, err
	}
	// Refuse overwrites and symlinks before making any gateway mutations.
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, nil, fmt.Errorf("creating result file: %w", err)
	}
	started := time.Now()
	return &applyResult{Version: 1, RunID: id, StartedAt: started.UTC(), Phase: "load", started: started}, file, nil
}

func (r *applyResult) setPhase(phase string) {
	if r != nil {
		r.Phase = phase
	}
}

func (r *applyResult) finish(ctx context.Context, file *os.File, runErr error) error {
	finished := time.Now()
	r.FinishedAt = finished.UTC()
	r.DurationMillis = finished.Sub(r.started).Milliseconds()
	switch {
	case runErr == nil:
		r.Status, r.Phase = "succeeded", "complete"
	case errors.Is(runErr, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded):
		r.Status = "timed_out"
	case errors.Is(runErr, context.Canceled) || errors.Is(ctx.Err(), context.Canceled):
		r.Status = "cancelled"
	default:
		r.Status = "failed"
	}
	encodeErr := json.NewEncoder(file).Encode(r)
	closeErr := file.Close()
	if err := errors.Join(encodeErr, closeErr); err != nil {
		return fmt.Errorf("writing result file: %w", err)
	}
	return nil
}
