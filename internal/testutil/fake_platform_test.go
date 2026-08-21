package testutil

import (
	"context"
	"testing"

	fake "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/fake"
	"github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/types"

	"github.com/stackrox/harness-openshell/internal/openshell"
)

func TestHealthRoundTrip(t *testing.T) {
	ctx := context.Background()
	c := NewFake("default", fake.WithHealthResult(&types.HealthResult{
		Healthy: true,
		Version: "1.2.3",
	}))

	h, err := c.Health(ctx)
	if err != nil {
		t.Fatalf("Health() returned unexpected error: %v", err)
	}
	if !h.Healthy {
		t.Errorf("expected Healthy=true, got %v", h.Healthy)
	}
	if h.Version != "1.2.3" {
		t.Errorf("expected Version=%q, got %q", "1.2.3", h.Version)
	}
}

func TestProvidersRoundTrip(t *testing.T) {
	ctx := context.Background()
	c, raw := NewFakeClient("default")
	raw.AddProvider("default", &types.Provider{
		Name: "p1",
		Type: "openai",
	})

	providers, err := c.Providers(ctx)
	if err != nil {
		t.Fatalf("Providers() returned unexpected error: %v", err)
	}
	if len(providers) != 1 {
		t.Errorf("expected 1 provider, got %d", len(providers))
	}
	if providers[0].Name != "p1" {
		t.Errorf("expected Name=%q, got %q", "p1", providers[0].Name)
	}
	if providers[0].Type != "openai" {
		t.Errorf("expected Type=%q, got %q", "openai", providers[0].Type)
	}
}

func TestEmptyProviders(t *testing.T) {
	ctx := context.Background()
	c := NewFake("default")

	providers, err := c.Providers(ctx)
	if err != nil {
		t.Fatalf("Providers() returned unexpected error: %v", err)
	}
	if len(providers) != 0 {
		t.Errorf("expected 0 providers, got %d", len(providers))
	}
}

func TestFakeFactory(t *testing.T) {
	ctx := context.Background()
	c := NewFake("default")
	f := FakeFactory(c)

	got, err := f(ctx, openshell.Target{Gateway: "anything", Workspace: "x"})
	if err != nil {
		t.Errorf("FakeFactory returned unexpected error: %v", err)
	}
	if got != c {
		t.Errorf("expected returned client to be the same as input")
	}
}
