package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/robbycochran/harness-openshell/internal/agent"
	"github.com/spf13/cobra"
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

func NewInitCmd(harnessDir string) *cobra.Command {
	var (
		outputPath     string
		force          bool
		nonInteractive bool
	)

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Generate a harness.yaml config file",
		Long: `Create a harness.yaml by selecting an entrypoint, providers, and
gateway target. The generated config is yours to version, share, and customize.

Use --non-interactive to write the embedded default config without prompts.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return initRun(os.Stdin, os.Stdout, outputPath, force, nonInteractive, DefaultAgentConfig, harnessDir)
		},
	}

	cmd.Flags().StringVarP(&outputPath, "output", "o", "harness.yaml", "Output file path")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite existing file")
	cmd.Flags().BoolVar(&nonInteractive, "non-interactive", false, "Use defaults without prompting")

	return cmd
}

func initRun(in io.Reader, out io.Writer, outputPath string, force, nonInteractive bool, defaultCfg []byte, harnessDir string) error {
	if _, err := os.Stat(outputPath); err == nil && !force {
		return fmt.Errorf("%s already exists (use --force to overwrite)", outputPath)
	}

	cfg, err := agent.Parse(defaultCfg)
	if err != nil {
		return fmt.Errorf("parsing default config: %w", err)
	}

	if !nonInteractive {
		scanner := bufio.NewScanner(in)

		entrypoint, err := promptEntrypoint(scanner, out)
		if err != nil {
			return err
		}
		cfg.Entrypoint = entrypoint

		providers, err := promptProviders(scanner, out)
		if err != nil {
			return err
		}
		cfg.Providers = providers

		target, err := promptGateway(scanner, out, harnessDir)
		if err != nil {
			return err
		}
		cfg.Gateway = target
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	if err := os.WriteFile(outputPath, data, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", outputPath, err)
	}

	fmt.Fprintf(out, "Config written to %s.\nRun `harness doctor` to validate your environment, then `harness apply -f %s` to launch.\n", outputPath, outputPath)
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

func promptProviders(scanner *bufio.Scanner, out io.Writer) ([]agent.ProviderRef, error) {
	available := discoverProviders()

	fmt.Fprintln(out, "Available providers:")
	for i, p := range available {
		fmt.Fprintf(out, "  [%d] %-20s (%s)\n", i+1, p.DisplayName, p.Category)
	}

	defaults := providerDefaults(available)
	fmt.Fprintf(out, "Select (comma-separated, or 'none') [%s]: ", defaults)

	if !scanner.Scan() {
		return buildProviderRefs(available, parseSelection(defaults, len(available))), nil
	}

	input := strings.TrimSpace(scanner.Text())
	if input == "" {
		return buildProviderRefs(available, parseSelection(defaults, len(available))), nil
	}
	if strings.ToLower(input) == "none" {
		return nil, nil
	}

	indices := parseSelection(input, len(available))
	if len(indices) == 0 {
		return nil, fmt.Errorf("invalid provider selection: %q", input)
	}

	return buildProviderRefs(available, indices), nil
}

func promptGateway(scanner *bufio.Scanner, out io.Writer, harnessDir string) (string, error) {
	profiles := listGatewayProfiles(harnessDir)
	defaultGW := "local-container"
	choices := strings.Join(profiles, "/")
	fmt.Fprintf(out, "Gateway target [%s] (default: %s): ", choices, defaultGW)
	if !scanner.Scan() {
		return defaultGW, nil
	}
	input := strings.TrimSpace(strings.ToLower(scanner.Text()))
	if input == "" {
		return defaultGW, nil
	}
	for _, p := range profiles {
		if input == p {
			return input, nil
		}
	}
	return "", fmt.Errorf("unknown gateway target: %q (available: %s)", input, choices)
}

func discoverProviders() []availableProvider {
	if providers := discoverFromOpenShell(); len(providers) > 0 {
		return providers
	}
	return defaultProviders
}

func discoverFromOpenShell() []availableProvider {
	path, err := exec.LookPath("openshell")
	if err != nil {
		return nil
	}
	out, err := exec.Command(path, "provider", "list-profiles").Output()
	if err != nil {
		return nil
	}
	return parseListProfiles(string(out))
}

func parseListProfiles(output string) []availableProvider {
	var providers []availableProvider
	var currentCategory string

	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// Category headers are indented with bold markers or all caps
		if !strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "\t") {
			continue
		}

		// Lines with 2+ spaces of indent and containing actual provider data
		// Format: "    name          Display Name           endpoints: N  category"
		fields := strings.Fields(trimmed)
		if len(fields) < 2 {
			continue
		}

		// Skip category header lines (like "INFERENCE", "AGENT", etc.)
		if len(fields) == 1 {
			currentCategory = strings.ToLower(fields[0])
			continue
		}

		// Check if this looks like a provider line (has "endpoints:" somewhere)
		epIdx := -1
		for i, f := range fields {
			if f == "endpoints:" {
				epIdx = i
				break
			}
		}
		if epIdx < 0 {
			continue
		}

		id := fields[0]
		displayName := strings.Join(fields[1:epIdx], " ")

		category := currentCategory
		if epIdx+2 < len(fields) {
			category = fields[epIdx+2]
		}

		providers = append(providers, availableProvider{
			ID:          id,
			DisplayName: displayName,
			Category:    category,
		})
	}

	return providers
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

func buildProviderRefs(available []availableProvider, indices []int) []agent.ProviderRef {
	var refs []agent.ProviderRef
	for _, i := range indices {
		if i < len(available) {
			refs = append(refs, agent.ProviderRef{Profile: available[i].ID})
		}
	}
	return refs
}
