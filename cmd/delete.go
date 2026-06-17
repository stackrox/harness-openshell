package cmd

import (
	"fmt"

	"github.com/robbycochran/harness-openshell/internal/gateway"
	"github.com/robbycochran/harness-openshell/internal/k8s"
	"github.com/robbycochran/harness-openshell/internal/status"
	"github.com/spf13/cobra"
)

func NewDeleteCmd(harnessDir, cli string) *cobra.Command {
	var (
		all       bool
		sandboxes bool
		providers bool
		k8sFlag   bool
	)

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

			gw := gateway.New(cli)

			// Targeted sandbox deletion
			if len(args) > 0 {
				for _, name := range args {
					if err := gw.SandboxDelete(name); err != nil {
						status.Failf("%s: %v", name, err)
					} else {
						status.OKf("Deleted sandbox %s", name)
					}
				}
				if !all && !providers && !k8sFlag {
					return nil
				}
			}

			activeGW := gw.ActiveGateway()
			if activeGW != "" {
				status.Infof("Active gateway: %s", activeGW)
			} else {
				status.Info("Active gateway: none")
			}
			fmt.Println()

			if all || sandboxes {
				teardownSandboxes(gw, activeGW)
			}
			if all || providers {
				if err := teardownProviders(gw, activeGW); err != nil {
					return err
				}
			}
			if all || k8sFlag {
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

	return cmd
}
