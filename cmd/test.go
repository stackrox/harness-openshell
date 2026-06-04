package cmd

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

func NewTestCmd(harnessDir string) *cobra.Command {
	return &cobra.Command{
		Use:   "test [podman|ocp|all] [--full]",
		Short: "End-to-end validation",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			path := filepath.Join(harnessDir, "test", "test-flow.sh")
			c := exec.Command("bash", append([]string{path}, args...)...)
			c.Stdin = os.Stdin
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			c.Dir = harnessDir
			return c.Run()
		},
	}
}
