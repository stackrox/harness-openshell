package main

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/stackrox/harness-openshell/cmd"
	"github.com/stackrox/harness-openshell/internal/openshell/sdkclient"
	"github.com/stackrox/harness-openshell/internal/status"
)

var version = "dev"

//go:embed profiles/harness-basic.yaml
var defaultHarnessConfig []byte

func main() {
	harnessDir := detectHarnessDir()
	var verbose, showCommands bool

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
	root.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Show external commands")
	root.PersistentFlags().BoolVar(&showCommands, "show-commands", false, "Show external commands on stdout")

	cli := os.Getenv("OPENSHELL_CLI")
	if cli == "" {
		cli = "openshell"
	}

	cmd.Version = version
	root.CompletionOptions.HiddenDefaultCmd = true

	root.AddCommand(
		cmd.NewApplyCmd(sdkclient.New),
		cmd.NewGetCmd(sdkclient.New),
		cmd.NewDescribeCmd(sdkclient.New),
		cmd.NewDeleteCmd(sdkclient.New),
		cmd.NewDoctorCmd(harnessDir, cli, defaultHarnessConfig, sdkclient.New),
		cmd.NewInitCmd(defaultHarnessConfig),
		cmd.NewPlanCmd(harnessDir, sdkclient.New),
	)

	if err := root.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func detectHarnessDir() string {
	if d := os.Getenv("HARNESS_PROFILE_DIR"); d != "" {
		return d
	}
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
			if _, err := os.Stat(filepath.Join(dir, "harness-basic.yaml")); err == nil {
				return dir
			}
			if _, err := os.Stat(filepath.Join(dir, "profiles", "harness-basic.yaml")); err == nil {
				return dir
			}
			dir = filepath.Dir(dir)
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		d := filepath.Join(home, ".config", "harness-openshell")
		os.MkdirAll(filepath.Join(d, "profiles", "providers"), 0o755)
		return d
	}
	return ""
}
