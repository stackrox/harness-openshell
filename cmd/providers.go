package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/robbycochran/harness-openshell/internal/agent"
	"github.com/robbycochran/harness-openshell/internal/gateway"
	"github.com/robbycochran/harness-openshell/internal/status"
	"gopkg.in/yaml.v3"
)

// registerProviders registers the providers listed in the agent config with
// the gateway. Only providers in the agent YAML are registered. Provider
// config values are passed via --config during registration.
func registerProviders(harnessDir string, gw gateway.Gateway, force bool, providers []agent.ProviderRef) error {
	model := envOr("OPENSHELL_MODEL", "claude-sonnet-4-6")

	wanted := make(map[string]*agent.ProviderRef, len(providers))
	for i := range providers {
		wanted[providers[i].Profile] = &providers[i]
	}

	if force {
		sandboxes, err := gw.SandboxList()
		if err != nil {
			return fmt.Errorf("listing sandboxes: %w", err)
		}
		if len(sandboxes) > 0 {
			return fmt.Errorf("cannot --provider-refresh with running sandboxes — delete them first")
		}
		for _, p := range providers {
			gw.ProviderDelete(p.Profile)
		}
		deleteCustomProfiles(harnessDir, gw)
		status.Info("Deleted existing providers")
	}

	status.Header("Providers")

	if err := gw.SettingsSet("providers_v2_enabled", "true"); err != nil {
		return fmt.Errorf("enabling providers v2: %w", err)
	}

	profilesDir := filepath.Join(harnessDir, "agents", "providers", "profiles")
	gw.ProviderProfileImport(profilesDir)

	if _, ok := wanted["github"]; ok {
		registerStandard("github", "github", gw, nil)
	}
	if _, ok := wanted["vertex-local"]; ok {
		home, _ := os.UserHomeDir()
		adcPath := envOr("GOOGLE_APPLICATION_CREDENTIALS",
			filepath.Join(home, ".config", "gcloud", "application_default_credentials.json"))
		project := envOr("ANTHROPIC_VERTEX_PROJECT_ID", readADCProject(adcPath))
		region := envOr("CLOUD_ML_REGION", "global")
		var configs []string
		if project != "" {
			configs = append(configs, "VERTEX_AI_PROJECT_ID="+project)
		}
		configs = append(configs, "VERTEX_AI_REGION="+region)
		registerADC("vertex-local", "google-vertex-ai", model, gw, configs)
	}
	if _, ok := wanted["atlassian"]; ok {
		registerStandard("atlassian", "atlassian", gw, nil)
	}
	if _, ok := wanted["gws"]; ok {
		if err := registerGWS(harnessDir, gw); err != nil {
			return err
		}
	}

	return nil
}

func registerStandard(name, profileType string, gw gateway.Gateway, configs []string) {
	if gw.ProviderGet(name) == nil {
		status.Infof("%s: exists", name)
		return
	}
	if err := gw.ProviderCreate(name, profileType, gateway.ProviderCreateOpts{
		FromExisting: true,
		Configs:      configs,
	}); err != nil {
		status.Infof("%s: skipped (%v)", name, err)
		return
	}
	status.OKf("%s: registered", name)
}

func registerADC(name, profileType, model string, gw gateway.Gateway, configs []string) {
	if gw.ProviderGet(name) == nil {
		status.Infof("%s: exists", name)
		return
	}
	if err := gw.ProviderCreate(name, profileType, gateway.ProviderCreateOpts{
		FromADC: true,
		Configs: configs,
	}); err != nil {
		status.Infof("%s: skipped (%v)", name, err)
		return
	}
	status.OKf("%s: registered", name)
	if err := gw.InferenceSet(name, model); err != nil {
		status.Infof("inference: %v", err)
		return
	}
	status.OKf("inference: model %s", model)
}

func registerGWS(harnessDir string, gw gateway.Gateway) error {
	if gw.ProviderGet("gws") == nil {
		status.Info("gws: exists (use --force to recreate)")
		return nil
	}

	gwsPath, _ := exec.LookPath("gws")
	if gwsPath == "" {
		status.Info("gws: not installed (skipping)")
		return nil
	}

	status.Cmd("gws", "auth", "export", "--unmasked")
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

