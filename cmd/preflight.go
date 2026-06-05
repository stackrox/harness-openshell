package cmd

import (
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
			// Support subcommands: available, names
			if len(args) > 0 {
				switch args[0] {
				case "available":
					return preflight.RunAvailable(harnessDir)
				case "names":
					return preflight.RunNames(harnessDir)
				}
			}
			gw := gateway.NewCLI(cli)
			return preflight.RunCheck(harnessDir, gw, strict)
		},
	}

	cmd.Flags().BoolVar(&strict, "strict", false, "Exit 1 if required providers missing")

	return cmd
}
