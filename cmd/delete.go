package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/stackrox/harness-openshell/internal/gateway"
	"github.com/stackrox/harness-openshell/internal/k8s"
	"github.com/stackrox/harness-openshell/internal/openshell"
	"github.com/stackrox/harness-openshell/internal/status"
)

func NewDeleteCmd(harnessDir, cli string, newClient openshell.Factory) *cobra.Command {
	var (
		all       bool
		sandboxes bool
		providers bool
		k8sFlag   bool
	)
	var gatewayName, workspace *string

	cmd := &cobra.Command{
		Use:   "delete [NAME...] [--all] [--providers] [--k8s]",
		Short: "Delete sandboxes, providers, or k8s resources",
		Long: `Delete specific sandboxes by name, or use flags for bulk operations.

Examples:
  harness delete my-sandbox          Delete a specific sandbox
  harness delete agent test          Delete multiple sandboxes
  harness delete --all               Delete all sandboxes, providers, and k8s resources
  harness delete --providers         Delete all providers (no running sandboxes allowed)
  harness delete --k8s               Delete k8s resources (helm, namespace, SCCs)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && !all && !sandboxes && !providers && !k8sFlag {
				return fmt.Errorf("specify sandbox name(s) or use --all, --sandboxes, --providers, --k8s")
			}

			ctx := cmd.Context()
			target := openshell.ResolveTarget(*gatewayName, *workspace, "", "", os.Getenv)

			// The --k8s branch is CLI/kubectl-backed and needs no SDK client;
			// open (and dial) one only when a sandbox/provider path will use it,
			// so `delete --k8s` still works when the OpenShell API is down.
			needsSDK := len(args) > 0 || all || sandboxes || providers
			var client openshell.Client
			if needsSDK {
				var err error
				client, err = newClient(ctx, target)
				if err != nil {
					return err
				}
				defer client.Close()
			}

			// Targeted sandbox deletion
			if len(args) > 0 {
				for _, name := range args {
					if err := client.DeleteSandbox(ctx, name); err != nil {
						status.Failf("%s: %v", name, err)
					} else {
						status.OKf("Deleted sandbox %s", name)
					}
				}
				if !all && !providers && !k8sFlag {
					return nil
				}
			}

			if target.Gateway != "" {
				status.Infof("Active gateway: %s", target.Gateway)
			} else {
				status.Info("Active gateway: none")
			}
			fmt.Println()

			if all || sandboxes {
				deleteSandboxesSDK(ctx, client, target.Gateway)
			}
			if all || providers {
				if err := deleteProvidersSDK(ctx, client, target.Gateway); err != nil {
					return err
				}
			}
			if all || k8sFlag {
				// internal/gateway residual: the --k8s path stays CLI-backed
				// until PR7b retires the legacy bridge. This is the only
				// sanctioned use of internal/gateway and internal/k8s in delete.
				gw := gateway.New(cli)
				ns := k8s.DefaultNamespace()
				gwCfg := resolveFirstRemoteGateway(harnessDir)
				teardownK8s(gw, gwCfg, k8s.New("", ns), k8s.New("", ""))
			}

			status.Done("Done.")
			return nil
		},
	}

	cmd.Flags().BoolVar(&all, "all", false, "Delete all sandboxes, providers, and k8s resources")
	cmd.Flags().BoolVar(&sandboxes, "sandboxes", false, "Delete all sandboxes")
	cmd.Flags().BoolVar(&providers, "providers", false, "Delete all providers")
	cmd.Flags().BoolVar(&k8sFlag, "k8s", false, "Delete k8s resources")
	gatewayName, workspace = registerTargetFlags(cmd)

	return cmd
}

// deleteSandboxesSDK sweeps every sandbox in the target workspace over the
// OpenShell SDK. It is delete's own SDK-backed sweep, intentionally mirroring
// teardownSandboxes (cmd/teardown.go) on a different backing; the duplication is
// a short-lived seam removed in PR7b when the CLI helper and teardown command
// are retired, leaving this the single owner of the sweep.
func deleteSandboxesSDK(ctx context.Context, client openshell.Client, activeGW string) {
	status.Section("Sandboxes")
	if activeGW == "" {
		status.Info("No active gateway, skipping")
		fmt.Println()
		return
	}

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

// deleteProvidersSDK sweeps every provider over the SDK, preserving the
// running-sandbox guard from teardownProviders (cmd/teardown.go): providers are
// refused while any sandbox is still up, with one brief retry to absorb a
// mid-deletion race. Like deleteSandboxesSDK this is a short-lived duplicate of
// the CLI helper, collapsed to the single owner in PR7b.
func deleteProvidersSDK(ctx context.Context, client openshell.Client, activeGW string) error {
	status.Section("Providers")
	if activeGW == "" {
		status.Info("No active gateway, skipping")
		fmt.Println()
		return nil
	}

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
