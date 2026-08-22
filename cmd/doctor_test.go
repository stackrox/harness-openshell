package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	fake "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/fake"
	"github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/types"
	"github.com/stackrox/harness-openshell/internal/agent"
	"github.com/stackrox/harness-openshell/internal/openshell"
	"github.com/stackrox/harness-openshell/internal/testutil"
)

func TestCheckOpenShell_Found(t *testing.T) {
	cfg := testAgentConfig(t)
	results := checkOpenShell(cfg, "openshell", "")
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	r := results[0]
	if r.Group != "openshell" {
		t.Errorf("Group = %q, want openshell", r.Group)
	}
	// On machines with openshell installed, this should pass.
	// On machines without it, it should fail.
	if r.Status != "pass" && r.Status != "fail" {
		t.Errorf("Status = %q, want pass or fail", r.Status)
	}
}

func TestCheckOpenShell_NotFound(t *testing.T) {
	cfg := testAgentConfig(t)
	results := checkOpenShell(cfg, "nonexistent-binary-xyz", "")
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != "fail" {
		t.Errorf("Status = %q, want fail", results[0].Status)
	}
	if results[0].Name != "binary" {
		t.Errorf("Name = %q, want binary", results[0].Name)
	}
}

func TestCheckTargetDeps_Local(t *testing.T) {
	cfg := testAgentConfig(t)
	cfg.Gateway = "local-container"
	results := checkTargetDeps(cfg, "", "")
	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}
	if results[0].Group != "target" {
		t.Errorf("Group = %q, want target", results[0].Group)
	}
}

func TestCheckTargetDeps_Remote(t *testing.T) {
	cfg := testAgentConfig(t)
	cfg.Gateway = "openshift"
	results := checkTargetDeps(cfg, "", "")
	if len(results) < 1 {
		t.Fatal("expected at least 1 result for remote")
	}
	hasKubeconfig := false
	for _, r := range results {
		if r.Name == "kubeconfig" {
			hasKubeconfig = true
		}
	}
	if !hasKubeconfig {
		t.Error("missing kubeconfig check for remote target")
	}
}

func TestCheckTargetDeps_EmptyGateway_DefaultsToLocal(t *testing.T) {
	cfg := testAgentConfig(t)
	cfg.Gateway = ""
	results := checkTargetDeps(cfg, "", "")
	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}
	if results[0].Name != "local-container" {
		t.Errorf("Name = %q, want local-container (default)", results[0].Name)
	}
}

func TestCheckProviderEnvVars_AllSet(t *testing.T) {
	dir := t.TempDir()
	writeProviderProfile(t, dir, "github", `
id: github
credentials:
  - name: token
    env_vars: [GITHUB_TOKEN]
    required: true
`)
	t.Setenv("GITHUB_TOKEN", "test-value")

	cfg := testAgentConfig(t)
	cfg.Providers = []agent.ProviderRef{{Profile: "github"}}

	results := checkProviderEnvVars(cfg, "nonexistent-cli", dir)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != "pass" {
		t.Errorf("Status = %q, want pass", results[0].Status)
	}
}

func TestCheckProviderEnvVars_Missing(t *testing.T) {
	dir := t.TempDir()
	writeProviderProfile(t, dir, "github", `
id: github
credentials:
  - name: token
    env_vars: [GITHUB_TOKEN]
    required: true
`)
	t.Setenv("GITHUB_TOKEN", "")

	cfg := testAgentConfig(t)
	cfg.Providers = []agent.ProviderRef{{Profile: "github"}}

	results := checkProviderEnvVars(cfg, "nonexistent-cli", dir)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != "fail" {
		t.Errorf("Status = %q, want fail", results[0].Status)
	}
}

func TestCheckProviderEnvVars_NoProviders(t *testing.T) {
	cfg := testAgentConfig(t)
	cfg.Providers = nil

	results := checkProviderEnvVars(cfg, "nonexistent-cli", "")
	if len(results) != 0 {
		t.Errorf("expected 0 results for no providers, got %d", len(results))
	}
}

func TestCheckProviderEnvVars_NoProfile(t *testing.T) {
	cfg := testAgentConfig(t)
	cfg.Providers = []agent.ProviderRef{{Profile: "unknown-provider"}}

	results := checkProviderEnvVars(cfg, "nonexistent-cli", "")
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != "warn" {
		t.Errorf("Status = %q, want warn for unknown provider", results[0].Status)
	}
}

func TestCheckProviderEnvVars_OptionalCredential(t *testing.T) {
	dir := t.TempDir()
	writeProviderProfile(t, dir, "vertex", `
id: google-vertex-ai
credentials:
  - name: service_account_key
    env_vars: [GOOGLE_SERVICE_ACCOUNT_KEY]
    required: false
`)

	cfg := testAgentConfig(t)
	cfg.Providers = []agent.ProviderRef{{Profile: "vertex"}}

	results := checkProviderEnvVars(cfg, "nonexistent-cli", dir)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != "pass" {
		t.Errorf("Status = %q, want pass (all credentials optional)", results[0].Status)
	}
}

func TestDoctorOutputJSON(t *testing.T) {
	results := []CheckResult{
		{Group: "openshell", Name: "binary", Status: "pass", Message: "v0.0.63"},
		{Group: "target", Name: "local-container", Status: "pass", Message: "podman running"},
	}

	err := printStructured(formatJSON, results)
	if err != nil {
		t.Fatalf("printStructured(json): %v", err)
	}
}

func TestDoctorNoCredentialValues(t *testing.T) {
	dir := t.TempDir()
	writeProviderProfile(t, dir, "github", `
id: github
credentials:
  - name: token
    env_vars: [GITHUB_TOKEN]
    required: true
`)
	secretValue := "ghp_secret123456789"
	t.Setenv("GITHUB_TOKEN", secretValue)

	cfg := testAgentConfig(t)
	cfg.Providers = []agent.ProviderRef{{Profile: "github"}}

	results := checkProviderEnvVars(cfg, "nonexistent-cli", dir)
	for _, r := range results {
		if r.Message == secretValue || r.Name == secretValue {
			t.Errorf("credential value leaked in output: %+v", r)
		}
	}
}

func TestCheckProviderEnvVars_GatewayManagedSkipsEnvCheck(t *testing.T) {
	dir := t.TempDir()
	writeProviderProfile(t, dir, "myoauth", `
id: myoauth
credentials:
  - name: access_token
    env_vars: [MY_TOKEN]
    required: true
    refresh:
      strategy: oauth2_refresh_token
`)

	cfg := testAgentConfig(t)
	cfg.Providers = []agent.ProviderRef{{Profile: "myoauth"}}

	results := checkProviderEnvVars(cfg, "nonexistent-cli", dir)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status == "fail" {
		t.Errorf("gateway-managed credential should not fail env var check, got: %s", results[0].Message)
	}
}

// --- helpers ---

func testAgentConfig(t *testing.T) *agent.AgentConfig {
	t.Helper()
	return &agent.AgentConfig{
		Name:       "test-agent",
		Entrypoint: "claude",
	}
}

func writeProviderProfile(t *testing.T, harnessDir, name, content string) {
	t.Helper()
	dir := filepath.Join(harnessDir, "profiles", "providers")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCheckOnlineSDK_Healthy(t *testing.T) {
	c := testutil.NewFake("default", fake.WithHealthResult(&types.HealthResult{Healthy: true, Version: "1.0.0"}))
	results := checkOnlineSDK(context.Background(), c, nil)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Group != "gateway" || results[0].Name != "status" || results[0].Status != "pass" {
		t.Errorf("unexpected status result: %+v", results[0])
	}
}

func TestCheckOnlineSDK_ProviderRegistered(t *testing.T) {
	c, raw := testutil.NewFakeClient("default", fake.WithHealthResult(&types.HealthResult{Healthy: true}))
	raw.AddProvider("default", &types.Provider{Name: "github", Type: "git"})
	results := checkOnlineSDK(context.Background(), c, []string{"github"})
	// results[0] is gateway/status pass; results[1] is the provider row.
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d: %+v", len(results), results)
	}
	if results[1].Name != "github" || results[1].Status != "pass" || results[1].Message != "registered" {
		t.Errorf("expected github registered pass, got %+v", results[1])
	}
}

func TestCheckOnlineSDK_ProviderNotRegistered(t *testing.T) {
	c := testutil.NewFake("default", fake.WithHealthResult(&types.HealthResult{Healthy: true}))
	results := checkOnlineSDK(context.Background(), c, []string{"github"})
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d: %+v", len(results), results)
	}
	if results[1].Name != "github" || results[1].Status != "warn" {
		t.Errorf("expected github not-registered warn, got %+v", results[1])
	}
}

func TestRunOnlineChecks_NoGateway(t *testing.T) {
	// No --gateway: single non-fatal warn, factory never called.
	called := false
	f := func(ctx context.Context, tgt openshell.Target) (openshell.Client, error) {
		called = true
		return nil, nil
	}
	results := runOnlineChecks(context.Background(), f, "", "default", nil)
	if len(results) != 1 || results[0].Status != "warn" {
		t.Fatalf("expected 1 warn result, got %+v", results)
	}
	if called {
		t.Error("factory should not be called when --gateway is empty")
	}
}

func TestRunOnlineChecks_HealthyViaFactory(t *testing.T) {
	c := testutil.NewFake("default", fake.WithHealthResult(&types.HealthResult{Healthy: true, Version: "1.0.0"}))
	f := testutil.FakeFactory(c)
	results := runOnlineChecks(context.Background(), f, "some-gateway", "default", nil)
	if len(results) != 1 || results[0].Status != "pass" {
		t.Fatalf("expected 1 pass result, got %+v", results)
	}
}

func TestResolveOnlineFlag(t *testing.T) {
	const envKey = "OPENSHELL_TEST_RESOLVE"

	// Explicit flag wins over env var and default.
	t.Setenv(envKey, "from-env")
	if got := resolveOnlineFlag("from-flag", envKey, "from-default"); got != "from-flag" {
		t.Errorf("flag precedence: got %q, want from-flag", got)
	}

	// Empty flag falls back to the env var over the default.
	if got := resolveOnlineFlag("", envKey, "from-default"); got != "from-env" {
		t.Errorf("env precedence: got %q, want from-env", got)
	}

	// Empty flag and unset env fall back to the default.
	t.Setenv(envKey, "")
	if got := resolveOnlineFlag("", envKey, "from-default"); got != "from-default" {
		t.Errorf("default fallback: got %q, want from-default", got)
	}
}
