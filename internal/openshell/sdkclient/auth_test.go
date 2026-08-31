package sdkclient

import (
	"errors"
	"path/filepath"
	"testing"

	gw "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/gateway"

	"github.com/stackrox/harness-openshell/internal/openshell"
)

func TestPlanConnection(t *testing.T) {
	tests := []struct {
		name         string
		cfg          *gw.Config
		expectBranch connBranch
		expectTLS    bool
		expectError  bool
	}{
		{
			name: "mtls auth mode",
			cfg: &gw.Config{
				Name:     "test-gateway",
				AuthMode: gw.AuthModeMTLS,
				Dir:      "/fake/gwdir",
			},
			expectBranch: branchMTLS,
			expectTLS:    true,
		},
		{
			name:         "none auth mode",
			cfg:          &gw.Config{Name: "test-gateway", AuthMode: gw.AuthModeNone},
			expectBranch: branchDefault,
		},
		{
			name:         "plaintext auth mode",
			cfg:          &gw.Config{Name: "test-gateway", AuthMode: gw.AuthModePlaintext},
			expectBranch: branchDefault,
		},
		{
			name:         "cloudflare auth mode",
			cfg:          &gw.Config{Name: "test-gateway", AuthMode: gw.AuthModeCloudflareJWT},
			expectBranch: branchDefault,
		},
		{
			name:         "oidc auth mode uses persisted CLI token",
			cfg:          &gw.Config{Name: "test-gateway", AuthMode: gw.AuthModeOIDC},
			expectBranch: branchDefault,
		},
		{
			name:        "unsupported auth mode",
			cfg:         &gw.Config{Name: "test-gateway", AuthMode: gw.AuthMode("bogus")},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := planConnection(tt.cfg)
			if tt.expectError {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !errors.Is(err, openshell.ErrConfig) {
					t.Fatalf("expected ErrConfig, got %v", err)
				}
				if plan != (connPlan{}) {
					t.Fatalf("expected zero connPlan, got %+v", plan)
				}
				return
			}

			if err != nil {
				t.Fatalf("plan connection: %v", err)
			}
			if plan.name != tt.cfg.Name {
				t.Errorf("name = %q, want %q", plan.name, tt.cfg.Name)
			}
			if plan.branch != tt.expectBranch {
				t.Errorf("branch = %v, want %v", plan.branch, tt.expectBranch)
			}
			if tt.expectTLS {
				if plan.tls == nil {
					t.Fatal("expected TLS config")
				}
				want := filepath.Join(tt.cfg.Dir, "mtls", "tls.crt")
				if plan.tls.CertFile != want {
					t.Errorf("cert file = %q, want %q", plan.tls.CertFile, want)
				}
			} else if plan.tls != nil {
				t.Errorf("expected nil TLS config, got %+v", plan.tls)
			}
		})
	}
}
