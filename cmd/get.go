package cmd

import (
	"fmt"

	"github.com/robbycochran/harness-openshell/internal/gateway"
	"github.com/spf13/cobra"
)

func NewGetCmd(harnessDir, cli string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Display resources",
		Long:  "List sandboxes, providers, or gateways. Use -o json or -o yaml for machine-readable output.",
	}

	cmd.AddCommand(
		newGetAgentsCmd(cli),
		newGetProvidersCmd(cli),
		newGetGatewaysCmd(cli),
	)

	return cmd
}

func newGetAgentsCmd(cli string) *cobra.Command {
	var output string

	cmd := &cobra.Command{
		Use:     "agents",
		Aliases: []string{"sandboxes", "sandbox"},
		Short:   "List running sandboxes",
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := parseOutputFormat(output)
			if err != nil {
				return err
			}

			gw := gateway.New(cli)
			sandboxes, err := gw.SandboxStatus()
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
	return cmd
}

func newGetProvidersCmd(cli string) *cobra.Command {
	var output string

	cmd := &cobra.Command{
		Use:     "providers",
		Aliases: []string{"provider"},
		Short:   "List registered providers",
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := parseOutputFormat(output)
			if err != nil {
				return err
			}

			gw := gateway.New(cli)
			providers, err := gw.ProviderList()
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
					out[i] = providerOut{Name: p}
				}
				return printStructured(format, out)
			}

			rows := make([][]string, len(providers))
			for i, p := range providers {
				rows[i] = []string{p}
			}
			printTable([]string{"Name"}, rows)
			return nil
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "", "Output format: table, json, or yaml")
	return cmd
}

func newGetGatewaysCmd(cli string) *cobra.Command {
	var output string

	cmd := &cobra.Command{
		Use:     "gateways",
		Aliases: []string{"gateway", "gw"},
		Short:   "List gateways",
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := parseOutputFormat(output)
			if err != nil {
				return err
			}

			gw := gateway.New(cli)
			gateways, err := gw.GatewayList()
			if err != nil {
				return fmt.Errorf("listing gateways: %w", err)
			}

			if len(gateways) == 0 {
				if format == formatTable {
					fmt.Println("No gateways registered.")
				} else {
					return printStructured(format, []any{})
				}
				return nil
			}

			if format != formatTable {
				type gwOut struct {
					Name     string `json:"name" yaml:"name"`
					Endpoint string `json:"endpoint" yaml:"endpoint"`
					Active   bool   `json:"active" yaml:"active"`
				}
				out := make([]gwOut, len(gateways))
				for i, g := range gateways {
					out[i] = gwOut{Name: g.Name, Endpoint: g.Endpoint, Active: g.Active}
				}
				return printStructured(format, out)
			}

			rows := make([][]string, len(gateways))
			for i, g := range gateways {
				active := ""
				if g.Active {
					active = "*"
				}
				rows[i] = []string{active + g.Name, g.Endpoint}
			}
			printTable([]string{"Name", "Endpoint"}, rows)
			return nil
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "", "Output format: table, json, or yaml")
	return cmd
}
