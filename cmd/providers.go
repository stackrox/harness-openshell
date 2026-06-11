package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/robbycochran/harness-openshell/internal/gateway"
	"github.com/robbycochran/harness-openshell/internal/status"
	"gopkg.in/yaml.v3"
)

// registerProviders registers providers with the gateway. If gwCfg is non-nil
// and has a [providers] section, only providers in that list are registered.
// Otherwise all providers are registered (backward-compatible behavior).
func registerProviders(harnessDir string, gw gateway.Gateway, force bool, gwCfg *gateway.GatewayConfig, standalone bool) error {
	model := envOr("OPENSHELL_MODEL", "claude-sonnet-4-6")

	// Build the set of enabled provider names from gateway config (if available)
	var enabledSet map[string]bool
	if gwCfg != nil && gwCfg.HasProviders() {
		enabledSet = make(map[string]bool)
		for _, name := range gwCfg.AllProviders() {
			enabledSet[name] = true
		}
	}

	providerEnabled := func(name string) bool {
		if enabledSet == nil {
			return true
		}
		return enabledSet[name]
	}

	// Force mode: require no running sandboxes
	if force {
		sandboxes, err := gw.SandboxList()
		if err != nil {
			return fmt.Errorf("listing sandboxes: %w", err)
		}
		if len(sandboxes) > 0 {
			return fmt.Errorf("cannot --force with running sandboxes — delete them first")
		}
		for _, name := range []string{"github", "vertex-local", "atlassian", "gws"} {
			if providerEnabled(name) {
				gw.ProviderDelete(name)
			}
		}
		deleteCustomProfiles(harnessDir, gw)
		status.Info("Deleted existing providers")
	}

	status.Header("Providers")

	// Enable providers v2
	if err := gw.SettingsSet("providers_v2_enabled", "true"); err != nil {
		return fmt.Errorf("enabling providers v2: %w", err)
	}

	// Import custom profiles
	profilesDir := filepath.Join(harnessDir, "agents", "providers", "profiles")
	gw.ProviderProfileImport(profilesDir)

	if err := registerGitHub(gw, providerEnabled); err != nil {
		return err
	}
	if err := registerVertexAI(gw, model, providerEnabled); err != nil {
		return err
	}
	if err := registerAtlassian(gw, providerEnabled); err != nil {
		return err
	}
	if err := registerGWS(harnessDir, gw, providerEnabled); err != nil {
		return err
	}

	if standalone {
		names, err := gw.ProviderList()
		if err != nil {
			return fmt.Errorf("listing providers: %w", err)
		}
		fmt.Println()
		for _, n := range names {
			status.OK(n)
		}
		m := gw.InferenceModel()
		if m != "" {
			status.OKf("Inference: %s", m)
		}
		status.Done("Done. Launch a sandbox with: harness up --local")
	}
	return nil
}

func registerGitHub(gw gateway.Gateway, enabled func(string) bool) error {
	if !enabled("github") {
		status.Info("github: disabled by gateway config")
		return nil
	}
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		status.Info("github: skipped (GITHUB_TOKEN not set)")
		return nil
	}
	if gw.ProviderGet("github") == nil {
		status.Info("github: exists (use --force to recreate)")
		return nil
	}
	if err := gw.ProviderCreate("github", "github", gateway.ProviderCreateOpts{
		Credentials: []string{"GITHUB_TOKEN=" + token},
	}); err != nil {
		return fmt.Errorf("creating github provider: %w", err)
	}
	status.OK("github: registered")
	return nil
}

func registerVertexAI(gw gateway.Gateway, model string, enabled func(string) bool) error {
	if !enabled("vertex-local") {
		status.Info("vertex-local: disabled by gateway config")
		return nil
	}
	home, _ := os.UserHomeDir()
	adcPath := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	if adcPath == "" {
		adcPath = filepath.Join(home, ".config", "gcloud", "application_default_credentials.json")
	}
	project := os.Getenv("ANTHROPIC_VERTEX_PROJECT_ID")
	region := envOr("CLOUD_ML_REGION", "global")

	if project == "" {
		project = readADCProject(adcPath)
	}

	if !fileExists(adcPath) {
		status.Infof("vertex-local: skipped (no ADC file at %s)", adcPath)
		return nil
	}
	if project == "" {
		status.Info("vertex-local: skipped (no project ID — set ANTHROPIC_VERTEX_PROJECT_ID)")
		return nil
	}
	if gw.ProviderGet("vertex-local") == nil {
		status.Info("vertex-local: exists (use --force to recreate)")
	} else {
		if err := gw.ProviderCreate("vertex-local", "google-vertex-ai", gateway.ProviderCreateOpts{
			FromADC: true,
			Configs: []string{
				"VERTEX_AI_PROJECT_ID=" + project,
				"VERTEX_AI_REGION=" + region,
			},
		}); err != nil {
			return fmt.Errorf("creating vertex-local provider: %w", err)
		}
		status.OKf("vertex-local: registered (project: %s, region: %s)", project, region)
	}
	if err := gw.InferenceSet("vertex-local", model); err != nil {
		return fmt.Errorf("setting inference: %w", err)
	}
	status.OKf("inference: model %s", model)
	return nil
}

func registerAtlassian(gw gateway.Gateway, enabled func(string) bool) error {
	if !enabled("atlassian") {
		status.Info("atlassian: disabled by gateway config")
		return nil
	}
	token := os.Getenv("JIRA_API_TOKEN")
	if token == "" {
		status.Info("atlassian: skipped (JIRA_API_TOKEN not set)")
		return nil
	}
	if gw.ProviderGet("atlassian") == nil {
		status.Info("atlassian: exists (use --force to recreate)")
		return nil
	}
	if err := gw.ProviderCreate("atlassian", "atlassian", gateway.ProviderCreateOpts{
		Credentials: []string{"JIRA_API_TOKEN=" + token},
	}); err != nil {
		return fmt.Errorf("creating atlassian provider: %w", err)
	}
	status.OK("atlassian: registered")
	return nil
}

func registerGWS(harnessDir string, gw gateway.Gateway, enabled func(string) bool) error {
	if !enabled("gws") {
		status.Info("gws: disabled by gateway config")
		return nil
	}
	if gw.ProviderGet("gws") == nil {
		status.Info("gws: exists (use --force to recreate)")
		return nil
	}

	gwsPath, _ := exec.LookPath("gws")
	if gwsPath == "" {
		status.Info("gws: not installed (skipping)")
		return nil
	}

	out, err := exec.Command(gwsPath, "auth", "export", "--unmasked").Output()
	if err != nil {
		status.Info("gws: not authenticated (run 'gws auth login')")
		return nil
	}

	var creds struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.Unmarshal(out, &creds); err != nil {
		return fmt.Errorf("parsing gws credentials: %w", err)
	}
	if creds.ClientID == "" || creds.ClientSecret == "" || creds.RefreshToken == "" {
		status.Info("gws: incomplete credentials (skipping)")
		return nil
	}

	// Create provider with a placeholder — the gateway will refresh it immediately.
	if err := gw.ProviderCreate("gws", "google-workspace", gateway.ProviderCreateOpts{
		Credentials: []string{"GOOGLE_WORKSPACE_CLI_TOKEN=pending"},
	}); err != nil {
		return fmt.Errorf("creating gws provider: %w", err)
	}

	// Read scopes from the provider profile so they're defined in one place.
	profileScopes := gwsProfileScopes(harnessDir)

	// Configure gateway-managed OAuth refresh. The gateway stores client_secret
	// and refresh_token as secret material — they are never injected into sandboxes.
	// Scopes are passed as material so Google mints a narrowed access token —
	// only these scopes are accessible even though the refresh_token has more.
	material := []string{
		"client_id=" + creds.ClientID,
		"client_secret=" + creds.ClientSecret,
		"refresh_token=" + creds.RefreshToken,
	}
	if profileScopes != "" {
		material = append(material, "scopes="+profileScopes)
	}
	if err := gw.ProviderRefreshConfigure("gws", gateway.ProviderRefreshOpts{
		CredentialKey:      "GOOGLE_WORKSPACE_CLI_TOKEN",
		Strategy:           "oauth2-refresh-token",
		Material:           material,
		SecretMaterialKeys: []string{"client_secret", "refresh_token"},
	}); err != nil {
		return fmt.Errorf("configuring gws refresh: %w", err)
	}

	// Force an immediate refresh so the token is valid before the first sandbox.
	if err := gw.ProviderRefreshRotate("gws", "GOOGLE_WORKSPACE_CLI_TOKEN"); err != nil {
		status.Infof("gws: refresh rotate failed (token will refresh automatically): %v", err)
	}

	status.OK("gws: registered (gateway-managed token refresh)")
	return nil
}

// gwsProfileScopes reads the refresh.scopes list from agents/providers/profiles/gws.yaml
// and returns them as a space-separated string for use as OAuth scope material.
func gwsProfileScopes(harnessDir string) string {
	profilePath := filepath.Join(harnessDir, "agents", "providers", "profiles", "gws.yaml")
	data, err := os.ReadFile(profilePath)
	if err != nil {
		return ""
	}
	var profile struct {
		Credentials []struct {
			Refresh struct {
				Scopes []string `yaml:"scopes"`
			} `yaml:"refresh"`
		} `yaml:"credentials"`
	}
	if err := yaml.Unmarshal(data, &profile); err != nil || len(profile.Credentials) == 0 {
		return ""
	}
	return strings.Join(profile.Credentials[0].Refresh.Scopes, " ")
}

func deleteCustomProfiles(harnessDir string, gw gateway.Gateway) {
	profilesDir := filepath.Join(harnessDir, "agents", "providers", "profiles")
	entries, err := os.ReadDir(profilesDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		id := extractYAMLID(filepath.Join(profilesDir, e.Name()))
		if id != "" {
			gw.ProviderProfileDelete(id)
		}
	}
}

func extractYAMLID(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if id, ok := strings.CutPrefix(line, "id:"); ok {
			return strings.TrimSpace(id)
		}
	}
	return ""
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func readADCProject(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var adc struct {
		QuotaProjectID string `json:"quota_project_id"`
	}
	if json.Unmarshal(data, &adc) != nil {
		return ""
	}
	return adc.QuotaProjectID
}

