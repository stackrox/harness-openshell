package sdkclient_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stackrox/harness-openshell/internal/openshell"
	"github.com/stackrox/harness-openshell/internal/openshell/sdkclient"
)

// TestHealthE2E proves an mTLS Health().Check succeeds from within the harness
// module against a real gateway.
//
// It is skipped unless HARNESS_E2E_GATEWAY names a registered openshell gateway
// (the local mTLS dev gateway is "openshell"). Optional HARNESS_E2E_WORKSPACE
// overrides the workspace.
//
//	HARNESS_E2E_GATEWAY=openshell go test ./internal/openshell/sdkclient/ -run HealthE2E -v
func TestHealthE2E(t *testing.T) {
	gw := os.Getenv("HARNESS_E2E_GATEWAY")
	if gw == "" {
		t.Skip("set HARNESS_E2E_GATEWAY to a registered mTLS gateway to run this end-to-end check")
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

	h, err := c.Health(ctx)
	if err != nil {
		t.Fatalf("Health().Check: %v", err)
	}
	if !h.Healthy {
		t.Fatalf("gateway reported unhealthy: %+v", h)
	}
	t.Logf("gateway %q healthy: version=%s", gw, h.Version)
}
