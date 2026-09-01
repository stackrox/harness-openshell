package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	fake "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/fake"
	"github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/types"
	"github.com/stackrox/harness-openshell/internal/config"
	"github.com/stackrox/harness-openshell/internal/openshell"
	"github.com/stackrox/harness-openshell/internal/testutil"
)

func TestCheckOpenShell_Found(t *testing.T) {
	cfg := testHarnessConfig(t)
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
	cfg := testHarnessConfig(t)
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

	cfg := testHarnessConfig(t)
	cfg.Spec.Providers = []config.Provider{{Name: "github", Management: "referenced"}}

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

	cfg := testHarnessConfig(t)
	cfg.Spec.Providers = []config.Provider{{Name: "github", Management: "referenced"}}

	results := checkProviderEnvVars(cfg, "nonexistent-cli", dir)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != "fail" {
		t.Errorf("Status = %q, want fail", results[0].Status)
	}
}

func TestCheckProviderEnvVars_NoProviders(t *testing.T) {
	cfg := testHarnessConfig(t)
	cfg.Spec.Providers = nil

	results := checkProviderEnvVars(cfg, "nonexistent-cli", "")
	if len(results) != 0 {
		t.Errorf("expected 0 results for no providers, got %d", len(results))
	}
}

func TestCheckProviderEnvVars_NoProfile(t *testing.T) {
	cfg := testHarnessConfig(t)
	cfg.Spec.Providers = []config.Provider{{Name: "unknown-provider", Management: "referenced"}}

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

	cfg := testHarnessConfig(t)
	cfg.Spec.Providers = []config.Provider{{Name: "vertex", Management: "referenced"}}

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

	cfg := testHarnessConfig(t)
	cfg.Spec.Providers = []config.Provider{{Name: "github", Management: "referenced"}}

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

	cfg := testHarnessConfig(t)
	cfg.Spec.Providers = []config.Provider{{Name: "myoauth", Management: "referenced"}}

	results := checkProviderEnvVars(cfg, "nonexistent-cli", dir)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status == "fail" {
		t.Errorf("gateway-managed credential should not fail env var check, got: %s", results[0].Message)
	}
}

func TestCheckProviderEnvVars_UsesProviderTypeAsProfile(t *testing.T) {
	dir := t.TempDir()
	writeProviderProfile(t, dir, "google-vertex-ai", `
id: google-vertex-ai
credentials:
  - name: access_token
    env_vars: [VERTEX_TOKEN]
    required: true
`)
	t.Setenv("VERTEX_TOKEN", "test-value")

	cfg := testHarnessConfig(t)
	cfg.Spec.Providers = []config.Provider{{
		Name:       "team-vertex",
		Type:       "google-vertex-ai",
		Management: "managed",
	}}

	results := checkProviderEnvVars(cfg, "nonexistent-cli", dir)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Name != "team-vertex" || results[0].Status != "pass" {
		t.Errorf("result = %+v, want team-vertex pass", results[0])
	}
}

func TestConfiguredProviders_IncludesSandboxOnlyWithoutDuplicates(t *testing.T) {
	cfg := testHarnessConfig(t)
	cfg.Spec.Providers = []config.Provider{{Name: "declared", Type: "github"}}
	cfg.Spec.Sandbox.Providers = []string{"declared", "platform-owned", "platform-owned"}

	got := configuredProviders(cfg)
	want := []configuredProvider{
		{Name: "declared", Profile: "github"},
		{Name: "platform-owned", Profile: "platform-owned"},
	}
	if len(got) != len(want) {
		t.Fatalf("configured providers = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("configured provider %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// TestDoctorCmd_TargetFlagWiring exercises the full flag-pointer path through
// cobra: registerTargetFlags populates the *string pointers on Parse, and the
// RunE closure feeds them to openshell.ResolveTarget. It guards the plumbing at
// doctor.go's flag registration and dereference that the direct runOnlineChecks
// tests never touch. Offline checks may fail against the test env; we assert
// only which gateway the Factory was constructed for.
func TestDoctorCmd_TargetFlagWiring(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		env         string // value for $OPENSHELL_GATEWAY ("" = unset)
		wantGateway string
	}{
		{name: "flag", args: []string{"--gateway", "from-flag"}, wantGateway: "from-flag"},
		{name: "env fallback", args: nil, env: "from-env", wantGateway: "from-env"},
		{name: "flag wins over env", args: []string{"--gateway", "from-flag"}, env: "from-env", wantGateway: "from-flag"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.env != "" {
				t.Setenv(openshell.EnvGateway, tt.env)
			}
			c := testutil.NewFake("default", fake.WithHealthResult(&types.HealthResult{Healthy: true}))
			var gotGateway string
			factoryCalled := false
			recording := func(_ context.Context, tgt openshell.Target) (openshell.Client, error) {
				factoryCalled = true
				gotGateway = tgt.Gateway
				return c, nil
			}

			cmd := NewDoctorCmd(t.TempDir(), "nonexistent-cli", testDefaultConfig, recording)
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true
			cmd.SetArgs(append(tt.args, "--output", "json"))
			// Offline checks may report failures in the test environment; RunE
			// then returns a non-nil error after the online path has run. We only
			// care that the Factory saw the resolved gateway.
			_ = cmd.Execute()

			if !factoryCalled {
				t.Fatal("Factory was never called; online path did not run")
			}
			if gotGateway != tt.wantGateway {
				t.Errorf("Factory got gateway %q, want %q", gotGateway, tt.wantGateway)
			}
		})
	}
}

func TestDoctorCmd_UsesCanonicalConfigTargetAndProviders(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "harness.yaml")
	if err := os.WriteFile(configPath, []byte(`apiVersion: harness.openshell.dev/v1alpha1
kind: Harness
metadata:
  name: doctor-test
spec:
  target:
    gateway: from-config
    workspace: team
  providers:
    - name: github-team
      management: referenced
`), 0o644); err != nil {
		t.Fatal(err)
	}

	c, raw := testutil.NewFakeClient("team", fake.WithHealthResult(&types.HealthResult{Healthy: true}))
	raw.AddProvider("team", &types.Provider{Name: "github-team", Type: "github"})
	var gotTarget openshell.Target
	recording := func(_ context.Context, target openshell.Target) (openshell.Client, error) {
		gotTarget = target
		return c, nil
	}

	cmd := NewDoctorCmd(dir, "nonexistent-cli", testDefaultConfig, recording)
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetArgs([]string{"--file", configPath, "--output", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("doctor: %v", err)
	}
	if gotTarget.Gateway != "from-config" || gotTarget.Workspace != "team" {
		t.Errorf("factory target = %+v, want gateway from-config, workspace team", gotTarget)
	}
}

// --- helpers ---

func testHarnessConfig(t *testing.T) *config.Harness {
	t.Helper()
	return &config.Harness{
		APIVersion: "harness.openshell.dev/v1alpha1",
		Kind:       "Harness",
		Metadata:   config.Metadata{Name: "test-agent"},
		Spec:       config.Spec{Agent: config.Agent{Type: "claude"}},
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
	results := runOnlineChecks(context.Background(), f, openshell.Target{}, nil)
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
	results := runOnlineChecks(context.Background(), f, openshell.Target{Gateway: "some-gateway", Workspace: "default"}, nil)
	if len(results) != 1 || results[0].Status != "pass" {
		t.Fatalf("expected 1 pass result, got %+v", results)
	}
}

// TestRunOnlineChecks_GatewayIsolation is the plan's acceptance test: naming one
// gateway never constructs or queries another. A recording Factory captures every
// Target.Gateway it is asked to build; doctor's online path is run with gateway A
// and the recorder must show exactly one construction, for A only — B is never
// touched.
func TestRunOnlineChecks_GatewayIsolation(t *testing.T) {
	fakeA := testutil.NewFake("default", fake.WithHealthResult(&types.HealthResult{Healthy: true}))
	fakeB := testutil.NewFake("default", fake.WithHealthResult(&types.HealthResult{Healthy: true}))
	clients := map[string]openshell.Client{"A": fakeA, "B": fakeB}

	var constructed []string
	recording := func(_ context.Context, tgt openshell.Target) (openshell.Client, error) {
		constructed = append(constructed, tgt.Gateway)
		c, ok := clients[tgt.Gateway]
		if !ok {
			t.Fatalf("factory asked for unknown gateway %q", tgt.Gateway)
		}
		return c, nil
	}

	runOnlineChecks(context.Background(), recording,
		openshell.Target{Gateway: "A", Workspace: "default"}, nil)

	if len(constructed) != 1 || constructed[0] != "A" {
		t.Fatalf("expected exactly one construction for gateway A, got %v", constructed)
	}
}
