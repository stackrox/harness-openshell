package run

import (
	"bytes"
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/stackrox/harness-openshell/internal/openshell"
)

type recordingSDKRunner struct {
	openshell.Client
	created            openshell.SandboxCreate
	executed           []string
	waited             bool
	deleted            bool
	exitCode           int
	stdout             string
	stderr             string
	execErr            error
	interactive        openshell.InteractiveSession
	interactiveCommand []string
	cols               uint32
	rows               uint32
}

func (r *recordingSDKRunner) ExecInteractive(_ context.Context, _ string, command []string, cols, rows uint32) (openshell.InteractiveSession, error) {
	r.interactiveCommand = append([]string(nil), command...)
	r.cols, r.rows = cols, rows
	return r.interactive, nil
}

func (r *recordingSDKRunner) CreateSandbox(_ context.Context, desired openshell.SandboxCreate) (openshell.Sandbox, error) {
	r.created = desired
	return openshell.Sandbox{Name: desired.Name, Phase: "Provisioning"}, nil
}

func (r *recordingSDKRunner) WaitSandboxReady(_ context.Context, name string) (openshell.Sandbox, error) {
	r.waited = true
	return openshell.Sandbox{Name: name, Phase: "Ready"}, nil
}

func (r *recordingSDKRunner) ExecSandbox(_ context.Context, _ string, command []string, stdout, stderr io.Writer) (int, error) {
	r.executed = append([]string(nil), command...)
	_, _ = io.WriteString(stdout, r.stdout)
	_, _ = io.WriteString(stderr, r.stderr)
	return r.exitCode, r.execErr
}

func (r *recordingSDKRunner) DeleteSandbox(_ context.Context, _ string) error {
	r.deleted = true
	return nil
}

func TestRunSandboxSDKLifecycle(t *testing.T) {
	runner := &recordingSDKRunner{stdout: "result\n", stderr: "warning\n"}
	req := SandboxRunRequest{
		Name:      "review",
		Image:     "quay.io/example/reviewer:v1",
		Providers: []string{"github-read"},
		Env:       map[string]string{"MODE": "strict"},
		Command:   []string{"codex", "exec", "review"},
		Policy:    []byte("version: 1\n"),
	}
	var stdout, stderr bytes.Buffer

	if err := RunSandboxSDK(context.Background(), runner, req, bytes.NewBuffer(nil), &stdout, &stderr); err != nil {
		t.Fatalf("RunSandboxSDK: %v", err)
	}
	if runner.created.Name != req.Name || runner.created.Image != req.Image || !reflect.DeepEqual(runner.created.Providers, req.Providers) {
		t.Errorf("create = %+v", runner.created)
	}
	if !bytes.Equal(runner.created.Policy, req.Policy) {
		t.Errorf("policy = %q, want %q", runner.created.Policy, req.Policy)
	}
	if !runner.waited || !runner.deleted {
		t.Errorf("waited=%v deleted=%v", runner.waited, runner.deleted)
	}
	if !reflect.DeepEqual(runner.executed, req.Command) {
		t.Errorf("command = %v, want %v", runner.executed, req.Command)
	}
	if stdout.String() != "result\n" || stderr.String() != "warning\n" {
		t.Errorf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestRunSandboxSDKFailsOnceAndCleansUp(t *testing.T) {
	runner := &recordingSDKRunner{execErr: errors.New("permission denied")}
	err := RunSandboxSDK(context.Background(), runner, SandboxRunRequest{
		Name: "review", Image: "reviewer", Command: []string{"reviewer"},
	}, bytes.NewBuffer(nil), &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("error = %v", err)
	}
	if !runner.deleted {
		t.Fatal("sandbox was not cleaned up after execution failure")
	}
}

func TestRunSandboxSDKKeepAndExitStatus(t *testing.T) {
	runner := &recordingSDKRunner{exitCode: 7}
	err := RunSandboxSDK(context.Background(), runner, SandboxRunRequest{
		Name: "review", Image: "reviewer", Command: []string{"reviewer"}, Keep: true,
	}, bytes.NewBuffer(nil), &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "status 7") {
		t.Fatalf("error = %v", err)
	}
	if runner.deleted {
		t.Fatal("keep=true sandbox was deleted")
	}
}

func TestRunSandboxSDKInteractiveExitStatus(t *testing.T) {
	session := &testInteractiveSession{exitCode: 7}
	runner := &recordingSDKRunner{interactive: session}
	err := RunSandboxSDK(context.Background(), runner, SandboxRunRequest{
		Name: "review", Image: "reviewer", Command: []string{"bash", "-i"}, TTY: true,
	}, strings.NewReader("exit\n"), &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "status 7") {
		t.Fatalf("error = %v, want interactive exit status", err)
	}
	if !reflect.DeepEqual(runner.interactiveCommand, []string{"bash", "-i"}) || runner.cols != 80 || runner.rows != 24 {
		t.Errorf("interactive command=%v size=%dx%d", runner.interactiveCommand, runner.cols, runner.rows)
	}
	if !session.closed {
		t.Fatal("interactive session was not closed")
	}
}

type testInteractiveSession struct {
	exitCode int
	closed   bool
}

func (*testInteractiveSession) Read([]byte) (int, error)    { return 0, io.EOF }
func (*testInteractiveSession) Write(p []byte) (int, error) { return len(p), nil }
func (*testInteractiveSession) Resize(uint32, uint32) error { return nil }
func (s *testInteractiveSession) ExitCode() (int, error)    { return s.exitCode, nil }
func (s *testInteractiveSession) Close() error              { s.closed = true; return nil }
