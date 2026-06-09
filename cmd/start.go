package cmd

import (
	"fmt"

	"github.com/robbycochran/harness-openshell/internal/gateway"
	"github.com/robbycochran/harness-openshell/internal/status"
	"github.com/spf13/cobra"
)

func NewStartCmd(harnessDir, cli string) *cobra.Command {
	return &cobra.Command{
		Use:   "start [SANDBOX_NAME]",
		Short: "Start a stopped sandbox",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			gw := gateway.New(cli)
			name, err := resolveSandboxName(gw, args)
			if err != nil {
				return err
			}
			if err := gw.SandboxStart(name); err != nil {
				return fmt.Errorf("starting %s: %w", name, err)
			}
			status.OKf("Started %s", name)
			return nil
		},
	}
}
