// Package sdkclient is the ONLY production package permitted to import the
// OpenShell Go SDK. It translates between the harness-owned internal/openshell
// vocabulary and the SDK, keeping every SDK type behind the firewall.
//
// Slice S1 scope: construct a client for an mTLS gateway (the auth mode all our
// managed gateways use) and report Health. The full auth-mode resolver
// (planConnection) lands in S2; provider mapping and error translation in S3.
package sdkclient

import (
	"context"
	"fmt"
	"path/filepath"

	v1 "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1"
	"github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/gateway"
	"github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/types"

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
// S1 handles only mTLS gateways: it loads the CLI-managed gateway config, points
// TLS at the CLI-managed client certificate under <cfg.Dir>/mtls, and dials via
// the gateway.NewClient escape hatch (an explicit WithAuth skips the SDK's
// "mtls not supported" resolver; WithTLS supplies the client cert). Other auth
// modes return ErrConfig until S2 generalizes construction.
func New(ctx context.Context, t openshell.Target) (openshell.Client, error) {
	cfg, err := gateway.LoadConfig(t.Gateway)
	if err != nil {
		return nil, fmt.Errorf("%w: load gateway %q: %v", openshell.ErrConfig, t.Gateway, err)
	}

	ws := t.Workspace
	if ws == "" {
		ws = defaultWorkspace
	}

	if cfg.AuthMode != gateway.AuthModeMTLS {
		return nil, fmt.Errorf("%w: auth mode %q not yet supported (S1 handles mtls only)", openshell.ErrConfig, cfg.AuthMode)
	}

	mtlsDir := filepath.Join(cfg.Dir, "mtls")
	tls := &types.TLSConfig{
		CertFile: filepath.Join(mtlsDir, "tls.crt"),
		KeyFile:  filepath.Join(mtlsDir, "tls.key"),
		CAFile:   filepath.Join(mtlsDir, "ca.crt"),
	}

	raw, err := gateway.NewClient(t.Gateway, gateway.WithAuth(v1.NoAuth()), gateway.WithTLS(tls))
	if err != nil {
		return nil, fmt.Errorf("%w: dial gateway %q: %v", openshell.ErrConfig, t.Gateway, err)
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
