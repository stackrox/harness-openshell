package cmd

import (
	"testing"

	fake "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/fake"
	"github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/types"

	"github.com/stackrox/harness-openshell/internal/testutil"
)

func TestDescribeSandbox(t *testing.T) {
	client, fc := testutil.NewFakeClient("default", fake.WithGatewayInfo(&types.GatewayInfo{
		Status:  types.ServiceStatusHealthy,
		Version: "0.0.110",
	}))
	fc.AddSandbox("default", &types.Sandbox{Name: "agent-a", Status: types.SandboxStatus{Phase: types.SandboxReady}})
	fc.AddProvider("default", &types.Provider{Name: "github", Type: "github"})

	cmd := NewDescribeCmd(testutil.FakeFactory(client))
	cmd.SetArgs([]string{"agent-a"})
	out, err := captureStdout(t, cmd.Execute)
	if err != nil {
		t.Fatalf("describe: %v", err)
	}
	for _, want := range []string{"agent-a", "Ready", "github"} {
		if !contains(out, want) {
			t.Errorf("describe output missing %q:\n%s", want, out)
		}
	}
}

func TestDescribeSandboxNotFound(t *testing.T) {
	client, _ := testutil.NewFakeClient("default")
	cmd := NewDescribeCmd(testutil.FakeFactory(client))
	cmd.SetArgs([]string{"nope"})
	_, err := captureStdout(t, cmd.Execute)
	if err == nil {
		t.Fatal("describe of a missing sandbox should error")
	}
	if !contains(err.Error(), `sandbox "nope" not found`) {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDescribeSandboxJSON(t *testing.T) {
	client, fc := testutil.NewFakeClient("default")
	fc.AddSandbox("default", &types.Sandbox{Name: "agent-a", Status: types.SandboxStatus{Phase: types.SandboxReady}})

	cmd := NewDescribeCmd(testutil.FakeFactory(client))
	cmd.SetArgs([]string{"agent-a", "-o", "json"})
	out, err := captureStdout(t, cmd.Execute)
	if err != nil {
		t.Fatalf("describe -o json: %v", err)
	}
	if !contains(out, `"name": "agent-a"`) || !contains(out, `"phase": "Ready"`) {
		t.Errorf("json output missing fields:\n%s", out)
	}
}
