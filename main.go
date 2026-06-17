package main

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/robbycochran/harness-openshell/cmd"
	"github.com/robbycochran/harness-openshell/internal/status"
	"github.com/spf13/cobra"
)

var version = "dev"

//go:embed profiles/agent-basic.yaml
var defaultAgentConfig []byte

//go:embed profiles/gateways/local.yaml
var localGatewayProfile []byte

//go:embed profiles/gateways/kind.yaml
var kindGatewayProfile []byte

//go:embed profiles/gateways/ocp.yaml
var ocpGatewayProfile []byte

func main() {
	harnessDir := detectHarnessDir()

	var verbose bool
	var showCommands bool

	root := &cobra.Command{
		Use:           "harness",
		Short:         "OpenShell Harness — deploy and manage AI agent sandboxes",
		Version:       version,
		SilenceErrors: true,
		SilenceUsage:  true,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			status.Verbose = verbose
			status.ShowCommands = showCommands
		},
	}

	root.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Show kubectl/helm/openshell commands")
	root.PersistentFlags().BoolVar(&showCommands, "show-commands", false, "Show openshell commands being executed")

	cli := os.Getenv("OPENSHELL_CLI")
	if cli == "" {
		cli = "openshell"
	}

	cmd.Version = version
	cmd.DefaultAgentConfig = defaultAgentConfig
	cmd.EmbeddedGatewayProfiles = map[string][]byte{
		"local": localGatewayProfile,
		"kind":  kindGatewayProfile,
		"ocp":   ocpGatewayProfile,
	}
	root.CompletionOptions.HiddenDefaultCmd = true

	root.AddCommand(
		cmd.NewApplyCmd(harnessDir, cli),
		cmd.NewGetCmd(harnessDir, cli),
		cmd.NewDescribeCmd(harnessDir, cli),
		cmd.NewDeleteCmd(harnessDir, cli),
		cmd.NewDeployCmd(harnessDir, cli),
		cmd.NewStopCmd(harnessDir, cli),
		cmd.NewStartCmd(harnessDir, cli),
	)

	// Deprecated aliases
	teardownCmd := cmd.NewTeardownCmd(harnessDir, cli)
	teardownCmd.Hidden = true
	teardownCmd.Deprecated = "use 'harness delete' instead"
	statusCmd := cmd.NewStatusCmd(harnessDir, cli)
	statusCmd.Hidden = true
	statusCmd.Deprecated = "use 'harness get agents' instead"
	root.AddCommand(teardownCmd, statusCmd)

	if err := root.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func detectHarnessDir() string {
	if d := os.Getenv("HARNESS_OS_DIR"); d != "" {
		return d
	}
	var roots []string
	if ex, err := os.Executable(); err == nil {
		roots = append(roots, filepath.Dir(ex))
	}
	if cwd, err := os.Getwd(); err == nil {
		roots = append(roots, cwd)
	}
	for _, root := range roots {
		dir := root
		for range 5 {
			if _, err := os.Stat(filepath.Join(dir, "agent-default.yaml")); err == nil {
				return dir
			}
			if _, err := os.Stat(filepath.Join(dir, "profiles", "agent-default.yaml")); err == nil {
				return dir
			}
			dir = filepath.Dir(dir)
		}
	}
	return ""
}
