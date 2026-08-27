package cmd

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/stackrox/harness-openshell/internal/openshell"
	"github.com/stackrox/harness-openshell/internal/status"
)

func NewDescribeCmd(newClient openshell.Factory) *cobra.Command {
	var output string
	var gatewayName, workspace *string

	cmd := &cobra.Command{
		Use:   "describe [NAME]",
		Short: "Show detailed status for a sandbox",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			format, err := parseOutputFormat(output)
			if err != nil {
				return err
			}

			client, err := openClient(cmd.Context(), newClient, gatewayName, workspace)
			if err != nil {
				return err
			}
			defer client.Close()

			sandbox, err := client.GetSandbox(cmd.Context(), name)
			if err != nil {
				if errors.Is(err, openshell.ErrNotFound) {
					return fmt.Errorf("sandbox %q not found", name)
				}
				return fmt.Errorf("reading sandbox: %w", err)
			}

			// Gateway context and providers are best-effort: a describe still
			// shows the sandbox even if gateway introspection or the provider
			// list fails (behavior-preserving with the former CLI path).
			var gwName, gwEndpoint string
			if info, err := client.GatewayInfo(cmd.Context()); err == nil {
				gwName = info.Name
				gwEndpoint = info.Endpoint
			}

			var providerNames []string
			if providers, err := client.Providers(cmd.Context()); err == nil {
				providerNames = make([]string, len(providers))
				for i, p := range providers {
					providerNames[i] = p.Name
				}
			}

			if format != formatTable {
				type describeOut struct {
					Name      string   `json:"name" yaml:"name"`
					Phase     string   `json:"phase" yaml:"phase"`
					Gateway   string   `json:"gateway,omitempty" yaml:"gateway,omitempty"`
					Endpoint  string   `json:"endpoint,omitempty" yaml:"endpoint,omitempty"`
					Providers []string `json:"providers,omitempty" yaml:"providers,omitempty"`
				}
				return printStructured(format, describeOut{
					Name:      sandbox.Name,
					Phase:     sandbox.Phase,
					Gateway:   gwName,
					Endpoint:  gwEndpoint,
					Providers: providerNames,
				})
			}

			status.Header(sandbox.Name)
			status.Infof("Phase: %s", sandbox.Phase)

			if gwName != "" {
				status.Infof("Gateway: %s (%s)", gwName, gwEndpoint)
			}

			if len(providerNames) > 0 {
				status.Infof("Providers: %d registered", len(providerNames))
				for _, p := range providerNames {
					fmt.Printf("  - %s\n", p)
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "", "Output format: table, json, or yaml")
	gatewayName, workspace = registerTargetFlags(cmd)
	return cmd
}
