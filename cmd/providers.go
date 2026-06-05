package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/robbycochran/harness-openshell/internal/gateway"
	"github.com/robbycochran/harness-openshell/internal/status"
	"github.com/spf13/cobra"
)

func NewProvidersCmd(harnessDir, cli string) *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "providers",
		Short: "Register providers with the gateway",
		RunE: func(cmd *cobra.Command, args []string) error {
			gw := gateway.NewCLI(cli)
			return registerProviders(harnessDir, gw, force)
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Delete and recreate all providers")

	return cmd
}

func registerProviders(harnessDir string, gw gateway.Gateway, force bool) error {
	model := os.Getenv("OPENSHELL_MODEL")
	if model == "" {
		model = "claude-sonnet-4-6"
	}

	// Force mode: require no running sandboxes
	if force {
		sandboxes, _ := gw.SandboxList()
		if len(sandboxes) > 0 {
			return fmt.Errorf("cannot --force with running sandboxes — delete them first")
		}
		for _, name := range []string{"github", "vertex-local", "atlassian"} {
			gw.ProviderDelete(name)
		}
		fmt.Println("Deleted existing providers.")
	}

	// Enable providers v2
	status.Section("Enabling providers v2")
	if err := gw.SettingsSet("providers_v2_enabled", "true"); err != nil {
		return fmt.Errorf("enabling providers v2: %w", err)
	}

	// Import custom profiles
	status.Section("Importing custom profiles")
	profilesDir := filepath.Join(harnessDir, "sandbox", "profiles")
	if err := gw.ProviderProfileImport(profilesDir); err != nil {
		fmt.Println("  (already imported)")
	}

	// Register providers
	status.Section("Registering providers")

	// GitHub
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		if gw.ProviderGet("github") != nil {
			if err := gw.ProviderCreate("github", "github", gateway.ProviderCreateOpts{
				Credentials: []string{"GITHUB_TOKEN=" + token},
			}); err != nil {
				return fmt.Errorf("creating github provider: %w", err)
			}
			fmt.Println("  github — registered")
		} else {
			fmt.Println("  github — exists (use --force to recreate)")
		}
	} else {
		fmt.Println("  github — skipped (GITHUB_TOKEN not set)")
	}

	// Vertex AI
	home, _ := os.UserHomeDir()
	adcPath := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	if adcPath == "" {
		adcPath = filepath.Join(home, ".config", "gcloud", "application_default_credentials.json")
	}
	project := os.Getenv("ANTHROPIC_VERTEX_PROJECT_ID")
	region := os.Getenv("CLOUD_ML_REGION")
	if region == "" {
		region = "global"
	}

	if project == "" {
		project = readADCProject(adcPath)
	}

	if fileExists(adcPath) && project != "" {
		if gw.ProviderGet("vertex-local") != nil {
			if err := gw.ProviderCreate("vertex-local", "google-vertex-ai", gateway.ProviderCreateOpts{
				FromADC: true,
				Configs: []string{
					"VERTEX_AI_PROJECT_ID=" + project,
					"VERTEX_AI_REGION=" + region,
				},
			}); err != nil {
				return fmt.Errorf("creating vertex-local provider: %w", err)
			}
			fmt.Printf("  vertex-local — registered (project: %s, region: %s)\n", project, region)
		} else {
			fmt.Println("  vertex-local — exists (use --force to recreate)")
		}
		if err := gw.InferenceSet("vertex-local", model); err != nil {
			return fmt.Errorf("setting inference: %w", err)
		}
		fmt.Printf("  inference — model: %s\n", model)
	} else if !fileExists(adcPath) {
		fmt.Printf("  vertex-local — skipped (no ADC file at %s)\n", adcPath)
	} else {
		fmt.Println("  vertex-local — skipped (no project ID — set ANTHROPIC_VERTEX_PROJECT_ID or run gcloud auth application-default login)")
	}

	// Atlassian
	if token := os.Getenv("JIRA_API_TOKEN"); token != "" {
		if gw.ProviderGet("atlassian") != nil {
			if err := gw.ProviderCreate("atlassian", "atlassian", gateway.ProviderCreateOpts{
				Credentials: []string{"JIRA_API_TOKEN=" + token},
			}); err != nil {
				return fmt.Errorf("creating atlassian provider: %w", err)
			}
			fmt.Println("  atlassian — registered")
		} else {
			fmt.Println("  atlassian — exists (use --force to recreate)")
		}
	} else {
		fmt.Println("  atlassian — skipped (JIRA_API_TOKEN not set)")
	}

	// Show results
	status.Section("Providers")
	gw.ProviderList()

	status.Section("Inference")
	gw.InferenceGet()

	fmt.Println()
	fmt.Println("Done.")
	return nil
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

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
