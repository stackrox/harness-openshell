// Package sdkclient is the ONLY production package permitted to import the
// OpenShell Go SDK. It translates between the harness-owned internal/openshell
// vocabulary and the SDK, keeping every SDK type behind the firewall.
//
// It dials via mTLS, an unauthenticated default, and service-account OIDC. The
// mTLS path is verified against a live gateway; the SA-OIDC path is untested
// end-to-end (no OIDC gateway is available in this environment).
package sdkclient

import (
	"context"
	"fmt"
	"os"
	"time"

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
// executes it via dial. Only the mTLS branch is verified against a live
// gateway; the SA-OIDC branch is untested end-to-end (no OIDC gateway
// available).
func New(ctx context.Context, t openshell.Target) (openshell.Client, error) {
	cfg, err := gateway.LoadConfig(t.Gateway)
	if err != nil {
		return nil, fmt.Errorf("%w: load gateway %q: %v", openshell.ErrConfig, t.Gateway, err)
	}

	plan, err := planConnection(cfg, os.Getenv)
	if err != nil {
		return nil, err
	}

	raw, err := dial(ctx, plan)
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

// oidcGrantTimeout bounds each client-credentials grant so a stalled OIDC token
// endpoint cannot block a dial or a background refresh indefinitely.
const oidcGrantTimeout = 30 * time.Second

// tokenSourceFunc adapts a plain function to oauth2.TokenSource so a fresh
// client-credentials grant can be run on demand.
type tokenSourceFunc func() (*oauth2.Token, error)

func (f tokenSourceFunc) Token() (*oauth2.Token, error) { return f() }

// clientCredentials runs the OIDC client-credentials grant for plan p, reading
// the secret fresh from the environment each call (so rotation is picked up and
// the secret is never stored on the plan). oidc.ClientCredentials guarantees the
// secret never appears in its error (SDK FR-014).
func clientCredentials(ctx context.Context, p connPlan) (*oauth2.Token, error) {
	return oidc.ClientCredentials(ctx,
		oidc.WithIssuer(p.oidcIssuer),
		oidc.WithClientID(p.oidcClientID),
		oidc.WithClientSecret(os.Getenv("OPENSHELL_OIDC_CLIENT_SECRET")),
	)
}

// dialSAOIDC builds a service-account (client-credentials) OIDC client.
//
// The token source genuinely refreshes: an eager grant validates the credentials
// up front (fast-fail as ErrUnauthenticated), and oauth2.ReuseTokenSource serves
// that token until it expires, then re-runs the grant. This is deliberately not
// oauth2.StaticTokenSource, which pins a single token and would break auth the
// moment it expires (v1.RefreshableToken re-calls its source on expiry).
//
// Refreshes must outlive this dial call, so the source detaches from the
// caller's cancellation with context.WithoutCancel. Because that also strips the
// deadline, every grant — eager and refresh — gets its own bounded
// oidcGrantTimeout so a stalled token endpoint can never block indefinitely.
//
// Untested end-to-end: no OIDC gateway is available in this environment, so
// this path is exercised only for branch selection and compilation, and its
// audience/scopes are unverified. On any failure it returns a wrapped sentinel,
// never panics, and never places the client secret into an error message.
func dialSAOIDC(ctx context.Context, p connPlan) (v1.ClientInterface, error) {
	eagerCtx, cancel := context.WithTimeout(ctx, oidcGrantTimeout)
	defer cancel()
	tok, err := clientCredentials(eagerCtx, p)
	if err != nil {
		return nil, fmt.Errorf("%w: oidc client-credentials login for %q", openshell.ErrUnauthenticated, p.name)
	}

	refreshBase := context.WithoutCancel(ctx)
	source := oauth2.ReuseTokenSource(tok, tokenSourceFunc(func() (*oauth2.Token, error) {
		grantCtx, cancel := context.WithTimeout(refreshBase, oidcGrantTimeout)
		defer cancel()
		return clientCredentials(grantCtx, p)
	}))

	provider, err := v1.RefreshableToken(source)
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
