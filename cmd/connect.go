package cmd

import (
	"github.com/robbycochran/harness-openshell/internal/runner"
	"github.com/spf13/cobra"
)

func NewConnectCmd(cli string) *cobra.Command {
	return &cobra.Command{
		Use:   "connect [SANDBOX_NAME]",
		Short: "Reconnect to a running sandbox",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cliArgs := []string{"sandbox", "connect"}
			if len(args) > 0 {
				cliArgs = append(cliArgs, args[0])
			}
			return runner.Exec(cli, cliArgs...)
		},
	}
}
