package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	fake "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/fake"
	"github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/types"
	"github.com/stackrox/harness-openshell/internal/openshell"
	"github.com/stackrox/harness-openshell/internal/testutil"
)

type resultSDK struct {
	*recordingSDK
	exitCode  int
	execErr   error
	deleteErr error
}

func (c *resultSDK) ExecSandbox(context.Context, string, []string, io.Writer, io.Writer) (int, error) {
	return c.exitCode, c.execErr
}

func (c *resultSDK) DeleteSandbox(context.Context, string) error {
	c.deleted = true
	return c.deleteErr
}

func TestApplyResultLifecycle(t *testing.T) {
	for _, tc := range []struct {
		name, status string
		exitCode     int
		execErr      error
		deleteErr    error
	}{
		{name: "success", status: "succeeded"},
		{name: "agent exit", status: "failed", exitCode: 42},
		{name: "transport", status: "failed", execErr: errors.New("secret-error-value")},
		{name: "cleanup", status: "failed", deleteErr: errors.New("secret-error-value")},
		{name: "cancel", status: "cancelled", execErr: context.Canceled},
		{name: "deadline", status: "timed_out", execErr: context.DeadlineExceeded},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			workflow := filepath.Join(dir, "workflow.yaml")
			writeTestFile(t, workflow, resultWorkflow)
			path := filepath.Join(dir, "result.json")
			sdk := &resultSDK{
				recordingSDK: &recordingSDK{Client: testutil.NewFake("default", fake.WithHealthResult(&types.HealthResult{Healthy: true}))},
				exitCode:     tc.exitCode, execErr: tc.execErr, deleteErr: tc.deleteErr,
			}
			cmd := NewApplyCmd(testutil.FakeFactory(sdk))
			cmd.SilenceErrors, cmd.SilenceUsage = true, true
			cmd.SetArgs([]string{"-f", workflow, "--result-file", path})
			_, err := captureStdout(t, cmd.Execute)
			if (err == nil) != (tc.status == "succeeded") {
				t.Fatalf("unexpected execution error: %v", err)
			}
			result := readApplyResult(t, path)
			if result.Status != tc.status || result.Version != 1 || len(result.RunID) != 32 {
				t.Fatalf("unexpected result: %+v", result)
			}
			phase := "execute"
			if tc.status == "succeeded" {
				phase = "complete"
			}
			if result.Phase != phase || !sdk.deleted {
				t.Fatalf("phase=%s deleted=%v", result.Phase, sdk.deleted)
			}
			if result.StartedAt.IsZero() || result.FinishedAt.Before(result.StartedAt) || result.DurationMillis < 0 {
				t.Fatalf("invalid timing: %+v", result)
			}
		})
	}
}

func TestApplyResultPreflight(t *testing.T) {
	for _, mode := range []string{"existing", "symlink", "dry-run", "output", "setup-only", "invalid-config", "plan-failure"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			workflow, path := filepath.Join(dir, "workflow.yaml"), filepath.Join(dir, "result.json")
			writeTestFile(t, workflow, resultWorkflow)
			req := applyRequest{File: workflow, ResultFile: path}
			switch mode {
			case "existing":
				writeTestFile(t, path, "preserve")
			case "symlink":
				if err := os.Symlink(workflow, path); err != nil {
					t.Fatal(err)
				}
			case "dry-run":
				req.DryRun = true
			case "output":
				req.Output = "json"
			case "setup-only":
				req.SetupOnly = true
			case "invalid-config":
				writeTestFile(t, workflow, "secret-invalid-config")
			}
			called := false
			factory := func(context.Context, openshell.Target) (openshell.Client, error) {
				called = true
				return nil, errors.New("secret-connection-error")
			}
			if err := runApply(context.Background(), factory, req, io.Discard); err == nil {
				t.Fatal("expected rejection")
			}
			if called != (mode == "plan-failure") {
				t.Fatalf("unexpected gateway access: %v", called)
			}
			switch mode {
			case "existing":
				data, _ := os.ReadFile(path)
				if string(data) != "preserve" {
					t.Fatal("result overwritten")
				}
			case "symlink":
				data, _ := os.ReadFile(workflow)
				if string(data) != resultWorkflow {
					t.Fatal("symlink target overwritten")
				}
			case "invalid-config", "plan-failure":
				r := readApplyResult(t, path)
				if r.Status != "failed" || (r.Phase != "load" && r.Phase != "plan") {
					t.Fatalf("unexpected failure result: %+v", r)
				}
			default:
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatal("incompatible mode created result file")
				}
			}
		})
	}
}

func TestApplyResultWriteFailure(t *testing.T) {
	r, file, err := startApplyResult(filepath.Join(t.TempDir(), "result.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := r.finish(context.Background(), file, nil); err == nil {
		t.Fatal("result write failure was swallowed")
	}
}

func TestApplyResultSourceCommit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "result.json")
	r, file, err := startApplyResult(path)
	if err != nil {
		t.Fatal(err)
	}
	r.SourceCommit = strings.Repeat("a", 40)
	if err := r.finish(context.Background(), file, nil); err != nil {
		t.Fatal(err)
	}
	if got := readApplyResult(t, path); got.SourceCommit != r.SourceCommit {
		t.Fatalf("source commit lost: %+v", got)
	}
}

func readApplyResult(t *testing.T, path string) applyResult {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "secret-") {
		t.Fatalf("sensitive value leaked into result: %s", data)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("result permissions: %v, %v", info, err)
	}
	var result applyResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

const resultWorkflow = `version: 1
name: result-test
sandbox:
  image: reviewer
  env:
    TOKEN: secret-env-value
agent:
  type: sh
  args: [-c, secret-prompt-value]
`
