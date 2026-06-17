package cmd

import (
	"fmt"

	"github.com/robbycochran/harness-openshell/internal/gateway"
	"github.com/robbycochran/harness-openshell/internal/status"
	"github.com/spf13/cobra"
)

func NewDescribeCmd(harnessDir, cli string) *cobra.Command {
	var output string

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

			// Find active gateway
			var activeGW *gateway.GatewayInfo
			gateways, err := gw.GatewayList()
			if err == nil {
				for i := range gateways {
					if gateways[i].Active {
						activeGW = &gateways[i]
						break
					}
				}
			}

			// Find providers
			providers, _ := gw.ProviderList()

			format, err := parseOutputFormat(output)
			if err != nil {
				return err
			}

			if format != formatTable {
				type describeOut struct {
					Name      string   `json:"name" yaml:"name"`
					Phase     string   `json:"phase" yaml:"phase"`
					Gateway   string   `json:"gateway,omitempty" yaml:"gateway,omitempty"`
					Endpoint  string   `json:"endpoint,omitempty" yaml:"endpoint,omitempty"`
					Providers []string `json:"providers,omitempty" yaml:"providers,omitempty"`
				}
				out := describeOut{
					Name:      found.Name,
					Phase:     found.Phase,
					Providers: providers,
				}
				if activeGW != nil {
					out.Gateway = activeGW.Name
					out.Endpoint = activeGW.Endpoint
				}
				return printStructured(format, out)
			}

			status.Header(found.Name)
			status.Infof("Phase: %s", found.Phase)

			if activeGW != nil {
				status.Infof("Gateway: %s (%s)", activeGW.Name, activeGW.Endpoint)
			}

			if len(providers) > 0 {
				status.Infof("Providers: %d registered", len(providers))
				for _, p := range providers {
					fmt.Printf("  - %s\n", p)
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "", "Output format: table, json, or yaml")
	return cmd
}
