package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/stackrox/harness-openshell/internal/config"
	"gopkg.in/yaml.v3"
)

type availableProvider struct {
	ID          string
	DisplayName string
	Category    string
}

var defaultProviders = []availableProvider{
	{ID: "github", DisplayName: "GitHub", Category: "source-control"},
	{ID: "google-vertex-ai", DisplayName: "Google Vertex AI", Category: "inference"},
	{ID: "atlassian", DisplayName: "Atlassian", Category: "knowledge"},
	{ID: "google-workspace", DisplayName: "Google Workspace", Category: "knowledge"},
}

// NewInitCmd constructs the command that generates a starter workflow file.
func NewInitCmd(defaultCfg []byte) *cobra.Command {
	var (
		outputPath     string
		force          bool
		nonInteractive bool
	)

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Generate a harness.yaml config file",
		Long: `Create a harness.yaml by selecting an entrypoint and providers.
The generated config is yours to version, share, and customize.

Use --non-interactive to write the embedded default config without prompts.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return initRun(os.Stdin, os.Stdout, outputPath, force, nonInteractive, defaultCfg)
		},
	}

	cmd.Flags().StringVarP(&outputPath, "output", "o", "harness.yaml", "Output file path")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite existing file")
	cmd.Flags().BoolVar(&nonInteractive, "non-interactive", false, "Use defaults without prompting")

	return cmd
}

// initRun writes a starter workflow, optionally collecting interactive choices.
func initRun(in io.Reader, out io.Writer, outputPath string, force, nonInteractive bool, defaultCfg []byte) error {
	if _, err := os.Stat(outputPath); err == nil && !force {
		return fmt.Errorf("%s already exists (use --force to overwrite)", outputPath)
	}

	cfg, err := config.Parse(defaultCfg)
	if err != nil {
		return fmt.Errorf("parsing default config: %w", err)
	}

	if !nonInteractive {
		scanner := bufio.NewScanner(in)

		entrypoint, err := promptEntrypoint(scanner, out)
		if err != nil {
			return err
		}
		cfg.Spec.Agent.Type = entrypoint

		providers, err := promptProviders(scanner, out)
		if err != nil {
			return err
		}
		cfg.Spec.Providers = providers
		cfg.Spec.Sandbox.Providers = selectedProviderNames(providers)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	if err := os.WriteFile(outputPath, data, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", outputPath, err)
	}

	fmt.Fprintf(out, "Config written to %s.\nRun `harness doctor -f %s` to validate your environment, then `harness apply -f %s` to launch.\n", outputPath, outputPath, outputPath)
	return nil
}

func promptEntrypoint(scanner *bufio.Scanner, out io.Writer) (string, error) {
	fmt.Fprint(out, "Entrypoint [claude/opencode/custom] (default: claude): ")
	if !scanner.Scan() {
		return "claude", nil
	}
	input := strings.TrimSpace(scanner.Text())
	if input == "" {
		return "claude", nil
	}
	return input, nil
}

func promptProviders(scanner *bufio.Scanner, out io.Writer) ([]config.Provider, error) {
	available := defaultProviders

	fmt.Fprintln(out, "Available providers:")
	for i, p := range available {
		fmt.Fprintf(out, "  [%d] %-20s (%s)\n", i+1, p.DisplayName, p.Category)
	}

	defaults := providerDefaults(available)
	fmt.Fprintf(out, "Select (comma-separated, or 'none') [%s]: ", defaults)

	if !scanner.Scan() {
		return buildProviders(available, parseSelection(defaults, len(available))), nil
	}

	input := strings.TrimSpace(scanner.Text())
	if input == "" {
		return buildProviders(available, parseSelection(defaults, len(available))), nil
	}
	if strings.ToLower(input) == "none" {
		return nil, nil
	}

	indices := parseSelection(input, len(available))
	if len(indices) == 0 {
		return nil, fmt.Errorf("invalid provider selection: %q", input)
	}

	return buildProviders(available, indices), nil
}

func providerDefaults(available []availableProvider) string {
	var defaults []string
	for i, p := range available {
		switch p.ID {
		case "github", "google-vertex-ai":
			defaults = append(defaults, strconv.Itoa(i+1))
		}
	}
	if len(defaults) == 0 && len(available) > 0 {
		defaults = append(defaults, "1")
	}
	return strings.Join(defaults, ",")
}

func parseSelection(input string, max int) []int {
	var indices []int
	for _, part := range strings.Split(input, ",") {
		s := strings.TrimSpace(part)
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 || n > max {
			continue
		}
		indices = append(indices, n-1)
	}
	return indices
}

func buildProviders(available []availableProvider, indices []int) []config.Provider {
	var refs []config.Provider
	for _, i := range indices {
		if i < len(available) {
			refs = append(refs, config.Provider{Name: available[i].ID, Management: "referenced"})
		}
	}
	return refs
}

func selectedProviderNames(providers []config.Provider) []string {
	names := make([]string, len(providers))
	for i, provider := range providers {
		names[i] = provider.Name
	}
	return names
}
