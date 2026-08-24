package plan

import (
	"context"
	"errors"
	"testing"

	fake "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/fake"
	"github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/types"

	"github.com/stackrox/harness-openshell/internal/config"
	"github.com/stackrox/harness-openshell/internal/openshell"
	"github.com/stackrox/harness-openshell/internal/testutil"
)

func TestReadCurrentState_HealthyGateway(t *testing.T) {
	ctx := context.Background()
	client := testutil.NewFake("default",
		fake.WithHealthResult(&types.HealthResult{Healthy: true, Version: "0.0.85"}),
	)
	desired := &config.Harness{}

	state, err := ReadCurrentState(ctx, client, desired)
	if err != nil {
		t.Fatalf("ReadCurrentState: %v", err)
	}

	if !state.Reachable {
		t.Error("expected Reachable=true for healthy gateway")
	}
	if !state.Health.Healthy {
		t.Error("expected Health.Healthy=true")
	}
	if state.Health.Version != "0.0.85" {
		t.Errorf("expected version 0.0.85, got %s", state.Health.Version)
	}
	if state.Inference.Capable {
		t.Error("expected Inference.Capable=false")
	}
}

func TestReadCurrentState_ProvidersPopulated(t *testing.T) {
	ctx := context.Background()
	c, raw := testutil.NewFakeClient("default",
		fake.WithHealthResult(&types.HealthResult{Healthy: true, Version: "0.0.85"}),
	)
	raw.AddProvider("default", &types.Provider{Name: "github", Type: "github"})
	raw.AddProvider("default", &types.Provider{Name: "gcp", Type: "google-vertex-ai"})
	desired := &config.Harness{}

	state, err := ReadCurrentState(ctx, c, desired)
	if err != nil {
		t.Fatalf("ReadCurrentState: %v", err)
	}

	if len(state.Providers) != 2 {
		t.Errorf("expected 2 providers, got %d", len(state.Providers))
	}
	if state.Providers[0].Name != "github" {
		t.Errorf("expected first provider 'github', got %s", state.Providers[0].Name)
	}
	if state.Providers[1].Name != "gcp" {
		t.Errorf("expected second provider 'gcp', got %s", state.Providers[1].Name)
	}
}

func TestReadCurrentState_UnavailableDegrades(t *testing.T) {
	ctx := context.Background()
	// Create a custom client that returns ErrUnavailable
	fakeErr := &errorClient{err: openshell.ErrUnavailable}
	desired := &config.Harness{}

	state, err := ReadCurrentState(ctx, fakeErr, desired)
	if err != nil {
		t.Fatalf("ReadCurrentState should degrade gracefully: %v", err)
	}

	if state.Reachable {
		t.Error("expected Reachable=false for unavailable gateway")
	}
}

func TestReadCurrentState_UnauthenticatedDegrades(t *testing.T) {
	ctx := context.Background()
	fakeErr := &errorClient{err: openshell.ErrUnauthenticated}
	desired := &config.Harness{}

	state, err := ReadCurrentState(ctx, fakeErr, desired)
	if err != nil {
		t.Fatalf("ReadCurrentState should degrade gracefully: %v", err)
	}

	if state.Reachable {
		t.Error("expected Reachable=false for unauthenticated gateway")
	}
}

func TestReadCurrentState_OtherErrorEscalates(t *testing.T) {
	ctx := context.Background()
	fakeErr := &errorClient{err: openshell.ErrPermission}
	desired := &config.Harness{}

	_, err := ReadCurrentState(ctx, fakeErr, desired)
	if err == nil {
		t.Fatal("expected ReadCurrentState to escalate ErrPermission")
	}
	if !errors.Is(err, openshell.ErrPermission) {
		t.Errorf("expected ErrPermission, got %v", err)
	}
}

func TestReadCurrentState_InferenceAlwaysFalse(t *testing.T) {
	ctx := context.Background()
	client := testutil.NewFake("default",
		fake.WithHealthResult(&types.HealthResult{Healthy: true, Version: "0.0.85"}),
	)
	desired := &config.Harness{}

	state, err := ReadCurrentState(ctx, client, desired)
	if err != nil {
		t.Fatalf("ReadCurrentState: %v", err)
	}

	if state.Inference.Capable {
		t.Error("expected Inference.Capable=false")
	}
	if state.Inference.Route != "" {
		t.Error("expected Inference.Route empty")
	}
}

func TestReadCurrentState_OnlyReadMethodsCalled(t *testing.T) {
	ctx := context.Background()
	client, fakeClient := testutil.NewFakeClient("default",
		fake.WithHealthResult(&types.HealthResult{Healthy: true, Version: "0.0.85"}),
	)
	fakeClient.AddProvider("default", &types.Provider{Name: "github", Type: "github"})
	desired := &config.Harness{}

	// Wrap the client in a recording wrapper to ensure only Health/Providers are called.
	recorder := &recordingClient{wrapped: client}

	state, err := ReadCurrentState(ctx, recorder, desired)
	if err != nil {
		t.Fatalf("ReadCurrentState: %v", err)
	}

	if !recorder.healthCalled {
		t.Error("expected Health() to be called")
	}
	if !recorder.providersCalled {
		t.Error("expected Providers() to be called")
	}
	if recorder.closeCalled {
		t.Error("expected Close() to NOT be called by ReadCurrentState")
	}

	_ = state
}

// recordingClient wraps an openshell.Client and records method calls.
type recordingClient struct {
	wrapped         openshell.Client
	healthCalled    bool
	providersCalled bool
	closeCalled     bool
}

func (r *recordingClient) Health(ctx context.Context) (openshell.Health, error) {
	r.healthCalled = true
	return r.wrapped.Health(ctx)
}

func (r *recordingClient) Providers(ctx context.Context) ([]openshell.Provider, error) {
	r.providersCalled = true
	return r.wrapped.Providers(ctx)
}

func (r *recordingClient) Close() error {
	r.closeCalled = true
	return r.wrapped.Close()
}

// errorClient is a test client that always returns a specific error.
type errorClient struct {
	err error
}

func (e *errorClient) Health(ctx context.Context) (openshell.Health, error) {
	return openshell.Health{}, e.err
}

func (e *errorClient) Providers(ctx context.Context) ([]openshell.Provider, error) {
	return nil, e.err
}

func (e *errorClient) Close() error {
	return nil
}
