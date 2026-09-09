package sdkclient

import (
	"context"

	v1 "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1"

	"github.com/stackrox/harness-openshell/internal/openshell"
)

// fromSDKProvider exposes provider identity only. Credentials and provider
// configuration remain gateway-owned and never cross the harness boundary.
func fromSDKProvider(p *v1.Provider) openshell.Provider {
	return openshell.Provider{Name: p.Name, Type: p.Type}
}

// GetProvider reads the named provider in the bound workspace.
func (c *client) GetProvider(ctx context.Context, name string) (openshell.Provider, error) {
	p, err := c.raw.Providers().Get(ctx, c.workspace, name)
	if err != nil {
		return openshell.Provider{}, translate(err)
	}
	return fromSDKProvider(p), nil
}
