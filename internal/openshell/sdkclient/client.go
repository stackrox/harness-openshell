// Package sdkclient is the ONLY production package permitted to import the
// OpenShell Go SDK. It translates between the harness-owned internal/openshell
// vocabulary and the SDK, keeping every SDK type behind the firewall.
//
// It dials either from CLI-compatible gateway state or directly from canonical
// workflow connection metadata using an audience-aware OIDC token source.
package sdkclient

import (
	"context"
	"fmt"

	v1 "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1"
	gateway "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/gateway"

	"github.com/stackrox/harness-openshell/internal/openshell"
)

// defaultWorkspace is the workspace assumed when a Target leaves it empty. This
// package is the single owner of that default.
const defaultWorkspace = "default"

// Compile-time guarantees that New satisfies the Factory seam and *client
// satisfies the Client interface.
var (
	_ openshell.Factory                = New
	_ openshell.Client                 = (*client)(nil)
	_ openshell.SandboxExecutionClient = (*client)(nil)
)

// client wraps the SDK client interface, binding it to one workspace. It holds
// the interface (not *v1.Client) so tests can inject the SDK fake.
//
// gatewayName and gatewayEndpoint are connection facts the SDK does not report
// over the wire (GatewayInfo carries neither): New captures them from the
// CLI-managed gateway config so GatewayInfo can merge them with the health RPC's
// status/version. The NewFromClient injection path leaves both empty.
type client struct {
	raw             v1.ClientInterface
	workspace       string
	gatewayName     string
	gatewayEndpoint string
}

// New constructs an openshell.Client for the given target: it loads the
// CLI-managed gateway config, resolves the dial plan via planConnection, and
// executes it via dial. For OIDC gateways, authenticate with the OpenShell CLI
// first so its audience-aware token is available to the SDK.
func New(ctx context.Context, t openshell.Target) (openshell.Client, error) {
	if t.Direct != nil {
		return newDirect(ctx, t)
	}
	cfg, err := gateway.LoadConfig(t.Gateway)
	if err != nil {
		return nil, fmt.Errorf("%w: load gateway %q: %v", openshell.ErrConfig, t.Gateway, err)
	}

	plan, err := planConnection(cfg)
	if err != nil {
		return nil, err
	}

	raw, err := dial(plan)
	if err != nil {
		return nil, err
	}

	// newClient is the single owner of the "" -> defaultWorkspace default; pass
	// t.Workspace straight through. Capture the connection facts the SDK never
	// reports (gateway name and endpoint) so GatewayInfo can merge them.
	c := newClient(raw, t.Workspace)
	c.gatewayName = t.Gateway
	c.gatewayEndpoint = cfg.Endpoint
	return c, nil
}

// NewFromClient wraps an existing SDK client (or the SDK fake) bound to a
// workspace. It is the injection seam used by white-box tests and by
// internal/testutil. Empty workspace defaults to defaultWorkspace. It leaves the
// gateway name and endpoint empty — those are connection facts only New can
// capture from the CLI-managed config.
func NewFromClient(raw v1.ClientInterface, workspace string) openshell.Client {
	return newClient(raw, workspace)
}

// newClient builds the concrete client with the workspace default applied. It
// returns the concrete type so New can set the connection facts before returning
// the interface.
func newClient(raw v1.ClientInterface, workspace string) *client {
	if workspace == "" {
		workspace = defaultWorkspace
	}
	return &client{raw: raw, workspace: workspace}
}

// dial executes a connPlan, returning a live SDK client. mTLS is the only
// branch verified against a real gateway.
func dial(p connPlan) (v1.ClientInterface, error) {
	switch p.branch {
	case branchMTLS:
		raw, err := gateway.NewClient(p.name, gateway.WithAuth(v1.NoAuth()), gateway.WithTLS(p.tls))
		if err != nil {
			return nil, fmt.Errorf("%w: dial gateway %q: %v", openshell.ErrConfig, p.name, err)
		}
		return raw, nil
	case branchDefault:
		raw, err := gateway.NewClient(p.name)
		if err != nil {
			return nil, fmt.Errorf("%w: dial gateway %q: %v", openshell.ErrConfig, p.name, err)
		}
		return raw, nil
	default:
		return nil, fmt.Errorf("%w: unsupported connection branch", openshell.ErrConfig)
	}
}

// Health reports gateway health.
func (c *client) Health(ctx context.Context) (openshell.Health, error) {
	h, err := c.raw.Health().Check(ctx)
	if err != nil {
		return openshell.Health{}, translate(err)
	}
	return openshell.Health{Healthy: h.Healthy, Version: h.Version}, nil
}

// Providers lists the providers registered in the bound workspace.
func (c *client) Providers(ctx context.Context) ([]openshell.Provider, error) {
	raw, err := c.raw.Providers().List(ctx, c.workspace)
	if err != nil {
		return nil, translate(err)
	}
	out := make([]openshell.Provider, 0, len(raw))
	for _, p := range raw {
		out = append(out, fromSDKProvider(p))
	}
	return out, nil
}

// Close releases the underlying SDK client.
func (c *client) Close() error {
	return c.raw.Close()
}
