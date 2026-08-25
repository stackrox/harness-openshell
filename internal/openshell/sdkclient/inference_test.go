package sdkclient

import (
	"context"
	"errors"
	"testing"

	fake "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/fake"

	"github.com/stackrox/harness-openshell/internal/openshell"
)

// TestInferenceRoundTrip exercises the full get/set/update/delete lifecycle
// against the SDK fake through the real sdkclient mapping and translation. It
// pins the server-assigned version semantics (1 on create, monotonic on update)
// and that a missing route surfaces the harness ErrNotFound sentinel rather than
// a raw SDK error.
func TestInferenceRoundTrip(t *testing.T) {
	ctx := context.Background()
	c := NewFromClient(fake.NewClient(), "default")

	// Get on an empty gateway -> ErrNotFound (translated sentinel).
	if _, err := c.GetInferenceRoute(ctx, ""); !errors.Is(err, openshell.ErrNotFound) {
		t.Fatalf("GetInferenceRoute on empty gateway: want ErrNotFound, got %v", err)
	}

	// Set creates the route at version 1.
	created, err := c.SetInferenceRoute(ctx, openshell.InferenceRouteConfig{
		Provider:    "gcp",
		Model:       "claude-opus-4-8",
		NoVerify:    true,
		TimeoutSecs: 90,
	})
	if err != nil {
		t.Fatalf("SetInferenceRoute create: %v", err)
	}
	if created.Version != 1 {
		t.Errorf("created version: want 1, got %d", created.Version)
	}
	if created.Provider != "gcp" || created.Model != "claude-opus-4-8" {
		t.Errorf("created route mismatch: %+v", created)
	}
	if created.TimeoutSecs != 90 {
		t.Errorf("created TimeoutSecs: want 90, got %d", created.TimeoutSecs)
	}

	// Get returns the created route.
	got, err := c.GetInferenceRoute(ctx, "")
	if err != nil {
		t.Fatalf("GetInferenceRoute after create: %v", err)
	}
	if got.Provider != created.Provider || got.Model != created.Model || got.Version != created.Version {
		t.Errorf("round-trip mismatch: set %+v, got %+v", created, got)
	}

	// Set with a changed model upserts and bumps the version to 2.
	updated, err := c.SetInferenceRoute(ctx, openshell.InferenceRouteConfig{
		Provider: "gcp",
		Model:    "claude-sonnet-5",
		NoVerify: true,
	})
	if err != nil {
		t.Fatalf("SetInferenceRoute update: %v", err)
	}
	if updated.Version != 2 {
		t.Errorf("updated version: want 2, got %d", updated.Version)
	}
	if updated.Model != "claude-sonnet-5" {
		t.Errorf("updated model: want claude-sonnet-5, got %q", updated.Model)
	}

	// Delete removes the route; a subsequent get is ErrNotFound again.
	if err := c.DeleteInferenceRoute(ctx, ""); err != nil {
		t.Fatalf("DeleteInferenceRoute: %v", err)
	}
	if _, err := c.GetInferenceRoute(ctx, ""); !errors.Is(err, openshell.ErrNotFound) {
		t.Fatalf("GetInferenceRoute after delete: want ErrNotFound, got %v", err)
	}
}

// TestInferenceDeleteIdempotent pins the firewall contract that deleting a
// missing route is not an error (mirrors the SDK's idempotent DeleteRoute).
func TestInferenceDeleteIdempotent(t *testing.T) {
	ctx := context.Background()
	c := NewFromClient(fake.NewClient(), "default")

	if err := c.DeleteInferenceRoute(ctx, ""); err != nil {
		t.Fatalf("DeleteInferenceRoute on empty gateway: want nil, got %v", err)
	}
}

// TestInferenceErrorsTranslated proves all three methods route SDK errors
// through translate to harness sentinels (not raw SDK errors). It uses the
// closed-client trick — the same convention as TestHealthErrorTranslated /
// TestProvidersErrorTranslated — under which the fake returns ErrorUnavailable.
// Without this, a regression dropping translate() on the Set/Delete success
// wrappers would go unnoticed (their success paths never error).
func TestInferenceErrorsTranslated(t *testing.T) {
	ctx := context.Background()
	c := NewFromClient(fake.NewClient(), "default")
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if _, err := c.GetInferenceRoute(ctx, ""); !errors.Is(err, openshell.ErrUnavailable) {
		t.Errorf("GetInferenceRoute on closed client: want ErrUnavailable, got %v", err)
	}
	if _, err := c.SetInferenceRoute(ctx, openshell.InferenceRouteConfig{
		Provider: "gcp", Model: "claude-opus-4-8",
	}); !errors.Is(err, openshell.ErrUnavailable) {
		t.Errorf("SetInferenceRoute on closed client: want ErrUnavailable, got %v", err)
	}
	if err := c.DeleteInferenceRoute(ctx, ""); !errors.Is(err, openshell.ErrUnavailable) {
		t.Errorf("DeleteInferenceRoute on closed client: want ErrUnavailable, got %v", err)
	}
}

// TestInferenceInvalidArgumentTranslated proves user-supplied invalid input
// (a required field left empty) surfaces the harness ErrInvalidArgument sentinel
// rather than a raw SDK *StatusError. Inference is the first firewall caller
// whose required-field input can trigger this.
func TestInferenceInvalidArgumentTranslated(t *testing.T) {
	ctx := context.Background()
	c := NewFromClient(fake.NewClient(), "default")

	// Empty model (required) -> InvalidArgument from the SDK.
	if _, err := c.SetInferenceRoute(ctx, openshell.InferenceRouteConfig{
		Provider: "gcp",
	}); !errors.Is(err, openshell.ErrInvalidArgument) {
		t.Errorf("SetInferenceRoute with empty model: want ErrInvalidArgument, got %v", err)
	}
}

// TestInferenceNamedRoute proves a non-default route name round-trips
// independently of the default route.
func TestInferenceNamedRoute(t *testing.T) {
	ctx := context.Background()
	c := NewFromClient(fake.NewClient(), "default")

	if _, err := c.SetInferenceRoute(ctx, openshell.InferenceRouteConfig{
		Provider: "gcp",
		Model:    "claude-opus-4-8",
		Route:    "scratch",
		NoVerify: true,
	}); err != nil {
		t.Fatalf("SetInferenceRoute named: %v", err)
	}

	got, err := c.GetInferenceRoute(ctx, "scratch")
	if err != nil {
		t.Fatalf("GetInferenceRoute named: %v", err)
	}
	if got.Route != "scratch" {
		t.Errorf("route name: want scratch, got %q", got.Route)
	}

	// The default route remains absent.
	if _, err := c.GetInferenceRoute(ctx, ""); !errors.Is(err, openshell.ErrNotFound) {
		t.Fatalf("default route should be absent: got %v", err)
	}
}
