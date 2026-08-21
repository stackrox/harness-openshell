// Package sdkclient is the ONLY production package permitted to import the
// OpenShell Go SDK. It translates between the harness-owned internal/openshell
// vocabulary and the SDK, keeping every SDK type behind the firewall.
//
// S3 has landed: translate (errors.go) and provider mapping (provider.go) are
// in place, and dial covers all branches (mTLS verified against a live gateway;
// default and SA-OIDC selected and compiled but unverified).
package sdkclient

import (
	"context"
	"fmt"
	"os"

	v1 "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1"
	gateway "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/gateway"
	oidc "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/oidc"
	"golang.org/x/oauth2"

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

// New constructs an openshell.Client for the given target: it loads the
// CLI-managed gateway config, resolves the dial plan via planConnection, and
// executes it via dial. Only the mTLS branch is verified against a live
// gateway; the default and SA-OIDC branches compile and are selected but the
// SA-OIDC path is UNVERIFIED (no OIDC gateway available).
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

	raw, err := dial(ctx, plan)
	if err != nil {
		return nil, err
	}

	return NewFromClient(raw, ws), nil
}

// NewFromClient wraps an existing SDK client (or the SDK fake) bound to a
// workspace. It is the injection seam used by white-box tests and, in S4, by
// internal/testutil. Empty workspace defaults to defaultWorkspace.
func NewFromClient(raw v1.ClientInterface, workspace string) openshell.Client {
	if workspace == "" {
		workspace = defaultWorkspace
	}
	return &client{raw: raw, workspace: workspace}
}

// dial executes a connPlan, returning a live SDK client. mTLS is the only
// branch verified against a real gateway.
func dial(ctx context.Context, p connPlan) (v1.ClientInterface, error) {
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
	case branchSAOIDC:
		return dialSAOIDC(ctx, p)
	default:
		return nil, fmt.Errorf("%w: unsupported connection branch", openshell.ErrConfig)
	}
}

// dialSAOIDC builds a service-account (client-credentials) OIDC client.
//
// UNVERIFIED (no OIDC gateway; see specs findings): this path is exercised only
// for branch selection and compilation. TODO(PR8): audience/scopes unverified —
// needs an OIDC gateway. On any failure it returns a wrapped sentinel, never
// panics, and never places the client secret into an error message.
func dialSAOIDC(ctx context.Context, p connPlan) (v1.ClientInterface, error) {
	secret := os.Getenv("OPENSHELL_OIDC_CLIENT_SECRET")
	tok, err := oidc.ClientCredentials(ctx,
		oidc.WithIssuer(p.oidcIssuer),
		oidc.WithClientID(p.oidcClientID),
		oidc.WithClientSecret(secret),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: oidc client-credentials login for %q", openshell.ErrUnauthenticated, p.name)
	}
	provider, err := v1.RefreshableToken(oauth2.StaticTokenSource(tok))
	if err != nil {
		return nil, fmt.Errorf("%w: build refreshable token: %v", openshell.ErrConfig, err)
	}
	raw, err := gateway.NewClient(p.name, gateway.WithAuth(provider))
	if err != nil {
		return nil, fmt.Errorf("%w: dial gateway %q: %v", openshell.ErrConfig, p.name, err)
	}
	return raw, nil
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
