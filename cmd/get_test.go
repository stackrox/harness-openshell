package cmd

import (
	"testing"

	fake "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/fake"
	"github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/types"

	"github.com/stackrox/harness-openshell/internal/testutil"
)

func TestGetAgents(t *testing.T) {
	client, fc := testutil.NewFakeClient("default")
	fc.AddSandbox("default", &types.Sandbox{Name: "agent-a", Status: types.SandboxStatus{Phase: types.SandboxReady}})
	fc.AddSandbox("default", &types.Sandbox{Name: "agent-b", Status: types.SandboxStatus{Phase: types.SandboxProvisioning}})

	cmd := NewGetCmd(testutil.FakeFactory(client))
	cmd.SetArgs([]string{"agents"})
	out, err := captureStdout(t, cmd.Execute)
	if err != nil {
		t.Fatalf("get agents: %v", err)
	}
	for _, want := range []string{"NAME", "PHASE", "agent-a", "Ready", "agent-b", "Provisioning"} {
		if !contains(out, want) {
			t.Errorf("get agents table missing %q:\n%s", want, out)
		}
	}
}

func TestGetAgentsEmpty(t *testing.T) {
	client, _ := testutil.NewFakeClient("default")
	cmd := NewGetCmd(testutil.FakeFactory(client))
	cmd.SetArgs([]string{"agents"})
	out, err := captureStdout(t, cmd.Execute)
	if err != nil {
		t.Fatalf("get agents: %v", err)
	}
	if !contains(out, "No sandboxes running.") {
		t.Errorf("empty get agents should print the friendly message:\n%s", out)
	}
}

func TestGetAgentsJSON(t *testing.T) {
	client, fc := testutil.NewFakeClient("default")
	fc.AddSandbox("default", &types.Sandbox{Name: "agent-a", Status: types.SandboxStatus{Phase: types.SandboxReady}})

	cmd := NewGetCmd(testutil.FakeFactory(client))
	cmd.SetArgs([]string{"agents", "-o", "json"})
	out, err := captureStdout(t, cmd.Execute)
	if err != nil {
		t.Fatalf("get agents -o json: %v", err)
	}
	if !contains(out, `"name": "agent-a"`) || !contains(out, `"phase": "Ready"`) {
		t.Errorf("json output missing fields:\n%s", out)
	}
	if !contains(out, "[") {
		t.Errorf("json output should be an array:\n%s", out)
	}
}

func TestGetProviders(t *testing.T) {
	client, fc := testutil.NewFakeClient("default")
	fc.AddProvider("default", &types.Provider{Name: "github", Type: "github"})
	fc.AddProvider("default", &types.Provider{Name: "vertex", Type: "google-vertex-ai"})

	cmd := NewGetCmd(testutil.FakeFactory(client))
	cmd.SetArgs([]string{"providers"})
	out, err := captureStdout(t, cmd.Execute)
	if err != nil {
		t.Fatalf("get providers: %v", err)
	}
	for _, want := range []string{"NAME", "github", "vertex"} {
		if !contains(out, want) {
			t.Errorf("get providers table missing %q:\n%s", want, out)
		}
	}
}

func TestGetProvidersEmpty(t *testing.T) {
	client, _ := testutil.NewFakeClient("default")
	cmd := NewGetCmd(testutil.FakeFactory(client))
	cmd.SetArgs([]string{"providers"})
	out, err := captureStdout(t, cmd.Execute)
	if err != nil {
		t.Fatalf("get providers: %v", err)
	}
	if !contains(out, "No providers registered.") {
		t.Errorf("empty get providers should print the friendly message:\n%s", out)
	}
}

// TestGetGateways pins the decision-4 reframe: a single active-gateway record
// with Status+Version (from the health RPC) and NO Active column.
func TestGetGateways(t *testing.T) {
	client, _ := testutil.NewFakeClient("default", fake.WithGatewayInfo(&types.GatewayInfo{
		Status:  types.ServiceStatusHealthy,
		Version: "0.0.110",
	}))
	cmd := NewGetCmd(testutil.FakeFactory(client))
	cmd.SetArgs([]string{"gateways"})
	out, err := captureStdout(t, cmd.Execute)
	if err != nil {
		t.Fatalf("get gateways: %v", err)
	}
	for _, want := range []string{"NAME", "ENDPOINT", "STATUS", "VERSION", "Healthy", "0.0.110"} {
		if !contains(out, want) {
			t.Errorf("get gateways missing %q:\n%s", want, out)
		}
	}
	if contains(out, "ACTIVE") {
		t.Errorf("get gateways should no longer show an Active column:\n%s", out)
	}
}

// TestGetGatewaysJSON checks the structured form is a single object, not an array.
func TestGetGatewaysJSON(t *testing.T) {
	client, _ := testutil.NewFakeClient("default", fake.WithGatewayInfo(&types.GatewayInfo{
		Status:  types.ServiceStatusDegraded,
		Version: "0.0.110",
	}))
	cmd := NewGetCmd(testutil.FakeFactory(client))
	cmd.SetArgs([]string{"gateways", "-o", "json"})
	out, err := captureStdout(t, cmd.Execute)
	if err != nil {
		t.Fatalf("get gateways -o json: %v", err)
	}
	if !contains(out, `"status": "Degraded"`) || !contains(out, `"version": "0.0.110"`) {
		t.Errorf("json output missing fields:\n%s", out)
	}
	// A single object starts with "{", not a "[" array.
	if contains(out, "[") {
		t.Errorf("get gateways json should be a single object, not an array:\n%s", out)
	}
}
