package cmd

import (
	"github.com/robbycochran/harness-openshell/internal/runner"
	"github.com/spf13/cobra"
)

func NewDeployCmd(harnessDir string) *cobra.Command {
	return &cobra.Command{
		Use:   "deploy [--local|--remote]",
		Short: "Deploy or verify the gateway",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runner.RunScript(harnessDir, "deploy.sh", args...)
		},
	}
}
