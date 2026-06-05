package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/robbycochran/harness-openshell/cmd"
	"github.com/spf13/cobra"
)

func main() {
	harnessDir := detectHarnessDir()

	root := &cobra.Command{
		Use:   "harness",
		Short: "OpenShell Harness — deploy and manage AI agent sandboxes",
	}

	cli := os.Getenv("OPENSHELL_CLI")
	if cli == "" {
		cli = "openshell"
	}

	root.AddCommand(
		cmd.NewNewCmd(harnessDir, cli),
		cmd.NewConnectCmd(cli),
		cmd.NewDeployCmd(harnessDir),
		cmd.NewTeardownCmd(harnessDir),
		cmd.NewPreflightCmd(harnessDir, cli),
		cmd.NewProvidersCmd(harnessDir),
		cmd.NewTestCmd(harnessDir),
	)

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func detectHarnessDir() string {
	if d := os.Getenv("HARNESS_DIR"); d != "" {
		return d
	}
	ex, err := os.Executable()
	if err == nil {
		dir := filepath.Dir(ex)
		for range 5 {
			if _, err := os.Stat(filepath.Join(dir, "profiles", "default.toml")); err == nil {
				return dir
			}
			dir = filepath.Dir(dir)
		}
	}
	cwd, err := os.Getwd()
	if err == nil {
		dir := cwd
		for range 5 {
			if _, err := os.Stat(filepath.Join(dir, "profiles", "default.toml")); err == nil {
				return dir
			}
			dir = filepath.Dir(dir)
		}
	}
	fmt.Fprintf(os.Stderr, "WARNING: could not detect harness directory (set HARNESS_DIR)\n")
	return "."
}
