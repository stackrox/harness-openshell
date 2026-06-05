package cmd

import (
	"fmt"

	"github.com/robbycochran/harness-openshell/internal/gateway"
	"github.com/robbycochran/harness-openshell/internal/runner"
	"github.com/robbycochran/harness-openshell/internal/status"
	"github.com/spf13/cobra"
)

func NewTeardownCmd(harnessDir, cli string) *cobra.Command {
	var (
		sandboxes bool
		providers bool
		k8s       bool
	)

	cmd := &cobra.Command{
		Use:   "teardown [--sandboxes] [--providers] [--k8s]",
		Short: "Tear down sandboxes, providers, or k8s resources",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Default: all flags true
			if !sandboxes && !providers && !k8s {
				sandboxes = true
				providers = true
				k8s = true
			}

			gw := gateway.NewCLI(cli)
			activeGW := gw.ActiveGateway()

			if activeGW != "" {
				fmt.Printf("Active gateway: %s\n", activeGW)
			} else {
				fmt.Println("Active gateway: none")
			}
			fmt.Println()

			if sandboxes {
				teardownSandboxes(gw, activeGW)
			}
			if providers {
				if err := teardownProviders(gw, activeGW); err != nil {
					return err
				}
			}
			if k8s {
				return teardownK8s(harnessDir)
			}

			fmt.Println("Done.")
			return nil
		},
	}

	cmd.Flags().BoolVar(&sandboxes, "sandboxes", false, "Delete all sandboxes")
	cmd.Flags().BoolVar(&providers, "providers", false, "Delete all providers")
	cmd.Flags().BoolVar(&k8s, "k8s", false, "Delete k8s resources")

	return cmd
}

func teardownSandboxes(gw gateway.Gateway, activeGW string) {
	status.Section("Sandboxes")
	if activeGW == "" {
		status.Info("No active gateway, skipping")
		fmt.Println()
		return
	}

	names, _ := gw.SandboxList()
	if len(names) == 0 {
		status.Info("None running")
	} else {
		for _, name := range names {
			fmt.Printf("  Deleting %s\n", name)
			gw.SandboxDelete(name)
		}
	}
	fmt.Println()
}

func teardownProviders(gw gateway.Gateway, activeGW string) error {
	status.Section("Providers")
	if activeGW == "" {
		status.Info("No active gateway, skipping")
		fmt.Println()
		return nil
	}

	remaining, _ := gw.SandboxList()
	if len(remaining) > 0 {
		return fmt.Errorf("cannot delete providers with running sandboxes — run: harness teardown --sandboxes")
	}

	names, _ := gw.ProviderList()
	if len(names) == 0 {
		status.Info("None registered")
	} else {
		for _, name := range names {
			fmt.Printf("  Deleting %s\n", name)
			gw.ProviderDelete(name)
		}
	}

	status.Section("Inference")
	if gw.InferenceRemove() == nil {
		status.Info("Cleared")
	} else {
		status.Info("Already cleared")
	}
	fmt.Println()
	return nil
}

func teardownK8s(harnessDir string) error {
	return runner.RunScript(harnessDir, "teardown.sh", "--k8s")
}
