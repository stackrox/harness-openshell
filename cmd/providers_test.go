package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stackrox/harness-openshell/internal/config"
	"github.com/stackrox/harness-openshell/internal/openshell"
	"github.com/stackrox/harness-openshell/internal/openshell/sdkclient"
)

func setupProvidersTest(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "profiles", "providers"), 0o755)
	return dir
}

// swapVertexADCCreate substitutes the SDK-native ADC create seam for a test
// double and restores it afterward, so registerADC can be exercised without a
// live gateway.
func swapVertexADCCreate(t *testing.T, fn func(context.Context, openshell.Target, string, map[string]string, sdkclient.ADC) error) {
	t.Helper()
	orig := vertexADCCreate
	vertexADCCreate = fn
	t.Cleanup(func() { vertexADCCreate = orig })
}

// writeValidADC writes an authorized_user ADC file and points
// GOOGLE_APPLICATION_CREDENTIALS at it so registerADC's ReadGcloudADC("")
// resolves deterministically in tests.
func writeValidADC(t *testing.T) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "application_default_credentials.json")
	if err := os.WriteFile(path, []byte(`{"type":"authorized_user","client_id":"c","client_secret":"s","refresh_token":"r"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", path)
}

// TestProviderCreatePlan pins the single owner of "which create strategy": it keys
// on Credentials.Source and provider type, never on a hard-coded profile switch.
func TestProviderCreatePlan(t *testing.T) {
	cases := []struct {
		name string
		p    config.Provider
		want createStrategy
	}{
		{"github references existing", config.Provider{Name: "github", Type: "github"}, strategyReference},
		{"atlassian references existing", config.Provider{Name: "atlassian", Type: "atlassian"}, strategyReference},
		{
			"vertex uses ADC via credential source",
			config.Provider{Name: "google-vertex-ai", Type: "google-vertex-ai", Credentials: &config.SecretRef{Source: "gcloud-adc"}},
			strategyADC,
		},
		{"workspace uses OAuth via type", config.Provider{Name: "google-workspace", Type: "google-workspace"}, strategyOAuth},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := providerCreatePlan(tc.p); got != tc.want {
				t.Errorf("providerCreatePlan(%+v) = %v, want %v", tc.p, got, tc.want)
			}
		})
	}
}

func TestRegisterProviders_BootstrapsAbsentReference(t *testing.T) {
	dir := setupProvidersTest(t)
	gw := &mockGW{providers: map[string]bool{}}

	err := registerProviders(dir, gw, openshell.Target{}, []config.Provider{
		{Name: "github", Type: "github"},
	})
	if err != nil {
		t.Fatalf("registerProviders: %v", err)
	}
	if len(gw.providerCreates) != 1 {
		t.Fatalf("providerCreates = %d, want 1", len(gw.providerCreates))
	}
	c := gw.providerCreates[0]
	if c.name != "github" || c.profileType != "github" {
		t.Errorf("create = %q/%q, want github/github", c.name, c.profileType)
	}
	if !c.opts.FromExisting {
		t.Errorf("github should register FromExisting, got %+v", c.opts)
	}
}

// TestRegisterProviders_VertexUsesADC pins that the vertex strategy now creates
// via the SDK-native ADC seam (not the CLI bridge): the seam is invoked with the
// provider name, the resolved Vertex config, and the ADC material, and the CLI
// bridge ProviderCreate is never touched for it.
func TestRegisterProviders_VertexUsesADC(t *testing.T) {
	dir := setupProvidersTest(t)
	gw := &mockGW{providers: map[string]bool{}}
	writeValidADC(t)
	t.Setenv("ANTHROPIC_VERTEX_PROJECT_ID", "test-proj")

	var gotName string
	var gotConfig map[string]string
	var gotADC sdkclient.ADC
	calls := 0
	swapVertexADCCreate(t, func(_ context.Context, _ openshell.Target, name string, config map[string]string, adc sdkclient.ADC) error {
		calls++
		gotName, gotConfig, gotADC = name, config, adc
		return nil
	})

	err := registerProviders(dir, gw, openshell.Target{Gateway: "openshell"}, []config.Provider{
		{Name: "google-vertex-ai", Type: "google-vertex-ai", Credentials: &config.SecretRef{Source: "gcloud-adc"}},
	})
	if err != nil {
		t.Fatalf("registerProviders: %v", err)
	}
	if calls != 1 {
		t.Fatalf("SDK ADC create called %d times, want 1", calls)
	}
	if len(gw.providerCreates) != 0 {
		t.Errorf("CLI bridge ProviderCreate called %d times, want 0 (ADC is SDK-native)", len(gw.providerCreates))
	}
	if gotName != "google-vertex-ai" {
		t.Errorf("create name = %q, want google-vertex-ai", gotName)
	}
	if gotConfig["VERTEX_AI_PROJECT_ID"] != "test-proj" || gotConfig["VERTEX_AI_REGION"] != "global" {
		t.Errorf("config = %v, want project=test-proj region=global", gotConfig)
	}
	if gotADC.RefreshToken != "r" {
		t.Errorf("adc.RefreshToken = %q, want r", gotADC.RefreshToken)
	}
}

func TestRegisterProviders_SkipsExistingProvider(t *testing.T) {
	dir := setupProvidersTest(t)
	gw := &mockGW{providers: map[string]bool{"github": true}}

	err := registerProviders(dir, gw, openshell.Target{}, []config.Provider{
		{Name: "github", Type: "github"},
	})
	if err != nil {
		t.Fatalf("registerProviders: %v", err)
	}
	if len(gw.providerCreates) != 0 {
		t.Errorf("providerCreates = %d, want 0 (already exists)", len(gw.providerCreates))
	}
}

func TestRegisterProviders_EmptyList(t *testing.T) {
	dir := setupProvidersTest(t)
	gw := &mockGW{providers: map[string]bool{}}

	err := registerProviders(dir, gw, openshell.Target{}, nil)
	if err != nil {
		t.Fatalf("registerProviders: %v", err)
	}
	if len(gw.providerCreates) != 0 {
		t.Errorf("providerCreates = %d, want 0", len(gw.providerCreates))
	}
}
