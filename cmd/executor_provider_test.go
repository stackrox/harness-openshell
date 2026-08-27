package cmd

import (
	"context"
	"path/filepath"
	"testing"

	fake "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/fake"
	"github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/types"
	"github.com/stackrox/harness-openshell/internal/openshell"
	"github.com/stackrox/harness-openshell/internal/plan"
	"github.com/stackrox/harness-openshell/internal/testutil"
)

// seedGatewayProviders adds setupTestAgent's referenced providers (github,
// atlassian) to the fake so their reconcile is a clean verify-noop, leaving the
// managed vertex provider as the interesting case. vertexLabels is stamped on the
// seeded vertex provider (nil = unowned).
func seedGatewayProviders(raw *fake.Client, vertexLabels map[string]string) {
	raw.AddProvider("default", &types.Provider{Name: "github", Type: "github"})
	raw.AddProvider("default", &types.Provider{Name: "atlassian", Type: "atlassian"})
	raw.AddProvider("default", &types.Provider{
		Name: "google-vertex-ai", Type: "google-vertex-ai", Labels: vertexLabels,
	})
}

// TestUpLocal_ProviderReconcile_AdoptsBootstrapped: apply drives desiredFromAgent,
// which marks the managed vertex provider adopt:true (the harness bootstrapped it
// on the CLI bridge, which cannot stamp the SDK owner label). The provider reconcile
// therefore adopts the unowned provider in place — an Update that stamps the owner
// label — rather than reporting adoption-required forever.
func TestUpLocal_ProviderReconcile_AdoptsBootstrapped(t *testing.T) {
	dir := setupTestAgent(t)
	gw := vertexGW()
	fakeClient, raw := testutil.NewFakeClient("default", fake.WithHealthResult(&types.HealthResult{Healthy: true}))
	seedGatewayProviders(raw, nil) // vertex unowned

	err := upLocal(upLocalOpts{
		harnessDir: dir,
		gw:         gw,
		target:     openshell.Target{Gateway: "test-gw"},
		agentPath:  filepath.Join(dir, "agents", "default.yaml"),
		noTTY:      true,
		setupOnly:  true,
		newClient:  keepOpenFactory(fakeClient),
	})
	if err != nil {
		t.Fatalf("upLocal: %v", err)
	}

	stored, err := raw.Providers().Get(context.Background(), "default", "google-vertex-ai")
	if err != nil {
		t.Fatalf("raw Get: %v", err)
	}
	if stored.Labels[plan.OwnerLabelKey] != plan.OwnerLabelValue {
		t.Errorf("vertex not adopted: labels = %v, want owner label stamped", stored.Labels)
	}
}

// TestUpLocal_ProviderReconcile_OwnedNoop: an already-owned managed vertex provider
// with no config drift is a noop — apply completes and creates the sandbox with no
// spurious rewrite. Referenced providers verify cleanly.
func TestUpLocal_ProviderReconcile_OwnedNoop(t *testing.T) {
	dir := setupTestAgent(t)
	gw := vertexGW()
	fakeClient, raw := testutil.NewFakeClient("default", fake.WithHealthResult(&types.HealthResult{Healthy: true}))
	seedGatewayProviders(raw, map[string]string{plan.OwnerLabelKey: plan.OwnerLabelValue})

	err := upLocal(upLocalOpts{
		harnessDir: dir,
		gw:         gw,
		target:     openshell.Target{Gateway: "test-gw"},
		agentPath:  filepath.Join(dir, "agents", "default.yaml"),
		noTTY:      true,
		newClient:  keepOpenFactory(fakeClient),
	})
	if err != nil {
		t.Fatalf("upLocal: %v", err)
	}
	// The provider reconcile must not have stranded apply: the sandbox is created.
	if gw.createCalls != 1 {
		t.Errorf("createCalls = %d, want 1 (sandbox created after clean reconcile)", gw.createCalls)
	}
	// The owner label survives an owned-noop pass.
	stored, err := raw.Providers().Get(context.Background(), "default", "google-vertex-ai")
	if err != nil {
		t.Fatalf("raw Get: %v", err)
	}
	if stored.Labels[plan.OwnerLabelKey] != plan.OwnerLabelValue {
		t.Errorf("owner label lost on noop: %v", stored.Labels)
	}
}
