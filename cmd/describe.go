package cmd

import (
	"fmt"

	"github.com/robbycochran/harness-openshell/internal/gateway"
	"github.com/robbycochran/harness-openshell/internal/status"
	"github.com/spf13/cobra"
)

func NewDescribeCmd(harnessDir, cli string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "describe [NAME]",
		Short: "Show detailed status for a sandbox",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			gw := gateway.New(cli)

			sandboxes, err := gw.SandboxStatus()
			if err != nil {
				return fmt.Errorf("listing sandboxes: %w", err)
			}

			var found *gateway.SandboxInfo
			for i := range sandboxes {
				if sandboxes[i].Name == name {
					found = &sandboxes[i]
					break
				}
			}
			if found == nil {
				return fmt.Errorf("sandbox %q not found", name)
			}

			status.Header(found.Name)
			status.Infof("Phase: %s", found.Phase)

			gateways, err := gw.GatewayList()
			if err == nil {
				for _, g := range gateways {
					if g.Active {
						status.Infof("Gateway: %s (%s)", g.Name, g.Endpoint)
						break
					}
				}
			}

			providers, err := gw.ProviderList()
			if err == nil && len(providers) > 0 {
				status.Infof("Providers: %d registered", len(providers))
				for _, p := range providers {
					fmt.Printf("  - %s\n", p)
				}
			}

			return nil
		},
	}

	return cmd
}
