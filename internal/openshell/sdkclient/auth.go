package sdkclient

import (
	"fmt"
	"path/filepath"

	gw "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/gateway"
	"github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/types"

	"github.com/stackrox/harness-openshell/internal/openshell"
)

// connBranch represents the auth mode branch decision for connection setup.
type connBranch int

const (
	branchDefault connBranch = iota // none/plaintext/token-backed auth → gateway.NewClient(name)
	branchMTLS                      // WithAuth(NoAuth()) + WithTLS(certs derived from cfg.Dir)
)

// connPlan holds the connection setup parameters resolved from auth mode.
type connPlan struct {
	name   string
	branch connBranch
	tls    *types.TLSConfig // set ONLY for branchMTLS; nil otherwise
}

// planConnection determines which auth branch and TLS configuration to use.
// It is pure: no disk access, network I/O, or secret material.
func planConnection(cfg *gw.Config) (connPlan, error) {
	plan := connPlan{name: cfg.Name}

	switch cfg.AuthMode {
	case gw.AuthModeMTLS:
		plan.branch = branchMTLS
		mtlsDir := filepath.Join(cfg.Dir, "mtls")
		plan.tls = &types.TLSConfig{
			CertFile: filepath.Join(mtlsDir, "tls.crt"),
			KeyFile:  filepath.Join(mtlsDir, "tls.key"),
			CAFile:   filepath.Join(mtlsDir, "ca.crt"),
		}

	case gw.AuthModeNone, gw.AuthModePlaintext, gw.AuthModeCloudflareJWT, gw.AuthModeOIDC:
		plan.branch = branchDefault
		plan.tls = nil

	default:
		return connPlan{}, fmt.Errorf("%w: unsupported auth mode %q", openshell.ErrConfig, cfg.AuthMode)
	}

	return plan, nil
}
