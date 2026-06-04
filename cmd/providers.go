package cmd

import (
	"github.com/robbycochran/harness-openshell/internal/runner"
	"github.com/spf13/cobra"
)

func NewProvidersCmd(harnessDir string) *cobra.Command {
	return &cobra.Command{
		Use:   "providers [--force]",
		Short: "Register providers with the gateway",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runner.RunScript(harnessDir, "providers.sh", args...)
		},
	}
}
