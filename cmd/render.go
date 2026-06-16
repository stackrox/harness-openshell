package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/robbycochran/harness-openshell/internal/agent"
	"github.com/robbycochran/harness-openshell/internal/status"
	"github.com/spf13/cobra"
)

func NewRenderCmd(harnessDir, cli string) *cobra.Command {
	var (
		agentName       string
		agentProfile    string
		outputFile      string
		includeDefaults bool
	)

	cmd := &cobra.Command{
		Use:   "render [flags]",
		Short: "Render a complete harness YAML from an agent config",
		Long:  "Reads an agent config and its referenced providers/gateways, then outputs a single multi-document YAML with all definitions included. Built-in OpenShell provider profiles are labeled separately from custom ones.",
		RunE: func(cmd *cobra.Command, args []string) error {
			h, err := resolveHarness(harnessDir, agentName, agentProfile)
			if err != nil {
				return err
			}

			// Collect built-in provider profiles from the profiles/providers/ directory
			builtinProviders := loadProviderProfiles(harnessDir)

			// Collect gateway profiles referenced by the agent
			gwName := h.Agent.Gateway
			if gwName == "" && includeDefaults {
				gwName = "local"
			}
			if gwName != "" && len(h.Gateways) == 0 {
				gwData := loadGatewayProfile(harnessDir, gwName)
				if gwData != nil {
					h.Gateways[gwName] = gwData
				}
			}

			out, err := agent.RenderHarness(h, builtinProviders)
			if err != nil {
				return fmt.Errorf("rendering harness: %w", err)
			}

			if outputFile != "" {
				if err := os.WriteFile(outputFile, out, 0o644); err != nil {
					return fmt.Errorf("writing output: %w", err)
				}
				status.OKf("Rendered to %s", outputFile)
			} else {
				fmt.Print(string(out))
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&agentName, "agent", "default", "Agent config name")
	cmd.Flags().StringVarP(&agentProfile, "agent-profile", "f", "", "Path to agent YAML file")
	cmd.Flags().StringVarP(&outputFile, "output", "o", "", "Write to file instead of stdout")
	cmd.Flags().BoolVar(&includeDefaults, "include-defaults", false, "Include default gateway even if not set in agent config")

	return cmd
}

func loadProviderProfiles(harnessDir string) map[string][]byte {
	profiles := make(map[string][]byte)
	dir := filepath.Join(harnessDir, "profiles", "providers")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return profiles
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		name := e.Name()[:len(e.Name())-5] // strip .yaml
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err == nil {
			profiles[name] = data
		}
	}
	return profiles
}

func loadGatewayProfile(harnessDir, name string) []byte {
	// Try profiles/gateways/<name>.yaml
	path := filepath.Join(harnessDir, "profiles", "gateways", name+".yaml")
	data, err := os.ReadFile(path)
	if err == nil {
		return data
	}
	// Try embedded
	if d, ok := EmbeddedGatewayProfiles[name]; ok {
		return d
	}
	return nil
}
