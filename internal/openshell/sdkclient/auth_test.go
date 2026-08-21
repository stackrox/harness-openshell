package sdkclient

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	gw "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/gateway"

	"github.com/stackrox/harness-openshell/internal/openshell"
)

func TestPlanConnection(t *testing.T) {
	tests := []struct {
		name         string
		cfg          *gw.Config
		env          EnvLookup
		expectBranch connBranch
		expectMode   gw.AuthMode
		expectTLS    bool // true if plan.tls is non-nil, false if nil
		expectError  bool
	}{
		{
			name: "mtls auth mode",
			cfg: &gw.Config{
				Name:     "test-gateway",
				Endpoint: "localhost:9876",
				AuthMode: gw.AuthModeMTLS,
				Dir:      "/fake/gwdir",
			},
			env:          func(string) string { return "" },
			expectBranch: branchMTLS,
			expectMode:   gw.AuthModeMTLS,
			expectTLS:    true,
			expectError:  false,
		},
		{
			name: "none auth mode",
			cfg: &gw.Config{
				Name:     "test-gateway",
				Endpoint: "localhost:9876",
				AuthMode: gw.AuthModeNone,
				Dir:      "/fake/gwdir",
			},
			env:          func(string) string { return "" },
			expectBranch: branchDefault,
			expectMode:   gw.AuthModeNone,
			expectTLS:    false,
			expectError:  false,
		},
		{
			name: "plaintext auth mode",
			cfg: &gw.Config{
				Name:     "test-gateway",
				Endpoint: "localhost:9876",
				AuthMode: gw.AuthModePlaintext,
				Dir:      "/fake/gwdir",
			},
			env:          func(string) string { return "" },
			expectBranch: branchDefault,
			expectMode:   gw.AuthModePlaintext,
			expectTLS:    false,
			expectError:  false,
		},
		{
			name: "cloudflare_jwt auth mode",
			cfg: &gw.Config{
				Name:     "test-gateway",
				Endpoint: "localhost:9876",
				AuthMode: gw.AuthModeCloudflareJWT,
				Dir:      "/fake/gwdir",
			},
			env:          func(string) string { return "" },
			expectBranch: branchDefault,
			expectMode:   gw.AuthModeCloudflareJWT,
			expectTLS:    false,
			expectError:  false,
		},
		{
			name: "oidc auth mode without secret",
			cfg: &gw.Config{
				Name:     "test-gateway",
				Endpoint: "localhost:9876",
				AuthMode: gw.AuthModeOIDC,
				Dir:      "/fake/gwdir",
			},
			env:          func(string) string { return "" },
			expectBranch: branchDefault,
			expectMode:   gw.AuthModeOIDC,
			expectTLS:    false,
			expectError:  false,
		},
		{
			name: "oidc auth mode with secret",
			cfg: &gw.Config{
				Name:     "test-gateway",
				Endpoint: "localhost:9876",
				AuthMode: gw.AuthModeOIDC,
				Dir:      "/fake/gwdir",
			},
			env: func(key string) string {
				if key == "OPENSHELL_OIDC_CLIENT_SECRET" {
					return "SUPER-SECRET-abc123"
				}
				return ""
			},
			expectBranch: branchSAOIDC,
			expectMode:   gw.AuthModeOIDC,
			expectTLS:    false,
			expectError:  false,
		},
		{
			name: "unsupported auth mode",
			cfg: &gw.Config{
				Name:     "test-gateway",
				Endpoint: "localhost:9876",
				AuthMode: gw.AuthMode("bogus"),
				Dir:      "/fake/gwdir",
			},
			env:         func(string) string { return "" },
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := planConnection(tt.cfg, tt.env)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				if !errors.Is(err, openshell.ErrConfig) {
					t.Errorf("expected error to wrap openshell.ErrConfig, got: %v", err)
				}
				if plan != (connPlan{}) {
					t.Errorf("expected zero connPlan on error, got: %+v", plan)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			// Verify name, address, and mode are always set.
			if plan.name != tt.cfg.Name {
				t.Errorf("expected name %q, got %q", tt.cfg.Name, plan.name)
			}
			if plan.address != tt.cfg.Endpoint {
				t.Errorf("expected address %q, got %q", tt.cfg.Endpoint, plan.address)
			}
			if plan.mode != tt.expectMode {
				t.Errorf("expected mode %q, got %q", tt.expectMode, plan.mode)
			}

			// Verify branch.
			if plan.branch != tt.expectBranch {
				t.Errorf("expected branch %v, got %v", tt.expectBranch, plan.branch)
			}

			// Verify TLS config.
			if tt.expectTLS {
				if plan.tls == nil {
					t.Errorf("expected non-nil TLS config, got nil")
				} else {
					expectedCertFile := filepath.Join(tt.cfg.Dir, "mtls", "tls.crt")
					if plan.tls.CertFile != expectedCertFile {
						t.Errorf("expected CertFile %q, got %q", expectedCertFile, plan.tls.CertFile)
					}
					expectedKeyFile := filepath.Join(tt.cfg.Dir, "mtls", "tls.key")
					if plan.tls.KeyFile != expectedKeyFile {
						t.Errorf("expected KeyFile %q, got %q", expectedKeyFile, plan.tls.KeyFile)
					}
					expectedCAFile := filepath.Join(tt.cfg.Dir, "mtls", "ca.crt")
					if plan.tls.CAFile != expectedCAFile {
						t.Errorf("expected CAFile %q, got %q", expectedCAFile, plan.tls.CAFile)
					}
				}
			} else {
				if plan.tls != nil {
					t.Errorf("expected nil TLS config, got: %+v", plan.tls)
				}
			}
		})
	}
}

func TestPlanConnectionSecretNonLeak(t *testing.T) {
	// Verify that the secret value does not appear in the plan output.
	secret := "SUPER-SECRET-abc123"
	cfg := &gw.Config{
		Name:     "test-gateway",
		Endpoint: "localhost:9876",
		AuthMode: gw.AuthModeOIDC,
		Dir:      "/fake/gwdir",
	}
	env := func(key string) string {
		if key == "OPENSHELL_OIDC_CLIENT_SECRET" {
			return secret
		}
		return ""
	}

	plan, err := planConnection(cfg, env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The key assertion: the secret must NOT appear in the plan's stringified
	// form. planConnection never stores it, so branch selection is the only
	// observable effect of the secret's presence.
	planStr := fmt.Sprintf("%+v", plan)
	for _, substr := range []string{secret, "SUPER-SECRET", "abc123"} {
		if strings.Contains(planStr, substr) {
			t.Errorf("secret material %q leaked into plan string %q", substr, planStr)
		}
	}

	if plan.branch != branchSAOIDC {
		t.Errorf("expected branchSAOIDC, got %v", plan.branch)
	}
}
