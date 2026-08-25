package reconcile

import (
	"context"
	"errors"
	"testing"

	fake "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/fake"
	"github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/types"

	"github.com/stackrox/harness-openshell/internal/config"
	"github.com/stackrox/harness-openshell/internal/openshell"
	"github.com/stackrox/harness-openshell/internal/plan"
	"github.com/stackrox/harness-openshell/internal/testutil"
)

func healthyClient(t *testing.T) (openshell.Client, *fake.Client) {
	t.Helper()
	return testutil.NewFakeClient("default",
		fake.WithHealthResult(&types.HealthResult{Healthy: true, Version: "0.0.110"}),
	)
}

func TestReconcileInference_Create(t *testing.T) {
	ctx := context.Background()
	c, _ := healthyClient(t)
	desired := config.Inference{Provider: "gcp", Model: "claude-opus-4-8"}

	res, err := ReconcileInference(ctx, c, desired)
	if err != nil {
		t.Fatalf("ReconcileInference: %v", err)
	}
	if res.Action != plan.ActionCreate {
		t.Errorf("action = %s, want create", res.Action)
	}
	if res.Route.Provider != "gcp" || res.Route.Model != "claude-opus-4-8" {
		t.Errorf("route not populated: %+v", res.Route)
	}
	// The route is now readable under the resolved default name.
	got, err := c.GetInferenceRoute(ctx, plan.DefaultInferenceRoute)
	if err != nil {
		t.Fatalf("GetInferenceRoute after create: %v", err)
	}
	if got.Model != "claude-opus-4-8" {
		t.Errorf("persisted model = %q, want claude-opus-4-8", got.Model)
	}
}

func TestReconcileInference_Update(t *testing.T) {
	ctx := context.Background()
	c, _ := healthyClient(t)
	// Seed a route with a stale model under the resolved default name.
	if _, err := c.SetInferenceRoute(ctx, openshell.InferenceRouteConfig{
		Provider: "gcp", Model: "claude-sonnet-5", Route: plan.DefaultInferenceRoute,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	res, err := ReconcileInference(ctx, c, config.Inference{Provider: "gcp", Model: "claude-opus-4-8"})
	if err != nil {
		t.Fatalf("ReconcileInference: %v", err)
	}
	if res.Action != plan.ActionUpdate {
		t.Errorf("action = %s, want update", res.Action)
	}
	if res.Route.Model != "claude-opus-4-8" {
		t.Errorf("updated model = %q, want claude-opus-4-8", res.Route.Model)
	}
}

func TestReconcileInference_Noop(t *testing.T) {
	ctx := context.Background()
	c, _ := healthyClient(t)
	seed, err := c.SetInferenceRoute(ctx, openshell.InferenceRouteConfig{
		Provider: "gcp", Model: "claude-opus-4-8", Route: plan.DefaultInferenceRoute,
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	res, err := ReconcileInference(ctx, c, config.Inference{Provider: "gcp", Model: "claude-opus-4-8"})
	if err != nil {
		t.Fatalf("ReconcileInference: %v", err)
	}
	if res.Action != plan.ActionNoop {
		t.Errorf("action = %s, want noop", res.Action)
	}
	// A noop must not write: SetRoute increments Version, so the persisted
	// version must equal the seed's.
	got, err := c.GetInferenceRoute(ctx, plan.DefaultInferenceRoute)
	if err != nil {
		t.Fatalf("GetInferenceRoute: %v", err)
	}
	if got.Version != seed.Version {
		t.Errorf("noop wrote the route: version %d -> %d", seed.Version, got.Version)
	}
}

// TestReconcileInference_VerifyMapping pins the single NoVerify mapping site:
// unset verify → verify (NoVerify false); explicit false → NoVerify true.
func TestReconcileInference_VerifyMapping(t *testing.T) {
	ctx := context.Background()
	tru, fls := true, false
	tests := []struct {
		name         string
		verify       *bool
		wantNoVerify bool
	}{
		{name: "unset defaults to verify", verify: nil, wantNoVerify: false},
		{name: "explicit true verifies", verify: &tru, wantNoVerify: false},
		{name: "explicit false skips", verify: &fls, wantNoVerify: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base, _ := healthyClient(t)
			rec := &capturingClient{Client: base}
			_, err := ReconcileInference(ctx, rec, config.Inference{
				Provider: "gcp", Model: "claude-opus-4-8", Verify: tt.verify,
			})
			if err != nil {
				t.Fatalf("ReconcileInference: %v", err)
			}
			if !rec.setCalled {
				t.Fatal("expected SetInferenceRoute to be called")
			}
			if rec.setCfg.NoVerify != tt.wantNoVerify {
				t.Errorf("NoVerify = %v, want %v", rec.setCfg.NoVerify, tt.wantNoVerify)
			}
		})
	}
}

func TestReconcileInference_ExplicitTimeoutMapped(t *testing.T) {
	ctx := context.Background()
	base, _ := healthyClient(t)
	rec := &capturingClient{Client: base}
	if _, err := ReconcileInference(ctx, rec, config.Inference{
		Provider: "gcp", Model: "claude-opus-4-8", Timeout: "90s",
	}); err != nil {
		t.Fatalf("ReconcileInference: %v", err)
	}
	if rec.setCfg.TimeoutSecs != 90 {
		t.Errorf("TimeoutSecs = %d, want 90", rec.setCfg.TimeoutSecs)
	}
}

// TestReconcileInference_UpdateUnsetTimeoutWritesDefault pins the intended
// semantics (review finding #3): an update triggered by a model change with an
// unset desired timeout writes 0, i.e. resets the stored timeout to the gateway
// default. "Unset timeout" always means "let the gateway decide", even mid-update.
func TestReconcileInference_UpdateUnsetTimeoutWritesDefault(t *testing.T) {
	ctx := context.Background()
	base, _ := healthyClient(t)
	// Seed a route with a non-default timeout.
	if _, err := base.SetInferenceRoute(ctx, openshell.InferenceRouteConfig{
		Provider: "gcp", Model: "claude-sonnet-5", Route: plan.DefaultInferenceRoute, TimeoutSecs: 300,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	rec := &capturingClient{Client: base}
	// Change only the model, leaving timeout unset.
	res, err := ReconcileInference(ctx, rec, config.Inference{Provider: "gcp", Model: "claude-opus-4-8"})
	if err != nil {
		t.Fatalf("ReconcileInference: %v", err)
	}
	if res.Action != plan.ActionUpdate {
		t.Fatalf("action = %s, want update", res.Action)
	}
	if rec.setCfg.TimeoutSecs != 0 {
		t.Errorf("TimeoutSecs written = %d, want 0 (unset resets to gateway default)", rec.setCfg.TimeoutSecs)
	}
}

func TestReconcileInference_UnsupportedErrors(t *testing.T) {
	ctx := context.Background()
	base, _ := healthyClient(t)
	c := &getErrClient{Client: base, err: openshell.ErrUnsupported}
	_, err := ReconcileInference(ctx, c, config.Inference{Provider: "gcp", Model: "claude-opus-4-8"})
	if !errors.Is(err, openshell.ErrUnsupported) {
		t.Fatalf("expected ErrUnsupported, got %v", err)
	}
}

func TestReconcileInference_ReadErrorPropagates(t *testing.T) {
	ctx := context.Background()
	base, _ := healthyClient(t)
	for _, want := range []error{openshell.ErrUnavailable, openshell.ErrPermission} {
		c := &getErrClient{Client: base, err: want}
		_, err := ReconcileInference(ctx, c, config.Inference{Provider: "gcp", Model: "claude-opus-4-8"})
		if !errors.Is(err, want) {
			t.Errorf("expected %v to propagate (no degradation), got %v", want, err)
		}
	}
}

func TestReconcileInference_WriteErrorPropagates(t *testing.T) {
	ctx := context.Background()
	base, _ := healthyClient(t)
	c := &setErrClient{Client: base, err: openshell.ErrPermission}
	_, err := ReconcileInference(ctx, c, config.Inference{Provider: "gcp", Model: "claude-opus-4-8"})
	if !errors.Is(err, openshell.ErrPermission) {
		t.Fatalf("expected write ErrPermission to propagate, got %v", err)
	}
}

// TestReconcileMatchesPlanAction locks the feature-level invariant that the
// read-only plan and the reconcile write agree on the action for the same gateway
// state (both route through plan.InferenceAction). Guards against future drift.
func TestReconcileMatchesPlanAction(t *testing.T) {
	ctx := context.Background()
	desired := config.Inference{Provider: "gcp", Model: "claude-opus-4-8"}

	cases := []struct {
		name string
		seed *openshell.InferenceRouteConfig // nil = no route
		want plan.Action
	}{
		{name: "absent -> create", seed: nil, want: plan.ActionCreate},
		{name: "matching -> noop", seed: &openshell.InferenceRouteConfig{Provider: "gcp", Model: "claude-opus-4-8", Route: plan.DefaultInferenceRoute}, want: plan.ActionNoop},
		{name: "stale -> update", seed: &openshell.InferenceRouteConfig{Provider: "gcp", Model: "claude-sonnet-5", Route: plan.DefaultInferenceRoute}, want: plan.ActionUpdate},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := healthyClient(t)
			if tc.seed != nil {
				if _, err := c.SetInferenceRoute(ctx, *tc.seed); err != nil {
					t.Fatalf("seed: %v", err)
				}
			}
			// Plan action from the read-only path.
			cur, err := plan.ReadInferenceState(ctx, c, desired)
			if err != nil {
				t.Fatalf("ReadInferenceState: %v", err)
			}
			planAction := plan.InferenceAction(desired, cur)
			// Reconcile action from the write path against the same state.
			res, err := ReconcileInference(ctx, c, desired)
			if err != nil {
				t.Fatalf("ReconcileInference: %v", err)
			}
			if planAction != tc.want || res.Action != tc.want {
				t.Errorf("plan=%s reconcile=%s, want %s", planAction, res.Action, tc.want)
			}
		})
	}
}

// capturingClient records the last SetInferenceRoute config while delegating to
// a real fake-backed client, so the outbound mapping can be asserted.
type capturingClient struct {
	openshell.Client
	setCalled bool
	setCfg    openshell.InferenceRouteConfig
}

func (c *capturingClient) SetInferenceRoute(ctx context.Context, cfg openshell.InferenceRouteConfig) (openshell.InferenceRoute, error) {
	c.setCalled = true
	c.setCfg = cfg
	return c.Client.SetInferenceRoute(ctx, cfg)
}

// getErrClient forces GetInferenceRoute to a chosen error.
type getErrClient struct {
	openshell.Client
	err error
}

func (c *getErrClient) GetInferenceRoute(context.Context, string) (openshell.InferenceRoute, error) {
	return openshell.InferenceRoute{}, c.err
}

// setErrClient reads normally but forces SetInferenceRoute to a chosen error.
type setErrClient struct {
	openshell.Client
	err error
}

func (c *setErrClient) SetInferenceRoute(context.Context, openshell.InferenceRouteConfig) (openshell.InferenceRoute, error) {
	return openshell.InferenceRoute{}, c.err
}
