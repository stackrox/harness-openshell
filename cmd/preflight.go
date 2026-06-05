package cmd

import (
	"fmt"

	"github.com/robbycochran/harness-openshell/internal/gateway"
	"github.com/robbycochran/harness-openshell/internal/preflight"
	"github.com/spf13/cobra"
)

func NewPreflightCmd(harnessDir, cli string) *cobra.Command {
	var strict bool

	cmd := &cobra.Command{
		Use:   "preflight",
		Short: "Check environment prerequisites",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				switch args[0] {
				case "available":
					return preflight.RunAvailable(harnessDir)
				case "names":
					return preflight.RunNames(harnessDir)
				default:
					return fmt.Errorf("unknown preflight subcommand: %s (use 'available' or 'names')", args[0])
				}
			}
			gw := gateway.New(cli)
			return preflight.RunCheck(harnessDir, gw, strict)
		},
	}

	cmd.Flags().BoolVar(&strict, "strict", false, "Exit 1 if required providers missing")

	return cmd
}
