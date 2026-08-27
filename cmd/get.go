package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/stackrox/harness-openshell/internal/openshell"
)

func NewGetCmd(newClient openshell.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Display resources",
		Long:  "List sandboxes, providers, or gateways. Use -o json or -o yaml for machine-readable output.",
	}

	cmd.AddCommand(
		newGetAgentsCmd(newClient),
		newGetProvidersCmd(newClient),
		newGetGatewaysCmd(newClient),
	)

	return cmd
}

func newGetAgentsCmd(newClient openshell.Factory) *cobra.Command {
	var output string
	var gatewayName, workspace *string

	cmd := &cobra.Command{
		Use:     "agents",
		Aliases: []string{"sandboxes", "sandbox"},
		Short:   "List running sandboxes",
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := parseOutputFormat(output)
			if err != nil {
				return err
			}

			client, err := openClient(cmd.Context(), newClient, gatewayName, workspace)
			if err != nil {
				return fmt.Errorf("create OpenShell client: %w", err)
			}
			defer client.Close()

			sandboxes, err := client.Sandboxes(cmd.Context())
			if err != nil {
				return fmt.Errorf("listing sandboxes: %w", err)
			}

			if len(sandboxes) == 0 {
				if format == formatTable {
					fmt.Println("No sandboxes running.")
				} else {
					return printStructured(format, []any{})
				}
				return nil
			}

			if format != formatTable {
				type sandboxOut struct {
					Name  string `json:"name" yaml:"name"`
					Phase string `json:"phase" yaml:"phase"`
				}
				out := make([]sandboxOut, len(sandboxes))
				for i, s := range sandboxes {
					out[i] = sandboxOut{Name: s.Name, Phase: s.Phase}
				}
				return printStructured(format, out)
			}

			rows := make([][]string, len(sandboxes))
			for i, s := range sandboxes {
				rows[i] = []string{s.Name, s.Phase}
			}
			printTable([]string{"Name", "Phase"}, rows)
			return nil
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "", "Output format: table, json, or yaml")
	gatewayName, workspace = registerTargetFlags(cmd)
	return cmd
}

func newGetProvidersCmd(newClient openshell.Factory) *cobra.Command {
	var output string
	var gatewayName, workspace *string

	cmd := &cobra.Command{
		Use:     "providers",
		Aliases: []string{"provider"},
		Short:   "List registered providers",
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := parseOutputFormat(output)
			if err != nil {
				return err
			}

			client, err := openClient(cmd.Context(), newClient, gatewayName, workspace)
			if err != nil {
				return fmt.Errorf("create OpenShell client: %w", err)
			}
			defer client.Close()

			providers, err := client.Providers(cmd.Context())
			if err != nil {
				return fmt.Errorf("listing providers: %w", err)
			}

			if len(providers) == 0 {
				if format == formatTable {
					fmt.Println("No providers registered.")
				} else {
					return printStructured(format, []any{})
				}
				return nil
			}

			if format != formatTable {
				type providerOut struct {
					Name string `json:"name" yaml:"name"`
				}
				out := make([]providerOut, len(providers))
				for i, p := range providers {
					out[i] = providerOut{Name: p.Name}
				}
				return printStructured(format, out)
			}

			rows := make([][]string, len(providers))
			for i, p := range providers {
				rows[i] = []string{p.Name}
			}
			printTable([]string{"Name"}, rows)
			return nil
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "", "Output format: table, json, or yaml")
	gatewayName, workspace = registerTargetFlags(cmd)
	return cmd
}

func newGetGatewaysCmd(newClient openshell.Factory) *cobra.Command {
	var output string
	var gatewayName, workspace *string

	cmd := &cobra.Command{
		Use:     "gateways",
		Aliases: []string{"gateway", "gw"},
		Short:   "Show the active gateway",
		Long: `Show the active OpenShell gateway (name, endpoint, status, version).

The OpenShell SDK has no gateway-list RPC, so this reports the single gateway the
client is bound to (via --gateway or $OPENSHELL_GATEWAY), not every configured
registration.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := parseOutputFormat(output)
			if err != nil {
				return err
			}

			client, err := openClient(cmd.Context(), newClient, gatewayName, workspace)
			if err != nil {
				return fmt.Errorf("create OpenShell client: %w", err)
			}
			defer client.Close()

			info, err := client.GatewayInfo(cmd.Context())
			if err != nil {
				return fmt.Errorf("reading gateway info: %w", err)
			}

			if format != formatTable {
				type gwOut struct {
					Name     string `json:"name" yaml:"name"`
					Endpoint string `json:"endpoint" yaml:"endpoint"`
					Status   string `json:"status" yaml:"status"`
					Version  string `json:"version" yaml:"version"`
				}
				return printStructured(format, gwOut{
					Name:     info.Name,
					Endpoint: info.Endpoint,
					Status:   info.Status,
					Version:  info.Version,
				})
			}

			printTable(
				[]string{"Name", "Endpoint", "Status", "Version"},
				[][]string{{info.Name, info.Endpoint, info.Status, info.Version}},
			)
			return nil
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "", "Output format: table, json, or yaml")
	gatewayName, workspace = registerTargetFlags(cmd)
	return cmd
}
