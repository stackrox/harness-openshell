package sdkclient

import (
	"context"
	"errors"
	"testing"
	"time"

	fake "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/fake"
	"github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/types"

	"github.com/stackrox/harness-openshell/internal/openshell"
)

// TestFromSDKProviderMapsConfigAndLabels pins the S1 read-widening: the harness
// Provider carries the SDK provider's non-secret Config and Labels, as fresh
// copies, and never the secret Spec fields.
func TestFromSDKProviderMapsConfigAndLabels(t *testing.T) {
	sdkConfig := map[string]string{"VERTEX_AI_REGION": "global"}
	sdkLabels := map[string]string{"harness.openshell.dev/managed-by": "harness"}
	p := &types.Provider{
		Name:            "google-vertex-ai",
		Type:            "google-vertex-ai",
		ResourceVersion: 7,
		Labels:          sdkLabels,
		Spec: types.ProviderSpec{
			Config:      sdkConfig,
			Credentials: map[string]string{"API_KEY": "secret"}, // must NOT cross
			CredentialHandles: map[string]types.CredentialHandle{
				"API_KEY": {Driver: "vault", Handle: "h1"},
			},
			CredentialExpiresAt: map[string]time.Time{"API_KEY": {}},
		},
	}

	got := fromSDKProvider(p)

	if got.Name != "google-vertex-ai" || got.Type != "google-vertex-ai" {
		t.Fatalf("name/type not mapped: %+v", got)
	}
	if got.Config["VERTEX_AI_REGION"] != "global" {
		t.Errorf("Config not mapped: %v", got.Config)
	}
	if got.Labels["harness.openshell.dev/managed-by"] != "harness" {
		t.Errorf("Labels not mapped: %v", got.Labels)
	}

	// Fresh copies: mutating the source must not affect the harness view.
	sdkConfig["VERTEX_AI_REGION"] = "us-east1"
	sdkLabels["harness.openshell.dev/managed-by"] = "someone-else"
	if got.Config["VERTEX_AI_REGION"] != "global" {
		t.Errorf("Config aliases the SDK map: %v", got.Config)
	}
	if got.Labels["harness.openshell.dev/managed-by"] != "harness" {
		t.Errorf("Labels aliases the SDK map: %v", got.Labels)
	}
}

// TestFromSDKProviderEmptyMapsAreNil keeps the harness view tidy: absent
// Config/Labels map to nil, not empty non-nil maps.
func TestFromSDKProviderEmptyMapsAreNil(t *testing.T) {
	got := fromSDKProvider(&types.Provider{Name: "p", Type: "openai"})
	if got.Config != nil {
		t.Errorf("expected nil Config, got %v", got.Config)
	}
	if got.Labels != nil {
		t.Errorf("expected nil Labels, got %v", got.Labels)
	}
}

// TestGetProvider covers the read path: fields map through, and a missing
// provider surfaces as openshell.ErrNotFound.
func TestGetProvider(t *testing.T) {
	ctx := context.Background()
	fc := fake.NewClient()
	fc.AddProvider("default", &types.Provider{
		Name: "github", Type: "github",
		Spec: types.ProviderSpec{Config: map[string]string{"k": "v"}},
	})
	c := NewFromClient(fc, "default")

	got, err := c.GetProvider(ctx, "github")
	if err != nil {
		t.Fatalf("GetProvider: %v", err)
	}
	if got.Name != "github" || got.Config["k"] != "v" {
		t.Errorf("unexpected provider: %+v", got)
	}

	if _, err := c.GetProvider(ctx, "absent"); !errors.Is(err, openshell.ErrNotFound) {
		t.Errorf("GetProvider(absent): want ErrNotFound, got %v", err)
	}
}

// TestUpdateProviderOverlaysConfigPreservesCredentials pins the copy-through:
// UpdateProvider changes only Config/Labels and leaves the stored credentials
// and handles intact.
//
// NOTE ON THE FAKE: this passes because the fake's Get RETURNS the stored
// credentials, so the copy-through carries them back. A real gateway's Get
// returns an EMPTY credentials map (write-only), so this test proves the
// harness overlay logic never drops what Get gave it — NOT the real
// empty-map-means-leave semantic, which only the gated
// TestLiveProviderUpdatePreservesCredentials can prove. The fake also does not
// enforce ResourceVersion OCC, so this asserts neither.
func TestUpdateProviderOverlaysConfigPreservesCredentials(t *testing.T) {
	ctx := context.Background()
	fc := fake.NewClient()
	fc.AddProvider("default", &types.Provider{
		Name: "google-vertex-ai", Type: "google-vertex-ai",
		Spec: types.ProviderSpec{
			Config:      map[string]string{"VERTEX_AI_REGION": "global"},
			Credentials: map[string]string{"API_KEY": "secret"},
			CredentialHandles: map[string]types.CredentialHandle{
				"API_KEY": {Driver: "vault", Handle: "h1"},
			},
		},
	})
	c := NewFromClient(fc, "default")

	out, err := c.UpdateProvider(ctx, openshell.Provider{
		Name:   "google-vertex-ai",
		Config: map[string]string{"VERTEX_AI_REGION": "us-east1"},
		Labels: map[string]string{"harness.openshell.dev/managed-by": "harness"},
	})
	if err != nil {
		t.Fatalf("UpdateProvider: %v", err)
	}
	if out.Config["VERTEX_AI_REGION"] != "us-east1" {
		t.Errorf("Config not updated in returned view: %v", out.Config)
	}

	// Inspect the raw stored object: creds + handles survived the overlay.
	stored, err := fc.Providers().Get(ctx, "default", "google-vertex-ai")
	if err != nil {
		t.Fatalf("raw Get: %v", err)
	}
	if stored.Spec.Credentials["API_KEY"] != "secret" {
		t.Errorf("credentials clobbered by update: %v", stored.Spec.Credentials)
	}
	if _, ok := stored.Spec.CredentialHandles["API_KEY"]; !ok {
		t.Errorf("credential handles clobbered by update: %v", stored.Spec.CredentialHandles)
	}
	if stored.Spec.Config["VERTEX_AI_REGION"] != "us-east1" {
		t.Errorf("stored Config not updated: %v", stored.Spec.Config)
	}
	if stored.Labels["harness.openshell.dev/managed-by"] != "harness" {
		t.Errorf("stored Labels not updated: %v", stored.Labels)
	}
}

// TestUpdateProviderWritesType: a declared Type is overlaid onto the stored
// provider, so a type delta reported as Update by plan.ProviderAction actually
// converges. An empty desired Type leaves the stored one untouched.
func TestUpdateProviderWritesType(t *testing.T) {
	ctx := context.Background()
	fc := fake.NewClient()
	fc.AddProvider("default", &types.Provider{
		Name: "gcp", Type: "old-type",
		Spec: types.ProviderSpec{Config: map[string]string{"K": "v"}},
	})
	c := NewFromClient(fc, "default")

	// Declared Type is written.
	if _, err := c.UpdateProvider(ctx, openshell.Provider{
		Name: "gcp", Type: "new-type", Config: map[string]string{"K": "v"},
	}); err != nil {
		t.Fatalf("UpdateProvider: %v", err)
	}
	stored, err := fc.Providers().Get(ctx, "default", "gcp")
	if err != nil {
		t.Fatalf("raw Get: %v", err)
	}
	if stored.Type != "new-type" {
		t.Errorf("Type not written: got %q, want new-type", stored.Type)
	}

	// Empty desired Type preserves the stored one.
	if _, err := c.UpdateProvider(ctx, openshell.Provider{
		Name: "gcp", Config: map[string]string{"K": "v2"},
	}); err != nil {
		t.Fatalf("UpdateProvider (empty type): %v", err)
	}
	stored, err = fc.Providers().Get(ctx, "default", "gcp")
	if err != nil {
		t.Fatalf("raw Get: %v", err)
	}
	if stored.Type != "new-type" {
		t.Errorf("empty desired Type wiped stored Type: got %q, want new-type", stored.Type)
	}
}

// TestUpdateProviderNotFound: updating an absent provider surfaces ErrNotFound
// from the internal Get, never a nil-object write.
func TestUpdateProviderNotFound(t *testing.T) {
	ctx := context.Background()
	c := NewFromClient(fake.NewClient(), "default")
	if _, err := c.UpdateProvider(ctx, openshell.Provider{Name: "absent"}); !errors.Is(err, openshell.ErrNotFound) {
		t.Errorf("UpdateProvider(absent): want ErrNotFound, got %v", err)
	}
}
