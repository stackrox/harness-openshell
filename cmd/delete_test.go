package cmd

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/types"

	"github.com/stackrox/harness-openshell/internal/openshell"
	"github.com/stackrox/harness-openshell/internal/testutil"
)

type noCloseClient struct{ openshell.Client }

func (noCloseClient) Close() error { return nil }

func keepOpenFactory(client openshell.Client) openshell.Factory {
	return testutil.FakeFactory(noCloseClient{client})
}

// The delete tests use keepOpenFactory so the command's deferred Close doesn't
// shut the shared fake before the test can inspect the resulting resources.

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
	cmd.SetArgs([]string{"agent-a", "--gateway", "prod"})
	if _, err := captureStdout(t, cmd.Execute); err != nil {
		t.Fatalf("delete agent-a: %v", err)
	}

	remaining := sandboxNames(t, client)
	if len(remaining) != 1 || remaining[0] != "agent-b" {
		t.Errorf("targeted delete should remove only agent-a, got %v", remaining)
	}
}

func TestDeleteTargetedContinuesAfterFailure(t *testing.T) {
	base, fc := testutil.NewFakeClient("default")
	fc.AddSandbox("default", &types.Sandbox{Name: "agent-a", Status: types.SandboxStatus{Phase: types.SandboxReady}})
	client := &deleteErrorClient{Client: base, name: "missing", err: errors.New("sandbox missing")}

	cmd := NewDeleteCmd(keepOpenFactory(client))
	cmd.SetArgs([]string{"missing", "agent-a", "--gateway", "prod"})
	_, err := captureStdout(t, cmd.Execute)
	if err == nil || !strings.Contains(err.Error(), `deleting sandbox "missing"`) {
		t.Fatalf("targeted delete should report the missing sandbox: %v", err)
	}
	if names := sandboxNames(t, client); len(names) != 0 {
		t.Errorf("targeted deletion should continue after a failure, got %v", names)
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

func TestDeleteSandboxesLeavesProvidersUntouched(t *testing.T) {
	client, fc := testutil.NewFakeClient("default")
	fc.AddProvider("default", &types.Provider{Name: "github", Type: "github"})
	fc.AddSandbox("default", &types.Sandbox{Name: "agent-a", Status: types.SandboxStatus{Phase: types.SandboxReady}})

	cmd := NewDeleteCmd(keepOpenFactory(client))
	cmd.SetArgs([]string{"--sandboxes", "--gateway", "prod"})
	if _, err := captureStdout(t, cmd.Execute); err != nil {
		t.Fatalf("delete --sandboxes: %v", err)
	}
	if names := providerNames(t, client); len(names) != 1 || names[0] != "github" {
		t.Errorf("sandbox deletion should leave providers untouched, got %v", names)
	}
}

func TestDeleteRejectsRemovedProviderFlagsBeforeClientCreation(t *testing.T) {
	for _, flag := range []string{"--all", "--providers"} {
		t.Run(flag, func(t *testing.T) {
			called := false
			factory := func(context.Context, openshell.Target) (openshell.Client, error) {
				called = true
				return nil, errors.New("factory should not be called")
			}
			cmd := NewDeleteCmd(factory)
			cmd.SetArgs([]string{flag})
			if _, err := captureStdout(t, cmd.Execute); err == nil {
				t.Fatalf("%s should be rejected", flag)
			}
			if called {
				t.Fatalf("%s should fail before creating a client", flag)
			}
		})
	}
}

func TestDeleteRejectsNamesWithSandboxSweep(t *testing.T) {
	called := false
	factory := func(context.Context, openshell.Target) (openshell.Client, error) {
		called = true
		return nil, errors.New("factory should not be called")
	}
	cmd := NewDeleteCmd(factory)
	cmd.SetArgs([]string{"agent-a", "--sandboxes"})
	if _, err := captureStdout(t, cmd.Execute); err == nil {
		t.Fatal("names and --sandboxes should be rejected")
	}
	if called {
		t.Fatal("invalid delete combination should fail before creating a client")
	}
}

func TestDeleteSandboxesListFailurePropagates(t *testing.T) {
	listErr := errors.New("gateway list failed")
	client := &listErrorClient{Client: testutil.NewFake("default"), err: listErr}
	cmd := NewDeleteCmd(keepOpenFactory(client))
	cmd.SetArgs([]string{"--sandboxes", "--gateway", "prod"})
	_, err := captureStdout(t, cmd.Execute)
	if !errors.Is(err, listErr) {
		t.Fatalf("list failure = %v, want wrapped %v", err, listErr)
	}
}

type listErrorClient struct {
	openshell.Client
	err error
}

func (c *listErrorClient) Sandboxes(context.Context) ([]openshell.Sandbox, error) {
	return nil, c.err
}

type deleteErrorClient struct {
	openshell.Client
	name string
	err  error
}

func (c *deleteErrorClient) DeleteSandbox(ctx context.Context, name string) error {
	if name == c.name {
		return c.err
	}
	return c.Client.DeleteSandbox(ctx, name)
}

// With no --gateway flag and no $OPENSHELL_GATEWAY, delete relies on the SDK
// to resolve the active gateway and must still sweep when the factory succeeds.
func TestDeleteUsesActiveGateway(t *testing.T) {
	t.Setenv("OPENSHELL_GATEWAY", "")
	client, fc := testutil.NewFakeClient("default")
	fc.AddSandbox("default", &types.Sandbox{Name: "agent-a", Status: types.SandboxStatus{Phase: types.SandboxReady}})

	cmd := NewDeleteCmd(keepOpenFactory(client))
	cmd.SetArgs([]string{"--sandboxes"})
	if _, err := captureStdout(t, cmd.Execute); err != nil {
		t.Fatalf("delete --sandboxes with an active gateway: %v", err)
	}
	if names := sandboxNames(t, client); len(names) != 0 {
		t.Errorf("active-gateway resolution should sweep every sandbox, got %v", names)
	}
}
