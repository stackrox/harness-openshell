package sdkclient

import (
	"context"

	v1 "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1"

	"github.com/stackrox/harness-openshell/internal/openshell"
)

// fromSDKGatewayInfo maps the SDK gateway health view to the harness
// GatewayInfo, merging the two sources of truth: name and endpoint are
// connection facts captured at construction (the SDK reports neither over the
// wire), while status and version come from the health RPC. Status is carried
// through as a string (Healthy|Degraded|Unhealthy|Unknown).
func fromSDKGatewayInfo(info *v1.GatewayInfo, name, endpoint string) openshell.GatewayInfo {
	return openshell.GatewayInfo{
		Name:     name,
		Endpoint: endpoint,
		Status:   string(info.Status),
		Version:  info.Version,
	}
}

// GatewayInfo introspects the active gateway. It reports the single gateway the
// client is bound to (the SDK has no gateway list), merging the health RPC's
// status/version with the name/endpoint captured at construction.
func (c *client) GatewayInfo(ctx context.Context) (openshell.GatewayInfo, error) {
	info, err := c.raw.Health().GetGatewayInfo(ctx)
	if err != nil {
		return openshell.GatewayInfo{}, translate(err)
	}
	return fromSDKGatewayInfo(info, c.gatewayName, c.gatewayEndpoint), nil
}
