package sdkclient

import (
	"bytes"
	"context"
	"errors"
	"io"
	"reflect"
	"testing"

	v1 "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1"
	fake "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/fake"
	"github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/types"

	"github.com/stackrox/harness-openshell/internal/openshell"
)

// TestFromSDKSandboxMapsNameAndPhase pins the read-widening: the harness Sandbox
// carries the top-level Name and the lifecycle phase as a string, and nothing
// else the SDK holds.
func TestFromSDKSandboxMapsNameAndPhase(t *testing.T) {
	got := fromSDKSandbox(&types.Sandbox{
		Name:   "agent-1",
		Status: types.SandboxStatus{SandboxName: "echo-should-be-ignored", Phase: types.SandboxReady},
	})
	if got.Name != "agent-1" {
		t.Errorf("Name: got %q, want agent-1 (top-level Name, not Status.SandboxName)", got.Name)
	}
	if got.Phase != "Ready" {
		t.Errorf("Phase: got %q, want Ready", got.Phase)
	}
}

// TestSandboxes lists mapped sandboxes; an empty store yields an empty slice.
func TestSandboxes(t *testing.T) {
	ctx := context.Background()
	fc := fake.NewClient()
	fc.AddSandbox("default", &types.Sandbox{Name: "a", Status: types.SandboxStatus{Phase: types.SandboxReady}})
	fc.AddSandbox("default", &types.Sandbox{Name: "b", Status: types.SandboxStatus{Phase: types.SandboxProvisioning}})
	c := NewFromClient(fc, "default")

	got, err := c.Sandboxes(ctx)
	if err != nil {
		t.Fatalf("Sandboxes: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 sandboxes, got %d: %+v", len(got), got)
	}
	byName := map[string]string{}
	for _, s := range got {
		byName[s.Name] = s.Phase
	}
	if byName["a"] != "Ready" || byName["b"] != "Provisioning" {
		t.Errorf("unexpected phases: %v", byName)
	}

	empty, err := NewFromClient(fake.NewClient(), "default").Sandboxes(ctx)
	if err != nil {
		t.Fatalf("Sandboxes(empty): %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("want empty slice, got %+v", empty)
	}
}

// TestGetSandbox covers the by-name read: fields map through and a missing
// sandbox surfaces as openshell.ErrNotFound.
func TestGetSandbox(t *testing.T) {
	ctx := context.Background()
	fc := fake.NewClient()
	fc.AddSandbox("default", &types.Sandbox{Name: "agent-1", Status: types.SandboxStatus{Phase: types.SandboxReady}})
	c := NewFromClient(fc, "default")

	got, err := c.GetSandbox(ctx, "agent-1")
	if err != nil {
		t.Fatalf("GetSandbox: %v", err)
	}
	if got.Name != "agent-1" || got.Phase != "Ready" {
		t.Errorf("unexpected sandbox: %+v", got)
	}

	if _, err := c.GetSandbox(ctx, "absent"); !errors.Is(err, openshell.ErrNotFound) {
		t.Errorf("GetSandbox(absent): want ErrNotFound, got %v", err)
	}
}

// TestDeleteSandbox removes the named sandbox; a follow-up list confirms it.
func TestDeleteSandbox(t *testing.T) {
	ctx := context.Background()
	fc := fake.NewClient()
	fc.AddSandbox("default", &types.Sandbox{Name: "agent-1", Status: types.SandboxStatus{Phase: types.SandboxReady}})
	c := NewFromClient(fc, "default")

	if err := c.DeleteSandbox(ctx, "agent-1"); err != nil {
		t.Fatalf("DeleteSandbox: %v", err)
	}
	got, err := c.Sandboxes(ctx)
	if err != nil {
		t.Fatalf("Sandboxes: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("sandbox not deleted: %+v", got)
	}
}

func TestCreateAndWaitSandbox(t *testing.T) {
	ctx := context.Background()
	raw := fake.NewClient()
	c := NewFromClient(raw, "team")
	executor := c.(openshell.SandboxExecutionClient)
	desired := openshell.SandboxCreate{
		Name:      "review",
		Image:     "quay.io/example/reviewer:v1",
		Providers: []string{"github-read"},
		Env:       map[string]string{"MODE": "strict"},
		Labels:    map[string]string{"workflow": "security-review"},
	}

	if _, err := executor.CreateSandbox(ctx, desired); err != nil {
		t.Fatalf("CreateSandbox: %v", err)
	}
	stored, err := raw.Sandboxes().Get(ctx, "team", "review")
	if err != nil {
		t.Fatalf("Get created sandbox: %v", err)
	}
	if stored.Spec.Template == nil || stored.Spec.Template.Image != desired.Image {
		t.Errorf("template = %+v", stored.Spec.Template)
	}
	if !reflect.DeepEqual(stored.Spec.Providers, desired.Providers) || !reflect.DeepEqual(stored.Spec.Environment, desired.Env) {
		t.Errorf("spec = %+v", stored.Spec)
	}
	if !reflect.DeepEqual(stored.Labels, desired.Labels) {
		t.Errorf("labels = %v", stored.Labels)
	}

	raw.AddSandbox("team", &types.Sandbox{Name: "ready", Status: types.SandboxStatus{Phase: types.SandboxReady}})
	ready, err := executor.WaitSandboxReady(ctx, "ready")
	if err != nil {
		t.Fatalf("WaitSandboxReady: %v", err)
	}
	if ready.Phase != "Ready" {
		t.Errorf("phase = %q", ready.Phase)
	}
}

type clientWithExec struct {
	v1.ClientInterface
	exec v1.ExecInterface
}

func (c *clientWithExec) Exec() v1.ExecInterface { return c.exec }

type recordingExec struct {
	v1.ExecInterface
	workspace string
	sandbox   string
	command   []string
}

func (e *recordingExec) Stream(_ context.Context, workspace, sandbox string, command []string, _ ...v1.ExecOptions) (v1.ExecStream, error) {
	e.workspace = workspace
	e.sandbox = sandbox
	e.command = append([]string(nil), command...)
	return &recordingExecStream{chunks: []*v1.ExecChunk{
		{Stream: v1.StreamStdout, Data: []byte("out")},
		{Stream: v1.StreamStderr, Data: []byte("err")},
	}, exitCode: 3}, nil
}

type recordingExecStream struct {
	chunks   []*v1.ExecChunk
	index    int
	exitCode int
}

func (s *recordingExecStream) Next() (*v1.ExecChunk, error) {
	if s.index == len(s.chunks) {
		return nil, io.EOF
	}
	chunk := s.chunks[s.index]
	s.index++
	return chunk, nil
}

func (s *recordingExecStream) ExitCode() (int, error) { return s.exitCode, nil }
func (s *recordingExecStream) Close() error           { return nil }

func TestExecSandbox(t *testing.T) {
	recorder := &recordingExec{}
	raw := &clientWithExec{ClientInterface: fake.NewClient(), exec: recorder}
	c := NewFromClient(raw, "team")
	executor := c.(openshell.SandboxExecutionClient)

	var stdout, stderr bytes.Buffer
	exitCode, err := executor.ExecSandbox(context.Background(), "review", []string{"codex", "exec"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("ExecSandbox: %v", err)
	}
	if recorder.workspace != "team" || recorder.sandbox != "review" || !reflect.DeepEqual(recorder.command, []string{"codex", "exec"}) {
		t.Errorf("exec target=%s/%s command=%v", recorder.workspace, recorder.sandbox, recorder.command)
	}
	if exitCode != 3 || stdout.String() != "out" || stderr.String() != "err" {
		t.Errorf("exit=%d stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
	}
}
