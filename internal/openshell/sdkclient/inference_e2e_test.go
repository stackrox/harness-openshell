package sdkclient_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/stackrox/harness-openshell/internal/openshell"
	"github.com/stackrox/harness-openshell/internal/openshell/sdkclient"
)

// defaultRoute mirrors plan.DefaultInferenceRoute. It is duplicated here (not
// imported) to keep the firewall's e2e test from depending on the plan package.
// A real 0.0.110 gateway accepts only a fixed set of route names —
// "inference.local" and "sandbox-system" — and rejects any other with
// InvalidArgument (verified live 2026-08-25); the harness only ever uses
// "inference.local".
const defaultRoute = "inference.local"

// TestLiveInferenceRoleProbe verifies the harness mTLS identity can serve the
// inference surface reconcile depends on: the user-role read path always, and —
// when a credentialed provider is supplied — the admin-role write path.
//
// It is skipped unless HARNESS_E2E_GATEWAY names a registered mTLS gateway (the
// same gate as the other live checks). Optional HARNESS_E2E_WORKSPACE overrides
// the workspace.
//
// The admin-role write is the S1 risk gate for PR4b: SetInferenceRoute requires
// the workspace "admin" role, GetInferenceRoute only "user". But a live gateway
// checks Set's preconditions BEFORE the role — the route name must be valid, the
// provider must exist in the workspace, and it must carry a usable credential —
// so the role can only be probed once a credentialed provider exists. Supply its
// name via HARNESS_E2E_INFERENCE_PROVIDER to exercise the write path; without it
// the write probe is skipped (admin was confirmed present on OCP 2026-08-25).
//
//	HARNESS_E2E_GATEWAY=openshell go test ./internal/openshell/sdkclient/ -run LiveInferenceRoleProbe -v
func TestLiveInferenceRoleProbe(t *testing.T) {
	gw := os.Getenv("HARNESS_E2E_GATEWAY")
	if gw == "" {
		t.Skip("set HARNESS_E2E_GATEWAY to a registered mTLS gateway to run the inference role probe")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	c, err := sdkclient.New(ctx, openshell.Target{
		Gateway:   gw,
		Workspace: os.Getenv("HARNESS_E2E_WORKSPACE"),
	})
	if err != nil {
		t.Fatalf("sdkclient.New(%q): %v", gw, err)
	}
	defer c.Close()

	// Read path (user role). On the default route the gateway returns the route
	// if configured, or ErrNotFound if not — both mean the identity can read and
	// the gateway serves inference. ErrPermission/ErrUnavailable/InvalidArgument
	// are all failures of the surface the harness needs.
	if _, err := c.GetInferenceRoute(ctx, defaultRoute); err != nil && !errors.Is(err, openshell.ErrNotFound) {
		t.Fatalf("GetInferenceRoute(%q) read path failed: %v", defaultRoute, err)
	}
	t.Logf("read path OK on gateway %q (user role confirmed, gateway serves inference)", gw)

	provider := os.Getenv("HARNESS_E2E_INFERENCE_PROVIDER")
	if provider == "" {
		t.Skip("set HARNESS_E2E_INFERENCE_PROVIDER to a registered, credentialed provider to probe the admin write path")
	}

	// Write path (admin role) on the default route. NoVerify skips endpoint
	// validation so the probe measures the role, not the provider's credentials.
	// Read the current route first so cleanup can restore it (the write bumps
	// Version); if it was unconfigured, delete to restore that state.
	before, beforeErr := c.GetInferenceRoute(ctx, defaultRoute)
	existed := beforeErr == nil

	_, setErr := c.SetInferenceRoute(ctx, openshell.InferenceRouteConfig{
		Provider: provider,
		Model:    "probe-model",
		Route:    defaultRoute,
		NoVerify: true,
	})
	switch {
	case setErr == nil:
		t.Logf("WRITE path OK on gateway %q: identity HAS the workspace admin role", gw)
	case errors.Is(setErr, openshell.ErrPermission):
		t.Fatalf("WRITE path DENIED on gateway %q: identity LACKS the workspace admin role "+
			"(reconcile-write will fail until the mTLS identity is granted admin): %v", gw, setErr)
	default:
		t.Fatalf("SetInferenceRoute returned an unexpected error: %v", setErr)
	}

	// Restore the pre-probe state.
	if existed {
		if _, err := c.SetInferenceRoute(ctx, openshell.InferenceRouteConfig{
			Provider: before.Provider, Model: before.Model, Route: defaultRoute,
			NoVerify: true, TimeoutSecs: before.TimeoutSecs,
		}); err != nil {
			t.Errorf("restoring inference route: %v", err)
		}
	} else if err := c.DeleteInferenceRoute(ctx, defaultRoute); err != nil {
		t.Errorf("cleanup DeleteInferenceRoute(%q): %v", defaultRoute, err)
	}
}
