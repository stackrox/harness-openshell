package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/robbycochran/harness-openshell/internal/agent"
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
	cfg.Gateway = "local"
	results := checkTargetDeps(cfg, "", "")
	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}
	if results[0].Group != "target" {
		t.Errorf("Group = %q, want target", results[0].Group)
	}
}

func TestCheckTargetDeps_Kind(t *testing.T) {
	cfg := testAgentConfig(t)
	cfg.Gateway = "kind"
	results := checkTargetDeps(cfg, "", "")
	if len(results) < 2 {
		t.Fatalf("expected at least 2 results for kind, got %d", len(results))
	}
	names := make(map[string]bool)
	for _, r := range results {
		names[r.Name] = true
	}
	if !names["kubectl"] {
		t.Error("missing kubectl check for kind target")
	}
	if !names["kind"] {
		t.Error("missing kind binary check for kind target")
	}
}

func TestCheckTargetDeps_Remote(t *testing.T) {
	cfg := testAgentConfig(t)
	cfg.Gateway = "ocp"
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
	if results[0].Name != "local" {
		t.Errorf("Name = %q, want local (default)", results[0].Name)
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
		{Group: "target", Name: "local", Status: "pass", Message: "podman running"},
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
