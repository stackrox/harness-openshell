package sdkclient

import (
	"context"
	"testing"

	fake "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/fake"
	"github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/types"

	"github.com/stackrox/harness-openshell/internal/openshell"
)

// TestGatewayInfoMergesConnectionFactsAndHealth pins invariant 37: Status and
// Version come from the SDK health RPC, while Name and Endpoint come from the
// connection facts captured at construction. A client built here with those
// fields set proves both sources merge into one record.
func TestGatewayInfoMergesConnectionFactsAndHealth(t *testing.T) {
	ctx := context.Background()
	fc := fake.NewClient(fake.WithGatewayInfo(&types.GatewayInfo{
		Status:  types.ServiceStatusHealthy,
		Version: "0.0.110",
	}))
	c := &client{raw: fc, workspace: "default", gatewayName: "prod", gatewayEndpoint: "gw.example:443"}

	got, err := c.GatewayInfo(ctx)
	if err != nil {
		t.Fatalf("GatewayInfo: %v", err)
	}
	want := openshell.GatewayInfo{Name: "prod", Endpoint: "gw.example:443", Status: "Healthy", Version: "0.0.110"}
	if got != want {
		t.Errorf("GatewayInfo: got %+v, want %+v", got, want)
	}
}

// TestGatewayInfoInjectionPathLeavesNameEndpointEmpty pins the other half of
// invariant 37: NewFromClient (the injection/test seam) loads no config, so Name
// and Endpoint are empty while Status and Version still come from the RPC. This
// is why command tests backed by the fake assert Status/Version, not Endpoint.
func TestGatewayInfoInjectionPathLeavesNameEndpointEmpty(t *testing.T) {
	ctx := context.Background()
	fc := fake.NewClient(fake.WithGatewayInfo(&types.GatewayInfo{
		Status:  types.ServiceStatusDegraded,
		Version: "0.0.110",
	}))
	c := NewFromClient(fc, "default")

	got, err := c.GatewayInfo(ctx)
	if err != nil {
		t.Fatalf("GatewayInfo: %v", err)
	}
	if got.Name != "" || got.Endpoint != "" {
		t.Errorf("injection path should leave Name/Endpoint empty, got %+v", got)
	}
	if got.Status != "Degraded" || got.Version != "0.0.110" {
		t.Errorf("Status/Version not mapped from RPC: %+v", got)
	}
}
