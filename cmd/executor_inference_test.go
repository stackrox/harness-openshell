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

// vertexGW returns a mockGW that has all of setupTestAgent's providers already
// registered, so upLocal reaches inference reconcile. Gateway reachability and
// active-gateway resolution are the SDK Factory's job now, not the mock's.
func vertexGW() *mockGW {
	return &mockGW{
		providers: map[string]bool{"github": true, "google-vertex-ai": true, "atlassian": true},
	}
}

// noCloseClient wraps an openshell.Client with a no-op Close so a test can keep
// reading the shared fake after upLocal's defer closes its client. (The SDK
// fake's Close marks the underlying raw client closed for all wrappers.)
type noCloseClient struct{ openshell.Client }

func (noCloseClient) Close() error { return nil }

// keepOpenFactory returns a Factory that hands out c (Close-guarded) on every
// call, so upLocal reconciles against the same fake the test inspects afterward.
func keepOpenFactory(c openshell.Client) openshell.Factory {
	return testutil.FakeFactory(noCloseClient{c})
}

func TestUpLocal_InferenceReconcile_Create(t *testing.T) {
	dir := setupTestAgent(t)
	gw := vertexGW()
	fakeClient := testutil.NewFake("default", fake.WithHealthResult(&types.HealthResult{Healthy: true}))

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

	route, err := fakeClient.GetInferenceRoute(context.Background(), plan.DefaultInferenceRoute)
	if err != nil {
		t.Fatalf("GetInferenceRoute: %v", err)
	}
	if route.Provider != "google-vertex-ai" {
		t.Errorf("route provider = %q, want google-vertex-ai", route.Provider)
	}
	if route.Model != "claude-sonnet-4-6" {
		t.Errorf("route model = %q, want claude-sonnet-4-6", route.Model)
	}
	// The route must still be created — apply always reconciles inference.
	if gw.createCalls != 1 {
		t.Errorf("createCalls = %d, want 1 (sandbox still created)", gw.createCalls)
	}
}

func TestUpLocal_InferenceReconcile_ModelChange(t *testing.T) {
	dir := setupTestAgent(t)
	gw := vertexGW()
	fakeClient := testutil.NewFake("default", fake.WithHealthResult(&types.HealthResult{Healthy: true}))

	// Seed a route with a stale model so reconcile must update it.
	if _, err := fakeClient.SetInferenceRoute(context.Background(), openshell.InferenceRouteConfig{
		Provider: "google-vertex-ai", Model: "claude-old-1", Route: plan.DefaultInferenceRoute, NoVerify: true,
	}); err != nil {
		t.Fatalf("seed route: %v", err)
	}

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

	route, err := fakeClient.GetInferenceRoute(context.Background(), plan.DefaultInferenceRoute)
	if err != nil {
		t.Fatalf("GetInferenceRoute: %v", err)
	}
	if route.Model != "claude-sonnet-4-6" {
		t.Errorf("route model = %q, want claude-sonnet-4-6 after update", route.Model)
	}
}

// An unreachable gateway (client construction fails) must abort apply at the
// reachability preflight — the harness never provisions a gateway, so there is
// nothing to create a sandbox on. The preflight and reconcile now share the SDK
// Factory, so a construction failure fails fast rather than degrading.
func TestUpLocal_UnreachableGateway_FailsFast(t *testing.T) {
	dir := setupTestAgent(t)
	gw := vertexGW()

	errFactory := func(context.Context, openshell.Target) (openshell.Client, error) {
		return nil, openshell.ErrUnavailable
	}

	err := upLocal(upLocalOpts{
		harnessDir: dir,
		gw:         gw,
		target:     openshell.Target{Gateway: "test-gw"},
		agentPath:  filepath.Join(dir, "agents", "default.yaml"),
		noTTY:      true,
		newClient:  errFactory,
	})
	if err == nil {
		t.Fatal("upLocal should fail fast when the gateway is unreachable")
	}
	if !contains(err.Error(), "no active gateway is reachable") {
		t.Errorf("unexpected error: %v", err)
	}
	// The preflight runs before provider registration and sandbox creation.
	if gw.createCalls != 0 {
		t.Errorf("createCalls = %d, want 0 (aborted before sandbox creation)", gw.createCalls)
	}
}

func TestUpLocal_SetupOnly_SkipsSandbox(t *testing.T) {
	dir := setupTestAgent(t)
	gw := vertexGW()
	fakeClient := testutil.NewFake("default", fake.WithHealthResult(&types.HealthResult{Healthy: true}))

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

	// --setup-only must not create a sandbox...
	if gw.createCalls != 0 {
		t.Errorf("createCalls = %d, want 0 (--setup-only skips sandbox)", gw.createCalls)
	}
	// ...but must still reconcile inference.
	route, err := fakeClient.GetInferenceRoute(context.Background(), plan.DefaultInferenceRoute)
	if err != nil {
		t.Fatalf("GetInferenceRoute: %v", err)
	}
	if route.Provider != "google-vertex-ai" {
		t.Errorf("route provider = %q, want google-vertex-ai (inference reconciled under --setup-only)", route.Provider)
	}
}
