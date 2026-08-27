package sdkclient

import (
	"context"
	"errors"
	"testing"

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
