package cmd

import (
	"github.com/robbycochran/harness-openshell/internal/runner"
	"github.com/spf13/cobra"
)

func NewTeardownCmd(harnessDir string) *cobra.Command {
	return &cobra.Command{
		Use:   "teardown [--sandboxes] [--providers] [--k8s]",
		Short: "Tear down sandboxes, providers, or k8s resources",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runner.RunScript(harnessDir, "teardown.sh", args...)
		},
	}
}
