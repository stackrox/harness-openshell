package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/robbycochran/harness-openshell/cmd"
	"github.com/robbycochran/harness-openshell/internal/status"
	"github.com/spf13/cobra"
)

var version = "dev"

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
	root.CompletionOptions.HiddenDefaultCmd = true

	root.AddCommand(
		cmd.NewUpCmd(harnessDir, cli),
		cmd.NewCreateCmd(harnessDir, cli),
		cmd.NewConnectCmd(cli),
		cmd.NewDeployCmd(harnessDir, cli),
		cmd.NewTeardownCmd(harnessDir, cli),
		cmd.NewPreflightCmd(harnessDir, cli),
		cmd.NewProvidersCmd(harnessDir, cli),
		cmd.NewLaunchCmd(harnessDir, cli),
		cmd.NewStatusCmd(harnessDir, cli),
		cmd.NewLogsCmd(harnessDir, cli),
		cmd.NewStopCmd(harnessDir, cli),
		cmd.NewStartCmd(harnessDir, cli),
	)

	if err := root.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func detectHarnessDir() string {
	if d := os.Getenv("HARNESS_DIR"); d != "" {
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
			if _, err := os.Stat(filepath.Join(dir, "agents", "default.yaml")); err == nil {
				return dir
			}
			dir = filepath.Dir(dir)
		}
	}
	return ""
}
