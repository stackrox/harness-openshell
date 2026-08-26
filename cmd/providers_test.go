package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stackrox/harness-openshell/internal/config"
)

func setupProvidersTest(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "profiles", "providers"), 0o755)
	return dir
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

	err := registerProviders(dir, gw, []config.Provider{
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

func TestRegisterProviders_VertexUsesADC(t *testing.T) {
	dir := setupProvidersTest(t)
	gw := &mockGW{providers: map[string]bool{}}

	err := registerProviders(dir, gw, []config.Provider{
		{Name: "google-vertex-ai", Type: "google-vertex-ai", Credentials: &config.SecretRef{Source: "gcloud-adc"}},
	})
	if err != nil {
		t.Fatalf("registerProviders: %v", err)
	}
	if len(gw.providerCreates) != 1 {
		t.Fatalf("providerCreates = %d, want 1", len(gw.providerCreates))
	}
	c := gw.providerCreates[0]
	if c.name != "google-vertex-ai" || !c.opts.FromADC {
		t.Errorf("vertex create = %q FromADC=%v, want google-vertex-ai FromADC=true", c.name, c.opts.FromADC)
	}
}

func TestRegisterProviders_SkipsExistingProvider(t *testing.T) {
	dir := setupProvidersTest(t)
	gw := &mockGW{providers: map[string]bool{"github": true}}

	err := registerProviders(dir, gw, []config.Provider{
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

	err := registerProviders(dir, gw, nil)
	if err != nil {
		t.Fatalf("registerProviders: %v", err)
	}
	if len(gw.providerCreates) != 0 {
		t.Errorf("providerCreates = %d, want 0", len(gw.providerCreates))
	}
}
