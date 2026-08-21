package sdkclient

import (
	"fmt"
	"path/filepath"

	gw "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/gateway"
	"github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/types"

	"github.com/stackrox/harness-openshell/internal/openshell"
)

// EnvLookup is an injectable environment variable lookup function.
// Production code passes os.Getenv; tests inject a closure.
type EnvLookup func(string) string

// connBranch represents the auth mode branch decision for connection setup.
type connBranch int

const (
	branchDefault connBranch = iota // none/plaintext/cloudflare_jwt/oidc-human → gateway.NewClient(name)
	branchMTLS                      // WithAuth(NoAuth()) + WithTLS(certs derived from cfg.Dir)
	branchSAOIDC                    // oidc + OPENSHELL_OIDC_CLIENT_SECRET present
)

// connPlan holds the connection setup parameters resolved from auth mode and environment.
type connPlan struct {
	name    string
	address string
	mode    gw.AuthMode
	branch  connBranch
	tls     *types.TLSConfig // set ONLY for branchMTLS; nil otherwise
}

// planConnection determines which auth branch and TLS configuration to use.
// It is pure: no disk access, no network I/O, and no secret material in its
// output or error messages. Environment lookups are injected for testability.
func planConnection(cfg *gw.Config, env EnvLookup) (connPlan, error) {
	plan := connPlan{
		name:    cfg.Name,
		address: cfg.Endpoint,
		mode:    cfg.AuthMode,
	}

	switch cfg.AuthMode {
	case gw.AuthModeMTLS:
		plan.branch = branchMTLS
		mtlsDir := filepath.Join(cfg.Dir, "mtls")
		plan.tls = &types.TLSConfig{
			CertFile: filepath.Join(mtlsDir, "tls.crt"),
			KeyFile:  filepath.Join(mtlsDir, "tls.key"),
			CAFile:   filepath.Join(mtlsDir, "ca.crt"),
		}

	case gw.AuthModeNone, gw.AuthModePlaintext, gw.AuthModeCloudflareJWT:
		plan.branch = branchDefault
		plan.tls = nil

	case gw.AuthModeOIDC:
		secret := env("OPENSHELL_OIDC_CLIENT_SECRET")
		if secret == "" {
			plan.branch = branchDefault
			plan.tls = nil
		} else {
			// TODO(PR8): audience/scopes unverified — needs OIDC gateway
			plan.branch = branchSAOIDC
			plan.tls = nil
		}

	default:
		return connPlan{}, fmt.Errorf("%w: unsupported auth mode %q", openshell.ErrConfig, cfg.AuthMode)
	}

	return plan, nil
}
