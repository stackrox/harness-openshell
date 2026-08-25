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

// TestLiveInferenceRoleProbe probes whether the harness mTLS identity holds the
// workspace "admin" role required to write inference routes. This is the S1 risk
// gate for PR4b: SetInferenceRoute/DeleteInferenceRoute require admin, while
// GetInferenceRoute only needs the user role. Slice 3's reconcile-write cannot
// succeed on a real gateway if the identity lacks admin.
//
// It is skipped unless HARNESS_E2E_GATEWAY names a registered mTLS gateway (the
// same gate as the other live checks). Optional HARNESS_E2E_WORKSPACE overrides
// the workspace. The probe uses a SCRATCH route name and cleans it up; it never
// touches the default route.
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

	const scratch = "harness-probe"

	// Read path requires only the user role; ErrNotFound is a success signal
	// (the identity can read; the scratch route just doesn't exist yet).
	if _, err := c.GetInferenceRoute(ctx, scratch); err != nil && !errors.Is(err, openshell.ErrNotFound) {
		t.Fatalf("GetInferenceRoute (user role) failed unexpectedly: %v", err)
	}
	t.Logf("read path OK on gateway %q (user role confirmed)", gw)

	// Write path requires the admin role. Either outcome is a recordable probe
	// result; ErrPermission is exactly the risk we are measuring, not a bug.
	_, setErr := c.SetInferenceRoute(ctx, openshell.InferenceRouteConfig{
		Provider: "gcp",
		Model:    "claude-opus-4-8",
		Route:    scratch,
		NoVerify: true,
	})
	switch {
	case setErr == nil:
		t.Logf("WRITE path OK on gateway %q: identity HAS the workspace admin role", gw)
		if delErr := c.DeleteInferenceRoute(ctx, scratch); delErr != nil {
			t.Errorf("cleanup DeleteInferenceRoute(%q): %v", scratch, delErr)
		}
	case errors.Is(setErr, openshell.ErrPermission):
		t.Fatalf("WRITE path DENIED on gateway %q: identity LACKS the workspace admin role "+
			"(PR4b Slice 3 reconcile-write will fail until the mTLS identity is granted admin): %v", gw, setErr)
	default:
		t.Fatalf("SetInferenceRoute returned an unexpected error: %v", setErr)
	}
}
