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
		fake.WithHealthResult(&types.HealthResult{Healthy: true, Version: "0.0.110"}),
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
	if state.Health.Version != "0.0.110" {
		t.Errorf("expected version 0.0.110, got %s", state.Health.Version)
	}
	if state.Inference.Capable {
		t.Error("expected Inference.Capable=false")
	}
}

func TestReadCurrentState_ProvidersPopulated(t *testing.T) {
	ctx := context.Background()
	c, raw := testutil.NewFakeClient("default",
		fake.WithHealthResult(&types.HealthResult{Healthy: true, Version: "0.0.110"}),
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
	// The fake stores providers in a name-keyed map, so Providers() order is
	// not guaranteed (and production does not rely on it — the plan matches
	// current providers by name). Assert membership, not position.
	byName := make(map[string]string, len(state.Providers))
	for _, p := range state.Providers {
		byName[p.Name] = p.Type
	}
	if byName["github"] != "github" {
		t.Errorf("expected provider 'github' of type github, got type %q", byName["github"])
	}
	if byName["gcp"] != "google-vertex-ai" {
		t.Errorf("expected provider 'gcp' of type google-vertex-ai, got type %q", byName["gcp"])
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

// TestReadCurrentState_InferenceNotReadWhenUnconfigured pins that an unused
// inference subsystem is never probed: with no desired inference config, the
// route is not read and Inference stays zero (Capable=false).
func TestReadCurrentState_InferenceNotReadWhenUnconfigured(t *testing.T) {
	ctx := context.Background()
	client := testutil.NewFake("default",
		fake.WithHealthResult(&types.HealthResult{Healthy: true, Version: "0.0.110"}),
	)
	desired := &config.Harness{}

	state, err := ReadCurrentState(ctx, client, desired)
	if err != nil {
		t.Fatalf("ReadCurrentState: %v", err)
	}

	if state.Inference.Capable {
		t.Error("expected Inference.Capable=false when inference is unconfigured")
	}
	if state.Inference.Route != "" {
		t.Error("expected Inference.Route empty")
	}
}

// inferenceDesired is a minimal harness with inference configured (default
// route), for exercising the inference read path.
func inferenceDesired() *config.Harness {
	return &config.Harness{
		Spec: config.Spec{
			Inference: config.Inference{Provider: "gcp", Model: "claude-opus-4-8"},
		},
	}
}

func TestReadCurrentState_InferencePresent(t *testing.T) {
	ctx := context.Background()
	client, _ := testutil.NewFakeClient("default",
		fake.WithHealthResult(&types.HealthResult{Healthy: true, Version: "0.0.110"}),
	)
	// Seed the route under the resolved default name so the read finds it.
	if _, err := client.SetInferenceRoute(ctx, openshell.InferenceRouteConfig{
		Provider: "gcp", Model: "claude-opus-4-8", Route: DefaultInferenceRoute, TimeoutSecs: 60, NoVerify: true,
	}); err != nil {
		t.Fatalf("seed SetInferenceRoute: %v", err)
	}

	state, err := ReadCurrentState(ctx, client, inferenceDesired())
	if err != nil {
		t.Fatalf("ReadCurrentState: %v", err)
	}

	got := state.Inference
	if !got.Capable || !got.Present {
		t.Fatalf("want Capable && Present, got %+v", got)
	}
	if got.Provider != "gcp" || got.Model != "claude-opus-4-8" || got.TimeoutSecs != 60 {
		t.Errorf("route not populated: %+v", got)
	}
	if got.Route != DefaultInferenceRoute {
		t.Errorf("route name: want %q, got %q", DefaultInferenceRoute, got.Route)
	}
}

func TestReadCurrentState_InferenceAbsent(t *testing.T) {
	ctx := context.Background()
	client := testutil.NewFake("default",
		fake.WithHealthResult(&types.HealthResult{Healthy: true, Version: "0.0.110"}),
	)

	state, err := ReadCurrentState(ctx, client, inferenceDesired())
	if err != nil {
		t.Fatalf("ReadCurrentState: %v", err)
	}

	if !state.Inference.Capable {
		t.Error("expected Capable=true (gateway serves inference, route just absent)")
	}
	if state.Inference.Present {
		t.Error("expected Present=false for a gateway with no route")
	}
}

func TestReadCurrentState_InferenceUnsupportedNotCapable(t *testing.T) {
	ctx := context.Background()
	base := testutil.NewFake("default",
		fake.WithHealthResult(&types.HealthResult{Healthy: true, Version: "0.0.110"}),
	)
	client := &inferenceErrClient{Client: base, getErr: openshell.ErrUnsupported}

	state, err := ReadCurrentState(ctx, client, inferenceDesired())
	if err != nil {
		t.Fatalf("ReadCurrentState should degrade on ErrUnsupported: %v", err)
	}

	if state.Inference.Capable {
		t.Error("expected Capable=false when the gateway does not serve inference")
	}
}

// TestReadCurrentState_InferenceTransientErrorKeepsReachable pins finding-4
// behavior: health and providers already proved the gateway reachable, so a
// transient inference-read failure degrades inference to the not-capable
// validate fallback rather than flipping Reachable to false.
func TestReadCurrentState_InferenceTransientErrorKeepsReachable(t *testing.T) {
	ctx := context.Background()
	for _, transient := range []error{openshell.ErrUnavailable, openshell.ErrUnauthenticated} {
		base := testutil.NewFake("default",
			fake.WithHealthResult(&types.HealthResult{Healthy: true, Version: "0.0.110"}),
		)
		client := &inferenceErrClient{Client: base, getErr: transient}

		state, err := ReadCurrentState(ctx, client, inferenceDesired())
		if err != nil {
			t.Fatalf("%v: ReadCurrentState should degrade, got %v", transient, err)
		}
		if !state.Reachable {
			t.Errorf("%v: expected Reachable=true (health+providers succeeded)", transient)
		}
		if state.Inference.Capable {
			t.Errorf("%v: expected Inference.Capable=false (validate fallback)", transient)
		}
	}
}

func TestReadCurrentState_InferenceOtherErrorEscalates(t *testing.T) {
	ctx := context.Background()
	base := testutil.NewFake("default",
		fake.WithHealthResult(&types.HealthResult{Healthy: true, Version: "0.0.110"}),
	)
	client := &inferenceErrClient{Client: base, getErr: openshell.ErrPermission}

	if _, err := ReadCurrentState(ctx, client, inferenceDesired()); !errors.Is(err, openshell.ErrPermission) {
		t.Fatalf("expected ErrPermission to escalate, got %v", err)
	}
}

// inferenceErrClient wraps a healthy client but forces GetInferenceRoute to
// return a chosen error, exercising read paths the SDK fake cannot produce
// (e.g. ErrUnsupported).
type inferenceErrClient struct {
	openshell.Client
	getErr error
}

func (c *inferenceErrClient) GetInferenceRoute(context.Context, string) (openshell.InferenceRoute, error) {
	return openshell.InferenceRoute{}, c.getErr
}

func TestReadCurrentState_OnlyReadMethodsCalled(t *testing.T) {
	ctx := context.Background()
	client, fakeClient := testutil.NewFakeClient("default",
		fake.WithHealthResult(&types.HealthResult{Healthy: true, Version: "0.0.110"}),
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

func (r *recordingClient) GetInferenceRoute(ctx context.Context, route string) (openshell.InferenceRoute, error) {
	return r.wrapped.GetInferenceRoute(ctx, route)
}

func (r *recordingClient) SetInferenceRoute(ctx context.Context, cfg openshell.InferenceRouteConfig) (openshell.InferenceRoute, error) {
	return r.wrapped.SetInferenceRoute(ctx, cfg)
}

func (r *recordingClient) DeleteInferenceRoute(ctx context.Context, route string) error {
	return r.wrapped.DeleteInferenceRoute(ctx, route)
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

func (e *errorClient) GetInferenceRoute(ctx context.Context, route string) (openshell.InferenceRoute, error) {
	return openshell.InferenceRoute{}, e.err
}

func (e *errorClient) SetInferenceRoute(ctx context.Context, cfg openshell.InferenceRouteConfig) (openshell.InferenceRoute, error) {
	return openshell.InferenceRoute{}, e.err
}

func (e *errorClient) DeleteInferenceRoute(ctx context.Context, route string) error {
	return e.err
}

func (e *errorClient) Close() error {
	return nil
}
