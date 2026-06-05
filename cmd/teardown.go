package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/robbycochran/harness-openshell/internal/gateway"
	"github.com/robbycochran/harness-openshell/internal/k8s"
	"github.com/robbycochran/harness-openshell/internal/status"
	"github.com/spf13/cobra"
)

func NewTeardownCmd(harnessDir, cli string) *cobra.Command {
	var (
		sandboxes bool
		providers bool
		k8sFlag   bool
	)

	cmd := &cobra.Command{
		Use:   "teardown [--sandboxes] [--providers] [--k8s]",
		Short: "Tear down sandboxes, providers, or k8s resources",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !sandboxes && !providers && !k8sFlag {
				return fmt.Errorf("specify at least one of --sandboxes, --providers, or --k8s")
			}

			gw := gateway.New(cli)
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
			if k8sFlag {
				ns := k8s.DefaultNamespace()
				teardownK8s(gw, k8s.New("", ns), k8s.New("", ""))
			}

			status.Done("Done.")
			return nil
		},
	}

	cmd.Flags().BoolVar(&sandboxes, "sandboxes", false, "Delete all sandboxes")
	cmd.Flags().BoolVar(&providers, "providers", false, "Delete all providers")
	cmd.Flags().BoolVar(&k8sFlag, "k8s", false, "Delete k8s resources")

	return cmd
}

func teardownSandboxes(gw gateway.Gateway, activeGW string) {
	status.Section("Sandboxes")
	if activeGW == "" {
		status.Info("No active gateway, skipping")
		fmt.Println()
		return
	}

	names, err := gw.SandboxList()
	if err != nil {
		status.Fail(fmt.Sprintf("could not list sandboxes: %v", err))
		fmt.Println()
		return
	}
	if len(names) == 0 {
		status.Info("None running")
	} else {
		for _, name := range names {
			status.Infof("Deleting %s", name)
			if err := gw.SandboxDelete(name); err != nil {
				status.Failf("failed to delete %s: %v", name, err)
			}
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

	remaining, err := gw.SandboxList()
	if err != nil {
		return fmt.Errorf("could not check for running sandboxes: %w", err)
	}
	if len(remaining) > 0 {
		// Sandbox may be mid-deletion — wait briefly and retry
		time.Sleep(2 * time.Second)
		remaining, _ = gw.SandboxList()
		if len(remaining) > 0 {
			return fmt.Errorf("cannot delete providers with running sandboxes — run: harness teardown --sandboxes")
		}
	}

	names, err := gw.ProviderList()
	if err != nil {
		return fmt.Errorf("could not list providers: %w", err)
	}
	if len(names) == 0 {
		status.Info("None registered")
	} else {
		for _, name := range names {
			status.Infof("Deleting %s", name)
			if err := gw.ProviderDelete(name); err != nil {
				status.Failf("failed to delete %s: %v", name, err)
			}
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

func teardownK8s(gw gateway.Gateway, kc, clusterRunner k8s.Runner) {
	ctx := context.Background()
	namespace := k8s.DefaultNamespace()

	if !clusterRunner.NamespaceExists(ctx, namespace) {
		status.Section("K8s")
		status.Info("No openshell namespace found, skipping")
		return
	}

	// Helm release
	status.Section("Helm release")
	if err := kc.RunHelm(ctx, "uninstall", "openshell"); err == nil {
		status.Info("Uninstalled")
	} else {
		status.Info("Not installed")
	}

	// Sandbox CRD namespace
	fmt.Println()
	status.Section("Sandbox CRD")
	if _, err := clusterRunner.RunKubectl(ctx, "delete", "ns", "agent-sandbox-system"); err == nil {
		status.Info("Deleted agent-sandbox-system")
	} else {
		status.Info("Not found")
	}

	// OpenShift SCCs
	fmt.Println()
	status.Section("OpenShift SCCs")
	for _, sa := range []string{"openshell", "openshell-sandbox", "default"} {
		kc.RunOC(ctx, "adm", "policy", "remove-scc-from-user", "privileged", "-z", sa, "-n", namespace)
	}
	kc.RunOC(ctx, "adm", "policy", "remove-scc-from-user", "anyuid", "-z", "openshell", "-n", namespace)
	clusterRunner.RunKubectl(ctx, "delete", "clusterrolebinding", "agent-sandbox-admin")
	status.Info("Cleared")

	// Secrets
	fmt.Println()
	status.Section("K8s secrets")
	for _, secret := range []string{"openshell-gws", "openshell-atlassian"} {
		if _, err := kc.RunKubectl(ctx, "delete", "secret", secret); err == nil {
			status.OKf("Deleted %s", secret)
		} else {
			status.Infof("%s: not found", secret)
		}
	}

	// Namespace
	fmt.Println()
	status.Section("Namespace")
	if _, err := clusterRunner.RunKubectl(ctx, "delete", "ns", namespace); err == nil {
		status.OKf("Deleted %s", namespace)
	} else {
		status.Infof("%s: not found", namespace)
	}

	// Gateway config cleanup
	fmt.Println()
	status.Section("Gateway config")
	gateways, _ := gw.GatewayList()
	for _, g := range gateways {
		if !strings.Contains(g.Endpoint, "127.0.0.1") {
			if err := gw.GatewayRemove(g.Name); err == nil {
				status.OKf("Removed gateway '%s'", g.Name)
			}
		}
	}
	// Select local gateway if available
	for _, g := range gateways {
		if strings.Contains(g.Endpoint, "127.0.0.1") {
			gw.GatewaySelect(g.Name)
			break
		}
	}

	fmt.Println()
}
