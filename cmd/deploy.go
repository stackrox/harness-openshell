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
		Use:   "deploy [gateway]",
		Short: "Deploy or verify the gateway",
		Long:  "Deploy a gateway by name (e.g., local, ocp, kind). Reads configuration from gateways/<name>/gateway.yaml.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			gatewayName, err := resolveGatewayName(args, local, remote)
			if err != nil {
				return err
			}

			gwDir := filepath.Join(harnessDir, "gateways", gatewayName)
			gwCfg, err := gateway.LoadConfig(gwDir)
			if err != nil {
				return fmt.Errorf("loading gateway config %q: %w", gatewayName, err)
			}

			gw := gateway.New(cli)

			if gwCfg.IsLocal() {
				return deployLocal(gw)
			}

			kc := k8s.New(kubeconfig, k8s.DefaultNamespace())
			clusterRunner := k8s.New(kubeconfig, "")
			return deployFromConfig(harnessDir, gwCfg, gw, kc, clusterRunner)
		},
	}

	cmd.Flags().BoolVar(&local, "local", false, "Alias for 'harness deploy local'")
	cmd.Flags().BoolVar(&remote, "remote", false, "Alias for 'harness deploy ocp'")
	cmd.Flags().StringVar(&kubeconfig, "kubeconfig", "", "Path to kubeconfig (remote only)")
	cmd.Flags().MarkHidden("local")
	cmd.Flags().MarkHidden("remote")

	return cmd
}

func resolveGatewayName(args []string, local, remote bool) (string, error) {
	if len(args) > 0 {
		return args[0], nil
	}
	if local {
		return "local", nil
	}
	if remote {
		return "ocp", nil
	}
	return "", fmt.Errorf("specify a gateway: harness deploy <local|ocp|kind>")
}

// lookPath is exec.LookPath, overridable in tests to avoid a host
// dependency on podman.
var lookPath = exec.LookPath

func deployLocal(gw gateway.Gateway) error {
	cliPath := gw.CLIPath()
	if cliPath == "" {
		return fmt.Errorf("openshell CLI not found. Install it first:\n  curl -LsSf https://raw.githubusercontent.com/NVIDIA/OpenShell/main/install.sh | sh")
	}

	status.Header("Deploy")
	if _, err := lookPath("podman"); err != nil {
		status.Fail("Podman not found")
		return fmt.Errorf("podman is required")
	}
	out, _ := exec.Command("podman", "--version").Output()
	status.OKf("Podman: %s", strings.TrimSpace(string(out)))
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
		status.Detail("Install OpenShell (auto-registers the gateway):")
		status.Sub("curl -LsSf https://raw.githubusercontent.com/NVIDIA/OpenShell/main/install.sh | sh")
		return fmt.Errorf("no local gateway")
	}

	if err := gw.GatewaySelect(localGW); err != nil {
		return fmt.Errorf("selecting gateway %s: %w", localGW, err)
	}

	// Retry InferenceGet a few times: the openshell daemon can briefly reload
	// its config after a gateway add/select and take a few seconds to respond.
	var inferErr error
	for i := range 5 {
		if inferErr = gw.InferenceGet(); inferErr == nil {
			break
		}
		if i < 4 {
			time.Sleep(3 * time.Second)
		}
	}
	if inferErr == nil {
		status.OKf("%s (active, reachable)", localGW)
	} else {
		status.Failf("%s (not responding)", localGW)
		status.Detail("Start the gateway:")
		status.Sub("macOS:  brew services start openshell")
		status.Sub("Linux:  systemctl --user start openshell")
		return fmt.Errorf("gateway not responding")
	}
	return nil
}

func deployFromConfig(harnessDir string, gwCfg *gateway.GatewayConfig, gw gateway.Gateway, kc, clusterRunner k8s.Runner) (retErr error) {
	defer func() {
		if retErr != nil {
			fmt.Fprintf(os.Stderr, "\nDeploy failed. Clean up with: harness teardown --k8s\n")
		}
	}()
	ctx := context.Background()
	namespace := k8s.DefaultNamespace()

	chartVersion := os.Getenv("OPENSHELL_CHART_VERSION")
	if chartVersion == "" {
		chartVersion = gwCfg.Chart.Version
	}

	status.Header("Deploy")
	status.Infof("Chart: %s", chartVersion)
	if kbcfg := os.Getenv("KUBECONFIG"); kbcfg != "" {
		status.Infof("KUBECONFIG: %s", kbcfg)
	}

	// Step 1: Namespace
	status.Step(1, "Namespace")
	clusterRunner.RunKubectl(ctx, "create", "ns", namespace)
	if _, err := clusterRunner.RunKubectl(ctx, "label", "ns", namespace,
		"pod-security.kubernetes.io/enforce=privileged",
		"pod-security.kubernetes.io/warn=privileged",
		"--overwrite"); err != nil {
		return fmt.Errorf("labeling namespace: %w", err)
	}

	// Step 2: Sandbox CRD
	status.Step(2, "Sandbox CRD")
	if _, err := clusterRunner.RunKubectl(ctx, "apply", "-f", gwCfg.Chart.CRD.URL); err != nil {
		return fmt.Errorf("installing sandbox CRD: %w", err)
	}
	status.OK("Installed")

	// Step 3: Platform-specific setup
	if gwCfg.IsOCP() {
		status.Step(3, "OpenShift SCCs")
		for _, sa := range gwCfg.OCP.SCCPrivileged {
			kc.RunOC(ctx, "adm", "policy", "add-scc-to-user", "privileged", "-z", sa, "-n", namespace)
		}
		for _, sa := range gwCfg.OCP.SCCAnyuid {
			kc.RunOC(ctx, "adm", "policy", "add-scc-to-user", "anyuid", "-z", sa, "-n", namespace)
		}
		status.OK("Granted")
	}

	// Addon manifests (RBAC, etc.)
	for _, manifestPath := range gwCfg.ManifestPaths() {
		if _, err := kc.RunKubectl(ctx, "apply", "-f", manifestPath); err != nil {
			return fmt.Errorf("applying %s: %w", filepath.Base(manifestPath), err)
		}
	}

	// Step 4: Helm install
	status.Step(4, "Helm install")

	// routeHost is needed before Helm (for OCP PKI cert SAN).
	// gatewayURL is resolved after Helm for nodeport (service doesn't exist yet).
	var routeHost string
	if gwCfg.Gateway.Service == "route" {
		appsDomain, err := clusterRunner.GetJSONPath(ctx, "ingresses.config.openshift.io/cluster", "{.spec.domain}")
		if err != nil || appsDomain == "" {
			return fmt.Errorf("could not determine OpenShift apps domain — is this an OpenShift cluster? (kubectl get ingresses.config.openshift.io cluster)")
		}
		routeHost = fmt.Sprintf("gateway-openshell.%s", appsDomain)
	}

	helmArgs := []string{
		"upgrade", "--install", "openshell", gwCfg.Chart.OCI,
		"--version", chartVersion,
	}
	if valuesPath := gwCfg.HelmValuesPath(); valuesPath != "" {
		helmArgs = append(helmArgs, "--values", valuesPath)
	}
	if sandboxImage := os.Getenv("SANDBOX_IMAGE"); sandboxImage != "" {
		helmArgs = append(helmArgs, "--set", "server.sandboxImage="+sandboxImage)
	}
	if routeHost != "" {
		helmArgs = append(helmArgs, "--set", "pkiInitJob.serverDnsNames[0]="+routeHost)
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

	if _, err := kc.RunKubectl(ctx, "rollout", "status", "statefulset/openshell", "--timeout=300s"); err != nil {
		return fmt.Errorf("gateway rollout failed: %w", err)
	}
	status.OK("Gateway ready")

	// Step 5: CLI gateway config
	status.Step(5, "CLI gateway")
	gatewayName := gwCfg.Gateway.Name

	var gatewayURL string
	switch gwCfg.Gateway.Service {
	case "route":
		gatewayURL = fmt.Sprintf("https://%s:443", routeHost)
	case "nodeport":
		nodePort, err := kc.GetServiceNodePort(ctx, "openshell", 8080)
		if err != nil {
			return fmt.Errorf("getting NodePort: %w", err)
		}
		nodeIP, err := clusterRunner.GetNodeInternalIP(ctx)
		if err != nil {
			return fmt.Errorf("getting node IP: %w", err)
		}
		// Use HTTP — kind gateway runs with disableTls=true so the CLI
		// registers plaintext, skipping mTLS and browser auth entirely.
		gatewayURL = fmt.Sprintf("http://%s:%d", nodeIP, nodePort)
	case "loadbalancer":
		return fmt.Errorf("loadbalancer endpoint resolution not yet implemented")
	}

	existing, err := gw.GatewayList()
	if err != nil {
		return fmt.Errorf("listing existing gateways: %w", err)
	}
	for _, g := range existing {
		// Remove stale registration for same name or same route host (idempotent re-deploy).
		if g.Name == gatewayName || (routeHost != "" && strings.Contains(g.Endpoint, routeHost)) {
			gw.GatewayRemove(g.Name)
		}
	}

	if err := gw.GatewayAdd(gatewayURL, gatewayName, true, false); err != nil {
		return fmt.Errorf("registering gateway %s: %w", gatewayName, err)
	}

	// mTLS cert extraction — needed for remote clusters (OCP) where the
	// gateway is exposed via TLS-passthrough Route.
	if !gwCfg.IsLocal() && gwCfg.Secrets.MTLS != "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("determining home directory: %w", err)
		}
		mtlsDir := filepath.Join(home, ".config", "openshell", "gateways", gatewayName, "mtls")
		if err := os.MkdirAll(mtlsDir, 0o700); err != nil {
			return fmt.Errorf("creating mtls directory: %w", err)
		}
		for _, field := range []string{"ca.crt", "tls.crt", "tls.key"} {
			data, err := kc.GetSecretField(ctx, gwCfg.Secrets.MTLS, field)
			if err != nil {
				return fmt.Errorf("extracting %s from %s: %w", field, gwCfg.Secrets.MTLS, err)
			}
			if err := os.WriteFile(filepath.Join(mtlsDir, field), data, 0o600); err != nil {
				return fmt.Errorf("writing %s: %w", field, err)
			}
		}
	}

	if err := gw.GatewaySelect(gatewayName); err != nil {
		return fmt.Errorf("selecting gateway %s: %w", gatewayName, err)
	}
	if !gwCfg.IsLocal() && gwCfg.Secrets.MTLS != "" {
		status.OKf("%s registered (certs from cluster)", gatewayName)
	} else {
		status.OKf("%s registered", gatewayName)
	}

	var gwReachable bool
	for range 30 {
		if gw.InferenceGet() == nil {
			gwReachable = true
			break
		}
		time.Sleep(2 * time.Second)
	}
	if !gwReachable {
		return fmt.Errorf("gateway not reachable after 60s (try: openshell inference get)")
	}
	status.OK("Reachable")
	return nil
}
