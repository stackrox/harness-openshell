// Package sdkclient is the ONLY production package permitted to import the
// OpenShell Go SDK. It translates between the harness-owned internal/openshell
// vocabulary and the SDK, keeping every SDK type behind the firewall.
//
// Construction routes through planConnection (auth.go), the pure resolver that
// decides the dial branch and derives mTLS cert paths. Only the mTLS branch is
// wired to a live dial today; the remaining branches, provider mapping, and
// error translation land in S3.
package sdkclient

import (
	"context"
	"fmt"
	"os"

	v1 "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1"
	"github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/gateway"

	"github.com/stackrox/harness-openshell/internal/openshell"
)

// defaultWorkspace is the workspace assumed when a Target leaves it empty. This
// package is the single owner of that default.
const defaultWorkspace = "default"

// Compile-time guarantees that New satisfies the Factory seam and *client
// satisfies the Client interface.
var (
	_ openshell.Factory = New
	_ openshell.Client  = (*client)(nil)
)

// client wraps the SDK client interface, binding it to one workspace. It holds
// the interface (not *v1.Client) so tests can inject the SDK fake.
type client struct {
	raw       v1.ClientInterface
	workspace string
}

// New constructs an openshell.Client for the given target.
//
// It loads the CLI-managed gateway config and delegates the dial decision to
// planConnection. Only the mTLS branch is dialed today (the auth mode all our
// managed gateways use); the other branches return ErrConfig until S3 wires
// their dial paths.
func New(ctx context.Context, t openshell.Target) (openshell.Client, error) {
	cfg, err := gateway.LoadConfig(t.Gateway)
	if err != nil {
		return nil, fmt.Errorf("%w: load gateway %q: %v", openshell.ErrConfig, t.Gateway, err)
	}

	ws := t.Workspace
	if ws == "" {
		ws = defaultWorkspace
	}

	plan, err := planConnection(cfg, os.Getenv)
	if err != nil {
		return nil, err
	}

	// S2 wires only the mTLS branch (the auth mode all our managed gateways
	// use). The remaining branches gain their dial paths in S3.
	if plan.branch != branchMTLS {
		return nil, fmt.Errorf("%w: auth mode %q not yet supported (mtls only until S3)", openshell.ErrConfig, plan.mode)
	}

	raw, err := gateway.NewClient(plan.name, gateway.WithAuth(v1.NoAuth()), gateway.WithTLS(plan.tls))
	if err != nil {
		return nil, fmt.Errorf("%w: dial gateway %q: %v", openshell.ErrConfig, plan.name, err)
	}

	return &client{raw: raw, workspace: ws}, nil
}

// Health reports gateway health. Error translation to openshell sentinels lands
// in S3; S1 surfaces the raw SDK error.
func (c *client) Health(ctx context.Context) (openshell.Health, error) {
	h, err := c.raw.Health().Check(ctx)
	if err != nil {
		return openshell.Health{}, err
	}
	return openshell.Health{Healthy: h.Healthy, Version: h.Version}, nil
}

// Providers is not implemented until S3.
func (c *client) Providers(ctx context.Context) ([]openshell.Provider, error) {
	return nil, fmt.Errorf("providers: not implemented until slice S3")
}

// Close releases the underlying SDK client.
func (c *client) Close() error {
	return c.raw.Close()
}
