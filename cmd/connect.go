package cmd

import (
	"github.com/robbycochran/harness-openshell/internal/gateway"
	"github.com/spf13/cobra"
)

func NewConnectCmd(cli string) *cobra.Command {
	return &cobra.Command{
		Use:   "connect [SANDBOX_NAME]",
		Short: "Reconnect to a running sandbox",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			gw := gateway.NewCLI(cli)
			name := ""
			if len(args) > 0 {
				name = args[0]
			}
			return gw.SandboxConnect(name)
		},
	}
}
