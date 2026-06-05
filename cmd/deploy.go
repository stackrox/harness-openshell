package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/robbycochran/harness-openshell/internal/gateway"
	"github.com/robbycochran/harness-openshell/internal/k8s"
	"github.com/robbycochran/harness-openshell/internal/preflight"
	"github.com/robbycochran/harness-openshell/internal/status"
	"github.com/spf13/cobra"
)

func NewDeployCmd(harnessDir, cli string) *cobra.Command {
	var (
		local      bool
		remote     bool
		kubeconfig string
	)

	cmd := &cobra.Command{
		Use:   "deploy [--local|--remote]",
		Short: "Deploy or verify the gateway",
		RunE: func(cmd *cobra.Command, args []string) error {
			if remote {
				gw := gateway.New(cli)
				kc := k8s.New(kubeconfig, k8s.DefaultNamespace())
				clusterRunner := k8s.New(kubeconfig, "")
				return deployRemote(harnessDir, gw, kc, clusterRunner)
			}
			if local {
				gw := gateway.New(cli)
				return deployLocal(gw)
			}
			return fmt.Errorf("specify --local or --remote")
		},
	}

	cmd.Flags().BoolVar(&local, "local", false, "Verify local podman gateway")
	cmd.Flags().BoolVar(&remote, "remote", false, "Deploy to OpenShift cluster")
	cmd.Flags().StringVar(&kubeconfig, "kubeconfig", "", "Path to kubeconfig (remote only)")

	return cmd
}

func deployRemote(harnessDir string, gw gateway.Gateway, kc, clusterRunner k8s.Runner) (retErr error) {
	defer func() {
		if retErr != nil {
			fmt.Fprintf(os.Stderr, "\nDeploy failed. Clean up with: harness teardown --k8s\n")
		}
	}()
	ctx := context.Background()
	namespace := k8s.DefaultNamespace()

	chartVersion := os.Getenv("OPENSHELL_CHART_VERSION")
	if chartVersion == "" {
		cfg, _ := preflight.LoadConfig(filepath.Join(harnessDir, "openshell.toml"))
		if cfg != nil {
			chartVersion = cfg.ChartVersion
		}
	}
	if chartVersion == "" {
		chartVersion = "0.0.55"
	}
	chartOCI := "oci://ghcr.io/nvidia/openshell/helm-chart"

	fmt.Printf("OpenShell chart: %s\n", chartVersion)
	if kbcfg := os.Getenv("KUBECONFIG"); kbcfg != "" {
		fmt.Printf("KUBECONFIG: %s\n", kbcfg)
	}
	fmt.Println()

	// Step 1: Namespace (idempotent — ignore AlreadyExists)
	status.Step(1, "Creating namespace")
	clusterRunner.RunKubectl(ctx, "create", "ns", namespace)
	if _, err := clusterRunner.RunKubectl(ctx, "label", "ns", namespace,
		"pod-security.kubernetes.io/enforce=privileged",
		"pod-security.kubernetes.io/warn=privileged",
		"--overwrite"); err != nil {
		return fmt.Errorf("labeling namespace: %w", err)
	}

	// Step 2: Sandbox CRD
	status.Step(2, "Installing Sandbox CRD")
	if err := clusterRunner.RunKubectlPassthrough(ctx, "apply", "-f",
		"https://github.com/kubernetes-sigs/agent-sandbox/releases/latest/download/manifest.yaml"); err != nil {
		return fmt.Errorf("installing sandbox CRD: %w", err)
	}

	// Step 3: OpenShift SCCs (best-effort — oc may not exist on non-OpenShift)
	status.Step(3, "Granting OpenShift SCCs")
	for _, sa := range []string{"openshell", "openshell-sandbox", "default"} {
		kc.RunOC(ctx, "adm", "policy", "add-scc-to-user", "privileged", "-z", sa, "-n", namespace)
	}
	kc.RunOC(ctx, "adm", "policy", "add-scc-to-user", "anyuid", "-z", "openshell", "-n", namespace)
	clusterRunner.RunKubectl(ctx, "create", "clusterrolebinding", "agent-sandbox-admin",
		"--clusterrole=cluster-admin",
		"--serviceaccount=agent-sandbox-system:agent-sandbox-controller")

	// RBAC for launcher (from deploy/rbac.yaml)
	if err := kc.RunKubectlPassthrough(ctx, "apply", "-f", filepath.Join(harnessDir, "deploy", "rbac.yaml")); err != nil {
		return fmt.Errorf("applying launcher RBAC: %w", err)
	}

	// Step 4: Helm install
	status.Step(4, "Deploying gateway via Helm")
	sandboxImage := os.Getenv("SANDBOX_IMAGE")
	if sandboxImage == "" {
		sandboxImage = "quay.io/rcochran/openshell:sandbox"
	}

	appsDomain, err := clusterRunner.GetJSONPath(ctx, "ingresses.config.openshift.io/cluster", "{.spec.domain}")
	if err != nil || appsDomain == "" {
		return fmt.Errorf("could not determine OpenShift apps domain — is this an OpenShift cluster? (kubectl get ingresses.config.openshift.io cluster)")
	}
	routeHost := fmt.Sprintf("gateway-openshell.%s", appsDomain)

	helmArgs := []string{
		"upgrade", "--install", "openshell", chartOCI,
		"--version", chartVersion,
		"--values", filepath.Join(harnessDir, "values-ocp.yaml"),
		"--set", "server.sandboxImage=" + sandboxImage,
		"--set", "pkiInitJob.serverDnsNames[0]=" + routeHost,
	}
	if ps := os.Getenv("PULL_SECRET"); ps != "" {
		helmArgs = append(helmArgs, "--set", "imagePullSecrets[0].name="+ps)
	}
	if sps := os.Getenv("SANDBOX_PULL_SECRET"); sps != "" {
		helmArgs = append(helmArgs, "--set", "server.sandboxImagePullSecrets[0].name="+sps)
	}
	if err := kc.RunHelm(ctx, helmArgs...); err != nil {
		return fmt.Errorf("helm install failed: %w", err)
	}

	// Wait for gateway
	status.Section("Waiting for gateway")
	if err := kc.RunKubectlPassthrough(ctx, "rollout", "status", "statefulset/openshell", "--timeout=300s"); err != nil {
		return fmt.Errorf("gateway rollout failed: %w", err)
	}

	// Step 5: Route (from deploy/route.yaml)
	status.Step(5, "Creating OpenShift route")
	if err := kc.RunKubectlQuiet(ctx, "get", "route", "gateway"); err != nil {
		kc.RunKubectlPassthrough(ctx, "apply", "-f", filepath.Join(harnessDir, "deploy", "route.yaml"))
	}
	fmt.Printf("  Route: %s\n", routeHost)

	// Step 6: CLI gateway config
	status.Step(6, "Configuring CLI gateway")
	gatewayName := os.Getenv("GATEWAY_NAME")
	if gatewayName == "" {
		gatewayName = "openshell-remote-ocp"
	}
	gatewayURL := fmt.Sprintf("https://%s:443", routeHost)

	// Remove existing gateways for this host
	existing, _ := gw.GatewayList()
	for _, g := range existing {
		if strings.Contains(g.Endpoint, routeHost) {
			gw.GatewayRemove(g.Name)
		}
	}

	if err := gw.GatewayAdd(gatewayURL, gatewayName, true); err != nil {
		return fmt.Errorf("registering gateway %s: %w", gatewayName, err)
	}

	// Extract mTLS certs
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("determining home directory: %w", err)
	}
	mtlsDir := filepath.Join(home, ".config", "openshell", "gateways", gatewayName, "mtls")
	for _, field := range []string{"ca.crt", "tls.crt", "tls.key"} {
		data, err := kc.GetSecretField(ctx, "openshell-client-tls", field)
		if err != nil {
			return fmt.Errorf("extracting %s from openshell-client-tls: %w", field, err)
		}
		if err := os.WriteFile(filepath.Join(mtlsDir, field), data, 0o600); err != nil {
			return fmt.Errorf("writing %s: %w", field, err)
		}
	}

	if err := gw.GatewaySelect(gatewayName); err != nil {
		return fmt.Errorf("selecting gateway %s: %w", gatewayName, err)
	}
	status.OKf("%s registered (certs from cluster)", gatewayName)

	// Wait for gateway to be reachable
	fmt.Print("  Waiting for gateway...")
	var gwReachable bool
	for range 30 {
		if gw.InferenceGet() == nil {
			gwReachable = true
			status.OK("reachable")
			break
		}
		time.Sleep(2 * time.Second)
		fmt.Print(".")
	}
	if !gwReachable {
		fmt.Println()
		return fmt.Errorf("gateway not reachable after 60s (try: openshell inference get)")
	}

	fmt.Println()
	status.Done("Done.")
	return nil
}

func deployLocal(gw gateway.Gateway) error {
	cliPath := gw.CLIPath()
	if cliPath == "" {
		return fmt.Errorf("openshell CLI not found. Install it first:\n  curl -LsSf https://raw.githubusercontent.com/NVIDIA/OpenShell/main/install.sh | sh")
	}

	status.Section("Container Runtime")
	podmanPath, _ := exec.LookPath("podman")
	if podmanPath == "" {
		status.Fail("Podman not found")
		return fmt.Errorf("podman is required")
	}
	cmd := exec.Command("podman", "--version")
	out, _ := cmd.Output()
	status.OKf("Podman: %s", strings.TrimSpace(string(out)))

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

	if err := gw.GatewaySelect(localGW); err != nil {
		return fmt.Errorf("selecting gateway %s: %w", localGW, err)
	}

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
	status.Done("Done.")
	return nil
}
