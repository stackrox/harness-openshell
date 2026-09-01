package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/stackrox/harness-openshell/internal/agent"
	"github.com/stackrox/harness-openshell/internal/config"
	"github.com/stackrox/harness-openshell/internal/gateway"
	"github.com/stackrox/harness-openshell/internal/openshell"
	"github.com/stackrox/harness-openshell/internal/openshell/sdkclient"
	"github.com/stackrox/harness-openshell/internal/status"
	"gopkg.in/yaml.v3"
)

// vertexADCCreate is the SDK-native gcloud-ADC provider create, injected as a
// package var so tests can substitute it without a live gateway. Production
// binds it to sdkclient.CreateVertexProviderFromADC — the sole credentialed
// create path, which reads the ADC refresh token and hands it to the gateway
// (Create carries no secret; the refresh token rides Refresh().Configure). The
// ADC flow is the one credentialed create the harness now owns directly rather
// than shelling to the CLI bridge; gws OAuth and reference (--from-existing)
// still bootstrap on the bridge.
var vertexADCCreate = sdkclient.CreateVertexProviderFromADC

// createStrategy names how a not-yet-existing provider is bootstrapped on the CLI
// bridge. Credentialed creation (ADC/OAuth) has to stay on the bridge — the
// firewall Provider type cannot carry a secret (invariant 26) — so this is the one
// place the SDK-vs-bridge fork for *creation* lives. Once a provider exists the SDK
// reconcile (reconcile.ReconcileProviders) owns verify/update/adoption.
type createStrategy int

const (
	// strategyReference registers a provider from an existing credential already
	// present in the environment (github, atlassian). It is the default.
	strategyReference createStrategy = iota
	// strategyADC creates a provider from gcloud Application Default Credentials
	// (google-vertex-ai).
	strategyADC
	// strategyOAuth creates a provider with a gateway-managed OAuth refresh flow
	// (google-workspace).
	strategyOAuth
)

// providerCreatePlan is the single owner of "which create strategy" for a desired
// provider. It keys on how credentials are acquired (Credentials.Source) and the
// provider type — never on a hard-coded per-profile switch. This is the classifier
// invariant 26 points at: ADC/OAuth acquisition stays on the CLI bridge because the
// harness cannot express a credentialed create through the firewall by design.
func providerCreatePlan(p config.Provider) createStrategy {
	switch {
	case p.Credentials != nil && p.Credentials.Source == "gcloud-adc":
		return strategyADC
	case p.Type == "google-workspace" || p.Name == "google-workspace":
		return strategyOAuth
	default:
		return strategyReference
	}
}

// registerProviders bootstrap-creates every desired provider that does not yet
// exist on the gateway, dispatching each through providerCreatePlan. It performs
// no destructive delete and no SDK write: credential-preserving update and owner
// adoption are the SDK reconcile's job (reconcile.ReconcileProviders, run from
// upLocal after this). The register* helpers each no-op when their provider
// already exists, so this is safe to call on every apply.
func registerProviders(harnessDir string, gw gateway.Gateway, target openshell.Target, desired []config.Provider) error {
	status.Header("Providers")

	profilesDir := filepath.Join(harnessDir, "profiles", "providers")
	if err := gw.ProviderProfileImport(profilesDir); err != nil {
		status.Warnf("provider profile import: %v", err)
	}

	for _, p := range desired {
		if err := bootstrapProvider(harnessDir, gw, target, p); err != nil {
			return err
		}
	}
	return nil
}

// bootstrapProvider creates one absent provider via its create strategy.
func bootstrapProvider(harnessDir string, gw gateway.Gateway, target openshell.Target, p config.Provider) error {
	switch providerCreatePlan(p) {
	case strategyADC:
		return registerADC(gw, target, p.Name, adcConfigs())
	case strategyOAuth:
		return registerGWS(harnessDir, gw)
	default:
		return registerStandard(p.Name, p.Type, gw, nil)
	}
}

// adcConfigs resolves the Vertex project/region config passed to the ADC create,
// preserving the legacy resolution order: explicit env overrides first, then the
// ADC file's quota project, then a "global" region default.
func adcConfigs() map[string]string {
	home, _ := os.UserHomeDir()
	adcPath := envOr("GOOGLE_APPLICATION_CREDENTIALS",
		filepath.Join(home, ".config", "gcloud", "application_default_credentials.json"))
	project := envOr("ANTHROPIC_VERTEX_PROJECT_ID", readADCProject(adcPath))
	region := envOr("CLOUD_ML_REGION", "global")
	configs := map[string]string{"VERTEX_AI_REGION": region}
	if project != "" {
		configs["VERTEX_AI_PROJECT_ID"] = project
	}
	return configs
}

func ensureProviders(harnessDir string, gw gateway.Gateway, target openshell.Target, agentCfg *agent.AgentConfig, h *agent.Harness) []string {
	providerNames := agentCfg.ProviderNames()
	if len(providerNames) == 0 {
		return nil
	}
	// Import harness-local provider profiles before checking registration
	if h != nil && len(h.Providers) > 0 {
		tmpDir, err := os.MkdirTemp("", "harness-providers-")
		if err == nil {
			defer os.RemoveAll(tmpDir)
			for name, data := range h.Providers {
				os.WriteFile(filepath.Join(tmpDir, name+".yaml"), data, 0o644)
			}
			if err := gw.ProviderProfileImport(tmpDir); err != nil {
				status.Warnf("harness provider import: %v", err)
			}
		}
	}

	registered, missing := gateway.ValidateProviders(providerNames, gw)
	if len(missing) > 0 {
		desired, _ := desiredFromAgent(agentCfg, os.Getenv)
		if err := registerProviders(harnessDir, gw, target, desired); err != nil {
			status.Warnf("provider registration: %v", err)
		}
		registered, missing = gateway.ValidateProviders(providerNames, gw)
	}
	status.Header("Providers")
	for _, name := range registered {
		status.OKf("%s: registered", name)
	}
	for _, name := range missing {
		status.Failf("%s: not registered", name)
	}
	return registered
}

func registerStandard(name, profileType string, gw gateway.Gateway, configs []string) error {
	if gw.ProviderGet(name) == nil {
		status.Infof("%s: exists", name)
		return nil
	}
	if err := gw.ProviderCreate(name, profileType, gateway.ProviderCreateOpts{
		FromExisting: true,
		Configs:      configs,
	}); err != nil {
		return fmt.Errorf("%s: registration failed: %w", name, err)
	}
	status.OKf("%s: registered", name)
	return nil
}

// registerADC creates a google-vertex-ai provider from gcloud Application
// Default Credentials via the SDK (sdkclient.CreateVertexProviderFromADC),
// replacing the former `openshell provider create --from-gcloud-adc` shell-out.
// It reads the ADC refresh token and the gateway mints/rotates the Vertex access
// token server-side. It does not set the inference route: that write is the SDK
// reconcile path's job (reconcileGateway in executor.go), so provider
// registration and inference reconciliation stay separate concerns.
func registerADC(gw gateway.Gateway, target openshell.Target, name string, configs map[string]string) error {
	if gw.ProviderGet(name) == nil {
		status.Infof("%s: exists", name)
		return nil
	}
	adc, err := sdkclient.ReadGcloudADC("")
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := vertexADCCreate(ctx, target, name, configs, adc); err != nil {
		return fmt.Errorf("%s: registration failed: %w", name, err)
	}
	status.OKf("%s: registered", name)
	return nil
}

func registerGWS(harnessDir string, gw gateway.Gateway) error {
	if gw.ProviderGet("google-workspace") == nil {
		status.Info("google-workspace: exists")
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
	if err := gw.ProviderCreate("google-workspace", "google-workspace", gateway.ProviderCreateOpts{
		Credentials: []string{"GOOGLE_WORKSPACE_CLI_TOKEN=pending"},
	}); err != nil {
		return fmt.Errorf("creating google-workspace provider: %w", err)
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
	if err := gw.ProviderRefreshConfigure("google-workspace", gateway.ProviderRefreshOpts{
		CredentialKey:      "GOOGLE_WORKSPACE_CLI_TOKEN",
		Strategy:           "oauth2-refresh-token",
		Material:           material,
		SecretMaterialKeys: []string{"client_secret", "refresh_token"},
	}); err != nil {
		return fmt.Errorf("configuring google-workspace refresh: %w", err)
	}

	// Force an immediate refresh so the token is valid before the first sandbox.
	if err := gw.ProviderRefreshRotate("google-workspace", "GOOGLE_WORKSPACE_CLI_TOKEN"); err != nil {
		status.Infof("google-workspace: refresh rotate failed (token will refresh automatically): %v", err)
	}

	status.OK("google-workspace: registered (gateway-managed token refresh)")
	return nil
}

// gwsProfileScopes reads the refresh.scopes list from profiles/providers/gws.yaml
// and returns them as a space-separated string for use as OAuth scope material.
func gwsProfileScopes(harnessDir string) string {
	profilePath := filepath.Join(harnessDir, "profiles", "providers", "gws.yaml")
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

