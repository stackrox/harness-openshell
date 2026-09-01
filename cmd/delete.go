package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/stackrox/harness-openshell/internal/openshell"
	"github.com/stackrox/harness-openshell/internal/status"
)

func NewDeleteCmd(newClient openshell.Factory) *cobra.Command {
	var (
		all       bool
		sandboxes bool
		providers bool
	)
	var gatewayName, workspace *string

	cmd := &cobra.Command{
		Use:   "delete [NAME...] [--all] [--sandboxes] [--providers]",
		Short: "Delete sandboxes or providers",
		Long: `Delete specific sandboxes by name, or use flags for bulk operations.

Examples:
  harness delete my-sandbox          Delete a specific sandbox
  harness delete agent test          Delete multiple sandboxes
  harness delete --all               Delete all sandboxes and providers
  harness delete --sandboxes         Delete all sandboxes
  harness delete --providers         Delete all providers (no running sandboxes allowed)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && !all && !sandboxes && !providers {
				return fmt.Errorf("specify sandbox name(s) or use --all, --sandboxes, --providers")
			}

			ctx := cmd.Context()
			target := openshell.ResolveTarget(*gatewayName, *workspace, "", "", os.Getenv)
			// When neither --gateway nor $OPENSHELL_GATEWAY pins a gateway, the
			// SDK resolves the active gateway from openshell config
			// (gateway.LoadConfig("") in sdkclient.New), surfacing
			// ErrNoActiveGateway if none is selected — the same selection apply
			// runs against.
			client, err := newClient(ctx, target)
			if err != nil {
				return fmt.Errorf("create OpenShell client: %w", err)
			}
			defer client.Close()

			// Targeted sandbox deletion
			if len(args) > 0 {
				for _, name := range args {
					if err := client.DeleteSandbox(ctx, name); err != nil {
						status.Failf("%s: %v", name, err)
					} else {
						status.OKf("Deleted sandbox %s", name)
					}
				}
				if !all && !providers {
					return nil
				}
			}

			// Banner the gateway we're sweeping. target.Gateway is empty when
			// resolved from the active-gateway marker by the SDK, so ask the
			// client for the resolved name (GatewayInfo carries what
			// LoadConfig resolved). Best-effort: a banner must never block the
			// sweep, so fall back to the pinned name on any error.
			if gwName := resolvedGatewayName(ctx, client, target); gwName != "" {
				status.Infof("Active gateway: %s", gwName)
				fmt.Println()
			}

			if all || sandboxes {
				deleteSandboxesSDK(ctx, client)
			}
			if all || providers {
				if err := deleteProvidersSDK(ctx, client); err != nil {
					return err
				}
			}

			status.Done("Done.")
			return nil
		},
	}

	cmd.Flags().BoolVar(&all, "all", false, "Delete all sandboxes and providers")
	cmd.Flags().BoolVar(&sandboxes, "sandboxes", false, "Delete all sandboxes")
	cmd.Flags().BoolVar(&providers, "providers", false, "Delete all providers")
	gatewayName, workspace = registerTargetFlags(cmd)

	return cmd
}

// resolvedGatewayName reports the gateway name to banner. When --gateway or
// $OPENSHELL_GATEWAY pinned one, target.Gateway holds it directly; otherwise the
// SDK resolved the active gateway and only the client knows the resolved name
// (via GatewayInfo). It is best-effort — the banner is cosmetic, so a failed
// GatewayInfo lookup yields "" and the caller simply skips the banner.
func resolvedGatewayName(ctx context.Context, client openshell.Client, target openshell.Target) string {
	if target.Gateway != "" {
		return target.Gateway
	}
	if info, err := client.GatewayInfo(ctx); err == nil {
		return info.Name
	}
	return ""
}

// deleteSandboxesSDK sweeps every sandbox in the target workspace over the
// OpenShell SDK. It is the sole owner of the bulk sandbox sweep. The caller has
// already resolved a non-empty gateway, so there is no no-gateway case here.
func deleteSandboxesSDK(ctx context.Context, client openshell.Client) {
	status.Section("Sandboxes")
	sandboxes, err := client.Sandboxes(ctx)
	if err != nil {
		status.Fail(fmt.Sprintf("could not list sandboxes: %v", err))
		fmt.Println()
		return
	}
	if len(sandboxes) == 0 {
		status.Info("None running")
	} else {
		for _, s := range sandboxes {
			status.Infof("Deleting %s", s.Name)
			if err := client.DeleteSandbox(ctx, s.Name); err != nil {
				status.Failf("failed to delete %s: %v", s.Name, err)
			}
		}
	}
	fmt.Println()
}

// deleteProvidersSDK sweeps every provider over the SDK. Providers are refused
// while any sandbox is still up, with one brief retry to absorb a mid-deletion
// race. It is the sole owner of the bulk provider sweep. The caller has already
// resolved a non-empty gateway, so there is no no-gateway case here.
func deleteProvidersSDK(ctx context.Context, client openshell.Client) error {
	status.Section("Providers")
	remaining, err := client.Sandboxes(ctx)
	if err != nil {
		return fmt.Errorf("could not check for running sandboxes: %w", err)
	}
	if len(remaining) > 0 {
		// Sandbox may be mid-deletion — wait briefly and retry.
		time.Sleep(2 * time.Second)
		remaining, err = client.Sandboxes(ctx)
		if err != nil {
			return fmt.Errorf("rechecking sandboxes: %w", err)
		}
		if len(remaining) > 0 {
			return fmt.Errorf("cannot delete providers with running sandboxes — run: harness delete --sandboxes")
		}
	}

	providers, err := client.Providers(ctx)
	if err != nil {
		return fmt.Errorf("could not list providers: %w", err)
	}
	if len(providers) == 0 {
		status.Info("None registered")
	} else {
		for _, p := range providers {
			status.Infof("Deleting %s", p.Name)
			if err := client.DeleteProvider(ctx, p.Name); err != nil {
				status.Failf("failed to delete %s: %v", p.Name, err)
			}
		}
	}

	fmt.Println()
	return nil
}
