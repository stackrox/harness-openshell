package reconcile

import (
	"context"
	"errors"
	"testing"

	"github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/types"

	"github.com/stackrox/harness-openshell/internal/config"
	"github.com/stackrox/harness-openshell/internal/openshell"
	"github.com/stackrox/harness-openshell/internal/plan"
)

// ownerLabels returns the harness owner label set, marking a seeded provider as
// harness-owned so managed reconcile treats it as adoptable-in-place, not drift.
func ownerLabels() map[string]string {
	return map[string]string{plan.OwnerLabelKey: plan.OwnerLabelValue}
}

// TestReconcileProviders_ReferencedVerify: a referenced provider that exists is a
// noop — the successful Get is the verification — and nothing is written.
func TestReconcileProviders_ReferencedVerify(t *testing.T) {
	ctx := context.Background()
	c, raw := healthyClient(t)
	raw.AddProvider("default", &types.Provider{Name: "ext", Type: "custom"})
	rec := &capturingProviderClient{Client: c}

	res, err := ReconcileProviders(ctx, rec, []config.Provider{
		{Name: "ext", Type: "custom", Management: "referenced"},
	})
	if err != nil {
		t.Fatalf("ReconcileProviders: %v", err)
	}
	if len(res) != 1 || res[0].Action != plan.ActionNoop {
		t.Fatalf("want one noop result, got %+v", res)
	}
	if rec.updateCalled {
		t.Error("referenced verify must not write")
	}
}

// TestReconcileProviders_ManagedNoop: an owned managed provider whose config
// matches is a noop with no write.
func TestReconcileProviders_ManagedNoop(t *testing.T) {
	ctx := context.Background()
	c, raw := healthyClient(t)
	raw.AddProvider("default", &types.Provider{
		Name: "gcp", Type: "google-vertex-ai", Labels: ownerLabels(),
		Spec: types.ProviderSpec{Config: map[string]string{"VERTEX_AI_REGION": "global"}},
	})
	rec := &capturingProviderClient{Client: c}

	res, err := ReconcileProviders(ctx, rec, []config.Provider{
		{Name: "gcp", Type: "google-vertex-ai", Management: "managed", Config: map[string]string{"VERTEX_AI_REGION": "global"}},
	})
	if err != nil {
		t.Fatalf("ReconcileProviders: %v", err)
	}
	if res[0].Action != plan.ActionNoop {
		t.Errorf("action = %s, want noop", res[0].Action)
	}
	if rec.updateCalled {
		t.Error("matching managed provider must not write")
	}
}

// TestReconcileProviders_ManagedUpdateConfigDrift: an owned managed provider with
// drifted config is updated, and the Provider handed to UpdateProvider carries
// the desired config merged over the current, plus the owner label — and extra
// unmanaged keys survive.
func TestReconcileProviders_ManagedUpdateConfigDrift(t *testing.T) {
	ctx := context.Background()
	c, raw := healthyClient(t)
	raw.AddProvider("default", &types.Provider{
		Name: "gcp", Type: "google-vertex-ai", Labels: ownerLabels(),
		Spec: types.ProviderSpec{
			Config:      map[string]string{"VERTEX_AI_REGION": "global", "UNMANAGED": "keep"},
			Credentials: map[string]string{"API_KEY": "secret"},
		},
	})
	rec := &capturingProviderClient{Client: c}

	res, err := ReconcileProviders(ctx, rec, []config.Provider{
		{Name: "gcp", Type: "google-vertex-ai", Management: "managed", Config: map[string]string{"VERTEX_AI_REGION": "us-east1"}},
	})
	if err != nil {
		t.Fatalf("ReconcileProviders: %v", err)
	}
	if res[0].Action != plan.ActionUpdate {
		t.Fatalf("action = %s, want update", res[0].Action)
	}
	if !rec.updateCalled {
		t.Fatal("config drift must trigger a write")
	}
	got := rec.updateArg
	if got.Config["VERTEX_AI_REGION"] != "us-east1" {
		t.Errorf("desired config not carried to Update: %v", got.Config)
	}
	if got.Config["UNMANAGED"] != "keep" {
		t.Errorf("unmanaged config key wiped by update: %v", got.Config)
	}
	if !plan.IsOwned(got) {
		t.Errorf("owner label not stamped on Update: %v", got.Labels)
	}
	// The fake's Get returns stored credentials, so the copy-through preserves
	// them; this proves the overlay logic, not the real empty-map semantic (that
	// is the S2 live gate's job).
	stored, err := raw.Providers().Get(ctx, "default", "gcp")
	if err != nil {
		t.Fatalf("raw Get: %v", err)
	}
	if stored.Spec.Credentials["API_KEY"] != "secret" {
		t.Errorf("credentials clobbered by update: %v", stored.Spec.Credentials)
	}
}

// TestReconcileProviders_AdoptStampsOwnerLabel: with adopt:true an unowned
// provider is taken over — the write stamps the owner label (the label delta that
// made this an update).
func TestReconcileProviders_AdoptStampsOwnerLabel(t *testing.T) {
	ctx := context.Background()
	c, raw := healthyClient(t)
	raw.AddProvider("default", &types.Provider{Name: "gcp", Type: "google-vertex-ai"}) // no owner label
	rec := &capturingProviderClient{Client: c}

	res, err := ReconcileProviders(ctx, rec, []config.Provider{
		{Name: "gcp", Type: "google-vertex-ai", Management: "managed", Adopt: true},
	})
	if err != nil {
		t.Fatalf("ReconcileProviders: %v", err)
	}
	if res[0].Action != plan.ActionUpdate {
		t.Fatalf("action = %s, want update", res[0].Action)
	}
	if !plan.IsOwned(rec.updateArg) {
		t.Errorf("adopt did not stamp the owner label: %v", rec.updateArg.Labels)
	}
}

// TestReconcileProviders_UnownedAdoptionRequiredNoWrite: an existing unowned
// managed provider without adopt is reported adoption-required and never written.
func TestReconcileProviders_UnownedAdoptionRequiredNoWrite(t *testing.T) {
	ctx := context.Background()
	c, raw := healthyClient(t)
	raw.AddProvider("default", &types.Provider{Name: "gcp", Type: "google-vertex-ai"})
	rec := &capturingProviderClient{Client: c}

	res, err := ReconcileProviders(ctx, rec, []config.Provider{
		{Name: "gcp", Type: "google-vertex-ai", Management: "managed"},
	})
	if err != nil {
		t.Fatalf("ReconcileProviders: %v", err)
	}
	if res[0].Action != plan.ActionAdoptionRequired {
		t.Errorf("action = %s, want adoption-required", res[0].Action)
	}
	if rec.updateCalled {
		t.Error("adoption-required must not write an unowned provider")
	}
}

// TestReconcileProviders_ManagedAbsentReturnsCreateNoWrite: a managed provider
// that does not exist is reported create without any SDK write (invariant 26).
func TestReconcileProviders_ManagedAbsentReturnsCreateNoWrite(t *testing.T) {
	ctx := context.Background()
	c, _ := healthyClient(t)
	rec := &capturingProviderClient{Client: c}

	res, err := ReconcileProviders(ctx, rec, []config.Provider{
		{Name: "gcp", Type: "google-vertex-ai", Management: "managed"},
	})
	if err != nil {
		t.Fatalf("ReconcileProviders: %v", err)
	}
	if res[0].Action != plan.ActionCreate {
		t.Errorf("action = %s, want create", res[0].Action)
	}
	if res[0].Provider.Name != "gcp" || res[0].Provider.Type != "google-vertex-ai" {
		t.Errorf("create result should echo name/type: %+v", res[0].Provider)
	}
	if rec.updateCalled {
		t.Error("reconcile must not SDK-create (invariant 26)")
	}
}

// TestReconcileProviders_ReferencedAbsentErrors: a referenced provider that does
// not exist is a hard error (unusable).
func TestReconcileProviders_ReferencedAbsentErrors(t *testing.T) {
	ctx := context.Background()
	c, _ := healthyClient(t)

	_, err := ReconcileProviders(ctx, c, []config.Provider{
		{Name: "ext", Management: "referenced"},
	})
	if !errors.Is(err, openshell.ErrNotFound) {
		t.Fatalf("want ErrNotFound for absent referenced provider, got %v", err)
	}
}

// TestReconcileProviders_ReadErrorPropagates: a non-NotFound read error is
// returned, never degraded (a write path must report it did not run).
func TestReconcileProviders_ReadErrorPropagates(t *testing.T) {
	ctx := context.Background()
	base, _ := healthyClient(t)
	for _, want := range []error{openshell.ErrUnavailable, openshell.ErrPermission} {
		c := &providerGetErrClient{Client: base, err: want}
		_, err := ReconcileProviders(ctx, c, []config.Provider{
			{Name: "gcp", Type: "google-vertex-ai", Management: "managed"},
		})
		if !errors.Is(err, want) {
			t.Errorf("expected %v to propagate, got %v", want, err)
		}
	}
}

// TestReconcileProviders_WriteErrorPropagates: an Update failure is returned.
func TestReconcileProviders_WriteErrorPropagates(t *testing.T) {
	ctx := context.Background()
	base, raw := healthyClient(t)
	raw.AddProvider("default", &types.Provider{Name: "gcp", Type: "google-vertex-ai", Labels: ownerLabels()})
	c := &providerUpdateErrClient{Client: base, err: openshell.ErrPermission}

	_, err := ReconcileProviders(ctx, c, []config.Provider{
		{Name: "gcp", Type: "google-vertex-ai", Management: "managed", Config: map[string]string{"K": "v"}},
	})
	if !errors.Is(err, openshell.ErrPermission) {
		t.Fatalf("expected write ErrPermission to propagate, got %v", err)
	}
}

// TestReconcileMatchesPlanProviderAction locks invariant 22: the read-only plan
// and the reconcile write agree on the action for the same gateway state (both
// route through plan.ProviderAction).
func TestReconcileMatchesPlanProviderAction(t *testing.T) {
	ctx := context.Background()
	desired := config.Provider{Name: "gcp", Type: "google-vertex-ai", Management: "managed"}

	cases := []struct {
		name string
		seed *types.Provider // nil = absent
		want plan.Action
	}{
		{name: "absent -> create", seed: nil, want: plan.ActionCreate},
		{name: "unowned -> adoption-required", seed: &types.Provider{Name: "gcp", Type: "google-vertex-ai"}, want: plan.ActionAdoptionRequired},
		{name: "owned match -> noop", seed: &types.Provider{Name: "gcp", Type: "google-vertex-ai", Labels: ownerLabels()}, want: plan.ActionNoop},
		{name: "owned type drift -> update", seed: &types.Provider{Name: "gcp", Type: "old", Labels: ownerLabels()}, want: plan.ActionUpdate},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, raw := healthyClient(t)
			if tc.seed != nil {
				raw.AddProvider("default", tc.seed)
			}

			// Plan action from the read path.
			var curPtr *openshell.Provider
			cur, err := c.GetProvider(ctx, desired.Name)
			switch {
			case err == nil:
				curPtr = &cur
			case errors.Is(err, openshell.ErrNotFound):
				curPtr = nil
			default:
				t.Fatalf("GetProvider: %v", err)
			}
			planAction := plan.ProviderAction(desired, curPtr)

			// Reconcile action from the write path against the same state.
			res, err := ReconcileProviders(ctx, c, []config.Provider{desired})
			if err != nil {
				t.Fatalf("ReconcileProviders: %v", err)
			}
			if planAction != tc.want || res[0].Action != tc.want {
				t.Errorf("plan=%s reconcile=%s, want %s", planAction, res[0].Action, tc.want)
			}
		})
	}
}

// capturingProviderClient records the last UpdateProvider argument while
// delegating to a real fake-backed client, so the outbound Provider can be
// asserted and writes can be detected.
type capturingProviderClient struct {
	openshell.Client
	updateCalled bool
	updateArg    openshell.Provider
}

func (c *capturingProviderClient) UpdateProvider(ctx context.Context, p openshell.Provider) (openshell.Provider, error) {
	c.updateCalled = true
	c.updateArg = p
	return c.Client.UpdateProvider(ctx, p)
}

// providerGetErrClient forces GetProvider to a chosen error.
type providerGetErrClient struct {
	openshell.Client
	err error
}

func (c *providerGetErrClient) GetProvider(context.Context, string) (openshell.Provider, error) {
	return openshell.Provider{}, c.err
}

// providerUpdateErrClient reads normally but forces UpdateProvider to an error.
type providerUpdateErrClient struct {
	openshell.Client
	err error
}

func (c *providerUpdateErrClient) UpdateProvider(context.Context, openshell.Provider) (openshell.Provider, error) {
	return openshell.Provider{}, c.err
}
