package cmd

import (
	"github.com/robbycochran/harness-openshell/internal/runner"
	"github.com/spf13/cobra"
)

func NewPreflightCmd(harnessDir string) *cobra.Command {
	return &cobra.Command{
		Use:   "preflight [--strict]",
		Short: "Check environment prerequisites",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runner.RunScript(harnessDir, "preflight.sh", args...)
		},
	}
}
