package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/stackrox/harness-openshell/internal/config"
	"github.com/stackrox/harness-openshell/internal/openshell"
	"github.com/stackrox/harness-openshell/internal/status"
	"gopkg.in/yaml.v3"
)

type CheckResult struct {
	Group   string `json:"group"`
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

type CheckFunc func(cfg *config.Harness, cli, harnessDir string) []CheckResult

func NewDoctorCmd(harnessDir, cli string, defaultCfg []byte, newClient openshell.Factory) *cobra.Command {
	var (
		file   string
		output string
	)
	// Assigned by registerTargetFlags below; RunE reads them at execution time.
	var gatewayName, workspace *string

	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Validate environment for configured sandbox",
		Long: `Check that prerequisites are met for running a sandbox.

Phase 1 (offline): checks the openshell binary and provider credentials
without requiring a running gateway.

Phase 2 (online): if the gateway is reachable, checks provider registration.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := parseOutputFormat(output)
			if err != nil {
				return err
			}

			var h *config.Harness
			var target openshell.Target
			if file != "" {
				workflow, err := loadWorkflow(file, *gatewayName, *workspace, applyOverrides{})
				if err != nil {
					return err
				}
				h = workflow.Desired
				target = workflow.Target
			} else {
				h, err = config.Parse(defaultCfg)
				if err != nil {
					return fmt.Errorf("parsing default config: %w", err)
				}
				h, err = config.Resolve(h, os.Getenv)
				if err != nil {
					return fmt.Errorf("resolving default config: %w", err)
				}
				target = openshell.ResolveTarget(*gatewayName, *workspace, h.Spec.Target.Gateway, h.Spec.Target.Workspace, os.Getenv)
			}

			checks := []CheckFunc{
				checkOpenShell,
				checkProviderEnvVars,
			}

			var results []CheckResult
			for _, fn := range checks {
				results = append(results, fn(h, cli, harnessDir)...)
			}

			providers := configuredProviders(h)
			providerNames := make([]string, len(providers))
			for i, provider := range providers {
				providerNames[i] = provider.Name
			}
			results = append(results, runOnlineChecks(cmd.Context(), newClient, target, providerNames)...)

			if format != formatTable {
				return printStructured(format, results)
			}

			printDoctorTable(results)

			for _, r := range results {
				if r.Status == "fail" {
					return fmt.Errorf("one or more checks failed")
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&file, "file", "f", "", "Path to harness YAML")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Output format (table, json, yaml)")
	gatewayName, workspace = registerTargetFlags(cmd)

	return cmd
}

func checkOpenShell(_ *config.Harness, cli, _ string) []CheckResult {
	path, err := exec.LookPath(cli)
	if err != nil {
		return []CheckResult{{
			Group:   "openshell",
			Name:    "binary",
			Status:  "fail",
			Message: fmt.Sprintf("%s not found on PATH", cli),
		}}
	}

	status.Cmd(path, "--version")
	out, err := exec.Command(path, "--version").Output()
	if err != nil {
		return []CheckResult{{
			Group:   "openshell",
			Name:    "binary",
			Status:  "warn",
			Message: fmt.Sprintf("found at %s, version unknown", path),
		}}
	}

	version := strings.TrimSpace(string(out))
	return []CheckResult{{
		Group:   "openshell",
		Name:    "binary",
		Status:  "pass",
		Message: version,
	}}
}

type providerProfile struct {
	ID          string               `yaml:"id"`
	DisplayName string               `yaml:"display_name"`
	Credentials []providerCredential `yaml:"credentials"`
}

type providerCredential struct {
	Name     string   `yaml:"name"`
	EnvVars  []string `yaml:"env_vars"`
	Required bool     `yaml:"required"`
	Refresh  *struct {
		Strategy string `yaml:"strategy"`
	} `yaml:"refresh,omitempty"`
}

func checkProviderEnvVars(cfg *config.Harness, cli, harnessDir string) []CheckResult {
	providers := configuredProviders(cfg)
	if len(providers) == 0 {
		return nil
	}

	var results []CheckResult
	for _, provider := range providers {
		profile := loadProviderProfile(provider.Profile, cli, harnessDir)
		if profile == nil {
			results = append(results, CheckResult{
				Group:   "provider",
				Name:    provider.Name,
				Status:  "warn",
				Message: "no profile found, cannot check credentials",
			})
			continue
		}

		allGatewayManaged := true
		allSet := true
		var missing []string
		for _, cred := range profile.Credentials {
			if !cred.Required {
				continue
			}
			// Gateway-managed credentials (OAuth refresh, service account JWT)
			// are handled by the gateway, not set by the user as env vars.
			if cred.Refresh != nil {
				continue
			}
			allGatewayManaged = false
			found := false
			for _, ev := range cred.EnvVars {
				if os.Getenv(ev) != "" {
					found = true
					break
				}
			}
			if !found {
				allSet = false
				missing = append(missing, cred.EnvVars[0])
			}
		}

		if allGatewayManaged {
			r := checkGatewayManagedProvider(provider.Profile)
			r.Name = provider.Name
			results = append(results, r)
		} else if allSet {
			results = append(results, CheckResult{
				Group:   "provider",
				Name:    provider.Name,
				Status:  "pass",
				Message: "credentials set",
			})
		} else {
			results = append(results, CheckResult{
				Group:   "provider",
				Name:    provider.Name,
				Status:  "fail",
				Message: "missing: " + strings.Join(missing, ", "),
			})
		}
	}

	return results
}

type configuredProvider struct {
	Name    string
	Profile string
}

func configuredProviders(cfg *config.Harness) []configuredProvider {
	providers := make([]configuredProvider, 0, len(cfg.Spec.Providers)+len(cfg.Spec.Sandbox.Providers))
	seen := make(map[string]struct{}, cap(providers))
	for _, provider := range cfg.Spec.Providers {
		profile := provider.Type
		if profile == "" {
			profile = provider.Name
		}
		providers = append(providers, configuredProvider{Name: provider.Name, Profile: profile})
		seen[provider.Name] = struct{}{}
	}
	for _, name := range cfg.Spec.Sandbox.Providers {
		if _, ok := seen[name]; ok {
			continue
		}
		providers = append(providers, configuredProvider{Name: name, Profile: name})
		seen[name] = struct{}{}
	}
	return providers
}

func checkGatewayManagedProvider(name string) CheckResult {
	switch name {
	case "google-workspace":
		gwsPath, _ := exec.LookPath("gws")
		if gwsPath == "" {
			return CheckResult{Group: "provider", Name: name, Status: "fail", Message: "gws CLI not installed (brew install googleworkspace/cli/gws)"}
		}
		status.Cmd(gwsPath, "auth", "export", "--unmasked")
		if err := exec.Command(gwsPath, "auth", "export", "--unmasked").Run(); err != nil {
			return CheckResult{Group: "provider", Name: name, Status: "fail", Message: "not authenticated (run: gws auth login)"}
		}
		return CheckResult{Group: "provider", Name: name, Status: "pass", Message: "authenticated (gateway-managed OAuth)"}
	case "google-vertex-ai":
		home, _ := os.UserHomeDir()
		adcPath := envOr("GOOGLE_APPLICATION_CREDENTIALS",
			filepath.Join(home, ".config", "gcloud", "application_default_credentials.json"))
		if _, err := os.Stat(adcPath); err != nil {
			return CheckResult{Group: "provider", Name: name, Status: "fail", Message: "ADC not found (run: gcloud auth application-default login)"}
		}
		return CheckResult{Group: "provider", Name: name, Status: "pass", Message: "ADC found (gateway-managed refresh)"}
	default:
		return CheckResult{Group: "provider", Name: name, Status: "pass", Message: "gateway-managed credentials"}
	}
}

func loadProviderProfile(name, cli, harnessDir string) *providerProfile {
	if profile := loadProfileFromOpenShell(name, cli); profile != nil {
		return profile
	}
	return loadProfileFromDisk(name, harnessDir)
}

func loadProfileFromOpenShell(name, cli string) *providerProfile {
	path, err := exec.LookPath(cli)
	if err != nil {
		return nil
	}
	status.Cmd(path, "provider", "profile", "export", name)
	out, err := exec.Command(path, "provider", "profile", "export", name).Output()
	if err != nil {
		return nil
	}
	var p providerProfile
	if err := yaml.Unmarshal(out, &p); err != nil {
		return nil
	}
	return &p
}

func loadProfileFromDisk(name, harnessDir string) *providerProfile {
	if harnessDir == "" {
		return nil
	}
	candidates := []string{
		filepath.Join(harnessDir, "profiles", "providers", name+".yaml"),
	}
	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var p providerProfile
		if err := yaml.Unmarshal(data, &p); err != nil {
			continue
		}
		return &p
	}
	return nil
}

// runOnlineChecks performs Phase 2 (online) checks via the SDK. It is
// non-fatal by construction: a missing --gateway or any client-construction
// failure yields a single warn (Phase 2 skipped), never a fail, preserving
// doctor's long-standing "online failures don't break the build" contract.
func runOnlineChecks(ctx context.Context, newClient openshell.Factory, target openshell.Target, providers []string) []CheckResult {
	if target.Gateway == "" && target.Direct == nil {
		return []CheckResult{{
			Group:   "gateway",
			Name:    "status",
			Status:  "warn",
			Message: "Phase 2 (online) checks skipped: no gateway specified",
		}}
	}

	client, err := newClient(ctx, target)
	if err != nil {
		return []CheckResult{{
			Group:   "gateway",
			Name:    "status",
			Status:  "warn",
			Message: fmt.Sprintf("Phase 2 (online) checks skipped: %v", err),
		}}
	}
	defer client.Close()

	return checkOnlineSDK(ctx, client, providers)
}

// checkOnlineSDK reads gateway health and provider registration through the
// SDK client. Health maps: healthy -> pass; ErrUnauthenticated -> fail (the
// one actionable online failure); ErrUnavailable and any other error -> warn
// (non-fatal). Provider rows are only produced when the gateway is healthy.
func checkOnlineSDK(ctx context.Context, client openshell.Client, providers []string) []CheckResult {
	h, err := client.Health(ctx)
	switch {
	case err == nil && h.Healthy:
		// fall through to provider checks below
	case errors.Is(err, openshell.ErrUnauthenticated):
		return []CheckResult{{
			Group:   "gateway",
			Name:    "status",
			Status:  "fail",
			Message: "authentication failed — check gateway credentials",
		}}
	case errors.Is(err, openshell.ErrUnavailable):
		return []CheckResult{{
			Group:   "gateway",
			Name:    "status",
			Status:  "warn",
			Message: "gateway not reachable (Phase 2 checks skipped)",
		}}
	case err != nil:
		return []CheckResult{{
			Group:   "gateway",
			Name:    "status",
			Status:  "warn",
			Message: fmt.Sprintf("gateway health check failed: %v (Phase 2 checks skipped)", err),
		}}
	default: // err == nil but not healthy
		return []CheckResult{{
			Group:   "gateway",
			Name:    "status",
			Status:  "warn",
			Message: "gateway reports unhealthy (Phase 2 checks skipped)",
		}}
	}

	results := []CheckResult{{
		Group:   "gateway",
		Name:    "status",
		Status:  "pass",
		Message: "connected",
	}}

	provs, err := client.Providers(ctx)
	if err != nil {
		results = append(results, CheckResult{
			Group:   "gateway",
			Name:    "providers",
			Status:  "warn",
			Message: fmt.Sprintf("could not list providers: %v", err),
		})
		return results
	}

	registered := make(map[string]bool, len(provs))
	for _, p := range provs {
		registered[p.Name] = true
	}
	for _, name := range providers {
		if registered[name] {
			results = append(results, CheckResult{
				Group:   "gateway",
				Name:    name,
				Status:  "pass",
				Message: "registered",
			})
		} else {
			results = append(results, CheckResult{
				Group:   "gateway",
				Name:    name,
				Status:  "warn",
				Message: "not registered (create through platform bootstrap before apply)",
			})
		}
	}

	return results
}

func printDoctorTable(results []CheckResult) {
	groups := []string{"openshell", "provider", "gateway"}
	groupLabels := map[string]string{
		"openshell": "OPENSHELL",
		"provider":  "PROVIDER",
		"gateway":   "GATEWAY",
	}

	for _, g := range groups {
		var groupResults []CheckResult
		for _, r := range results {
			if r.Group == g {
				groupResults = append(groupResults, r)
			}
		}
		if len(groupResults) == 0 {
			continue
		}

		fmt.Println(groupLabels[g])
		for _, r := range groupResults {
			icon := "  "
			switch r.Status {
			case "pass":
				icon = "OK"
			case "warn":
				icon = "!!"
			case "fail":
				icon = "XX"
			}
			fmt.Printf("  %-16s %s  %s\n", r.Name, icon, r.Message)
		}
		fmt.Println()
	}
}
