package cmd

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/robbycochran/harness-openshell/internal/gateway"
	"github.com/robbycochran/harness-openshell/internal/runner"
	"github.com/robbycochran/harness-openshell/internal/status"
	"github.com/spf13/cobra"
)

func NewDeployCmd(harnessDir, cli string) *cobra.Command {
	var (
		local  bool
		remote bool
	)

	cmd := &cobra.Command{
		Use:   "deploy [--local|--remote]",
		Short: "Deploy or verify the gateway",
		RunE: func(cmd *cobra.Command, args []string) error {
			if remote {
				return runner.RunScript(harnessDir, "deploy.sh", "--remote")
			}
			if local {
				gw := gateway.NewCLI(cli)
				return deployLocal(gw)
			}
			return fmt.Errorf("specify --local or --remote")
		},
	}

	cmd.Flags().BoolVar(&local, "local", false, "Verify local podman gateway")
	cmd.Flags().BoolVar(&remote, "remote", false, "Deploy to OpenShift cluster")

	return cmd
}

func deployLocal(gw gateway.Gateway) error {
	// CLI check
	cliPath := gw.CLIPath()
	if cliPath == "" {
		return fmt.Errorf("openshell CLI not found. Install it first:\n  curl -LsSf https://raw.githubusercontent.com/NVIDIA/OpenShell/main/install.sh | sh")
	}

	// Podman check
	status.Section("Container Runtime")
	podmanPath, _ := exec.LookPath("podman")
	if podmanPath == "" {
		status.Fail("Podman not found")
		return fmt.Errorf("podman is required")
	}
	cmd := exec.Command("podman", "--version")
	out, _ := cmd.Output()
	status.OKf("Podman: %s", strings.TrimSpace(string(out)))

	// Find local gateway
	status.Section("Gateway")
	gateways, err := gw.GatewayList()
	if err != nil {
		return fmt.Errorf("listing gateways: %w", err)
	}

	var localGW string
	for _, g := range gateways {
		if strings.Contains(g.Endpoint, "127.0.0.1") {
			localGW = g.Name
			break
		}
	}

	if localGW == "" {
		status.Fail("No local gateway found")
		fmt.Println()
		fmt.Println("  Install OpenShell (auto-registers the gateway):")
		fmt.Println("    curl -LsSf https://raw.githubusercontent.com/NVIDIA/OpenShell/main/install.sh | sh")
		return fmt.Errorf("no local gateway")
	}

	gw.GatewaySelect(localGW)

	if gw.InferenceGet() == nil {
		status.OKf("%s (active, reachable)", localGW)
	} else {
		status.Failf("%s (not responding)", localGW)
		fmt.Println()
		fmt.Println("  Start the gateway:")
		fmt.Println("    macOS:  brew services start openshell")
		fmt.Println("    Linux:  systemctl --user start openshell")
		return fmt.Errorf("gateway not responding")
	}

	fmt.Println()
	fmt.Println("Done.")
	return nil
}
