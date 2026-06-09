package cmd

import (
	"github.com/robbycochran/harness-openshell/internal/gateway"
	"github.com/spf13/cobra"
)

func NewLogsCmd(harnessDir, cli string) *cobra.Command {
	var follow bool

	cmd := &cobra.Command{
		Use:   "logs [SANDBOX_NAME]",
		Short: "Stream sandbox logs",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			gw := gateway.New(cli)
			name := ""
			if len(args) > 0 {
				name = args[0]
			}
			return gw.SandboxLogs(name, follow)
		},
	}

	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Follow log output")
	return cmd
}
