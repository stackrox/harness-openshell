package cmd

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/stackrox/harness-openshell/internal/openshell"
	"github.com/stackrox/harness-openshell/internal/status"
)

// NewDescribeCmd constructs the sandbox detail command.
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
				return fmt.Errorf("create OpenShell client: %w", err)
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
			var gatewayInfo openshell.GatewayInfo
			if info, err := client.GatewayInfo(cmd.Context()); err == nil {
				gatewayInfo = info
			}

			var providers []openshell.Provider
			if listedProviders, err := client.Providers(cmd.Context()); err == nil {
				providers = listedProviders
			}

			if format != formatTable {
				return printStructured(format, describeRecord(sandbox, gatewayInfo, providers))
			}

			status.Header(sandbox.Name)
			status.Infof("Phase: %s", sandbox.Phase)

			if gatewayInfo.Name != "" {
				status.Infof("Gateway: %s (%s)", gatewayInfo.Name, gatewayInfo.Endpoint)
			}

			providerIDs := providerNames(providers)
			if len(providerIDs) > 0 {
				status.Infof("Providers: %d registered", len(providerIDs))
				for _, p := range providerIDs {
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
