package cmd

import (
	"context"
	"testing"

	"github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/types"

	"github.com/stackrox/harness-openshell/internal/openshell"
	"github.com/stackrox/harness-openshell/internal/testutil"
)

// The delete tests use keepOpenFactory (executor_inference_test.go) so the
// command's deferred Close doesn't shut the shared fake before the test can
// assert the resources were actually removed, not merely that a log line printed.

func sandboxNames(t *testing.T, c openshell.Client) []string {
	t.Helper()
	sandboxes, err := c.Sandboxes(context.Background())
	if err != nil {
		t.Fatalf("list sandboxes: %v", err)
	}
	names := make([]string, len(sandboxes))
	for i, s := range sandboxes {
		names[i] = s.Name
	}
	return names
}

func providerNames(t *testing.T, c openshell.Client) []string {
	t.Helper()
	providers, err := c.Providers(context.Background())
	if err != nil {
		t.Fatalf("list providers: %v", err)
	}
	names := make([]string, len(providers))
	for i, p := range providers {
		names[i] = p.Name
	}
	return names
}

func TestDeleteTargeted(t *testing.T) {
	client, fc := testutil.NewFakeClient("default")
	fc.AddSandbox("default", &types.Sandbox{Name: "agent-a", Status: types.SandboxStatus{Phase: types.SandboxReady}})
	fc.AddSandbox("default", &types.Sandbox{Name: "agent-b", Status: types.SandboxStatus{Phase: types.SandboxReady}})

	cmd := NewDeleteCmd(keepOpenFactory(client))
	cmd.SetArgs([]string{"agent-a"})
	if _, err := captureStdout(t, cmd.Execute); err != nil {
		t.Fatalf("delete agent-a: %v", err)
	}

	remaining := sandboxNames(t, client)
	if len(remaining) != 1 || remaining[0] != "agent-b" {
		t.Errorf("targeted delete should remove only agent-a, got %v", remaining)
	}
}

func TestDeleteSandboxesSweep(t *testing.T) {
	client, fc := testutil.NewFakeClient("default")
	fc.AddSandbox("default", &types.Sandbox{Name: "agent-a", Status: types.SandboxStatus{Phase: types.SandboxReady}})
	fc.AddSandbox("default", &types.Sandbox{Name: "agent-b", Status: types.SandboxStatus{Phase: types.SandboxReady}})

	cmd := NewDeleteCmd(keepOpenFactory(client))
	cmd.SetArgs([]string{"--sandboxes", "--gateway", "prod"})
	if _, err := captureStdout(t, cmd.Execute); err != nil {
		t.Fatalf("delete --sandboxes: %v", err)
	}

	if remaining := sandboxNames(t, client); len(remaining) != 0 {
		t.Errorf("--sandboxes should sweep every sandbox, got %v", remaining)
	}
}

func TestDeleteProvidersGuard(t *testing.T) {
	client, fc := testutil.NewFakeClient("default")
	fc.AddSandbox("default", &types.Sandbox{Name: "agent-a", Status: types.SandboxStatus{Phase: types.SandboxReady}})
	fc.AddProvider("default", &types.Provider{Name: "github", Type: "github"})

	cmd := NewDeleteCmd(keepOpenFactory(client))
	cmd.SetArgs([]string{"--providers", "--gateway", "prod"})
	_, err := captureStdout(t, cmd.Execute)
	if err == nil {
		t.Fatal("deleting providers with a running sandbox should be refused")
	}
	if !contains(err.Error(), "running sandboxes") {
		t.Errorf("unexpected guard error: %v", err)
	}

	// The guard must prevent deletion, not delete-then-error: the provider survives.
	if names := providerNames(t, client); len(names) != 1 || names[0] != "github" {
		t.Errorf("guard should leave the provider untouched, got %v", names)
	}
}

func TestDeleteProvidersSweep(t *testing.T) {
	client, fc := testutil.NewFakeClient("default")
	fc.AddProvider("default", &types.Provider{Name: "github", Type: "github"})
	fc.AddProvider("default", &types.Provider{Name: "vertex", Type: "google-vertex-ai"})

	cmd := NewDeleteCmd(keepOpenFactory(client))
	cmd.SetArgs([]string{"--providers", "--gateway", "prod"})
	if _, err := captureStdout(t, cmd.Execute); err != nil {
		t.Fatalf("delete --providers: %v", err)
	}

	if names := providerNames(t, client); len(names) != 0 {
		t.Errorf("--providers should sweep every provider, got %v", names)
	}
}
