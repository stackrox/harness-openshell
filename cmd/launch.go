package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/robbycochran/harness-openshell/internal/agent"
	"github.com/spf13/cobra"
)

func NewLaunchCmd(harnessDir, cli string) *cobra.Command {
	return &cobra.Command{
		Use:    "launch",
		Short:  "Run in-cluster: render agent config into a sandbox",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLaunch(cli)
		},
	}
}

func runLaunch(cli string) error {
	endpoint := os.Getenv("GATEWAY_ENDPOINT")
	if endpoint == "" {
		endpoint = "https://openshell.openshell.svc.cluster.local:8080"
	}
	if cli == "" {
		cli = "openshell"
	}

	if err := configureGateway(endpoint, "/secrets/mtls", cli); err != nil {
		return fmt.Errorf("gateway config: %w", err)
	}

	agentCfg, err := agent.ParseFile("/etc/openshell/sandbox/agent.yaml")
	if err != nil {
		return err
	}

	agentCfg.Image = resolveSandboxImage(agentCfg.Image)

	fmt.Println("=== Sandbox Runner ===")
	fmt.Printf("  Name:       %s\n", agentCfg.Name)
	fmt.Printf("  Image:      %s\n", agentCfg.Image)
	fmt.Printf("  Entrypoint: %s\n", agentCfg.EffectiveEntrypoint())
	fmt.Printf("  Gateway:    %s\n", endpoint)
	fmt.Println()

	providerNames := agentCfg.ProviderNames()
	registered := checkProviders(providerNames, cli)

	payloadDir := "/tmp/openshell-staging/openshell"
	if err := agent.RenderPayload(agentCfg, "/etc/openshell/sandbox", payloadDir); err != nil {
		return fmt.Errorf("rendering payload: %w", err)
	}
	fmt.Printf("  Payload: %s\n", payloadDir)

	if err := launchCreateSandbox(agentCfg, registered, payloadDir, cli); err != nil {
		return err
	}

	fmt.Printf("\nSandbox '%s' is ready.\n", agentCfg.Name)
	fmt.Printf("Connect with: openshell sandbox connect %s\n", agentCfg.Name)
	return nil
}

func configureGateway(endpoint, mtlsDir, cli string) error {
	requiredCerts := []string{"ca.crt", "tls.crt", "tls.key"}
	var missing []string
	for _, name := range requiredCerts {
		if _, err := os.Stat(filepath.Join(mtlsDir, name)); err != nil {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		fmt.Fprintf(os.Stderr, "WARNING: mTLS certs missing from %s: %v\n", mtlsDir, missing)
		fmt.Fprintf(os.Stderr, "WARNING: falling back to INSECURE mode\n")
		os.Setenv("OPENSHELL_GATEWAY_ENDPOINT", endpoint)
		os.Setenv("OPENSHELL_GATEWAY_INSECURE", "true")
		return nil
	}
	fmt.Println("  mTLS certs found")

	httpEndpoint := strings.Replace(endpoint, "https:", "http:", 1)

	cmd := exec.Command(cli, "gateway", "add", httpEndpoint, "--name", "openshell")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stdout
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("gateway add: %w", err)
	}

	home := os.Getenv("HOME")
	gwDir := filepath.Join(home, ".config", "openshell", "gateways", "openshell")
	mtlsDest := filepath.Join(gwDir, "mtls")
	if err := os.MkdirAll(mtlsDest, 0o700); err != nil {
		return fmt.Errorf("creating mtls dir: %w", err)
	}

	metaPath := filepath.Join(gwDir, "metadata.json")
	var meta map[string]any
	if data, err := os.ReadFile(metaPath); err == nil {
		if err := json.Unmarshal(data, &meta); err != nil {
			return fmt.Errorf("parsing metadata.json: %w", err)
		}
	}
	if meta == nil {
		meta = make(map[string]any)
	}
	meta["gateway_endpoint"] = endpoint
	meta["auth_mode"] = "mtls"
	out, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshaling metadata.json: %w", err)
	}
	if err := os.WriteFile(metaPath, out, 0o644); err != nil {
		return fmt.Errorf("writing metadata.json: %w", err)
	}

	for _, name := range []string{"ca.crt", "tls.crt", "tls.key"} {
		if err := copyFile(filepath.Join(mtlsDir, name), filepath.Join(mtlsDest, name)); err != nil {
			return fmt.Errorf("copying %s: %w", name, err)
		}
	}

	selectCmd := exec.Command(cli, "gateway", "select", "openshell")
	selectCmd.Stdout = os.Stdout
	selectCmd.Stderr = io.Discard
	selectCmd.Run()
	fmt.Println("  mTLS gateway configured")
	return nil
}

func checkProviders(providers []string, cli string) []string {
	var registered []string
	for _, name := range providers {
		cmd := exec.Command(cli, "provider", "get", name)
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
		if cmd.Run() == nil {
			registered = append(registered, name)
			fmt.Printf("  Provider %s: attached\n", name)
		} else {
			fmt.Printf("  Provider %s: not registered (skipping)\n", name)
		}
	}
	return registered
}

func launchCreateSandbox(cfg *agent.AgentConfig, providers []string, payloadDir, cli string) error {
	fmt.Println("\n=== Creating sandbox ===")
	envInit := ". /sandbox/.config/openshell/sandbox.env 2>/dev/null && " +
		"cat /sandbox/.config/openshell/sandbox.env >> /sandbox/.bashrc 2>/dev/null; " +
		"gh auth setup-git 2>/dev/null; true"
	for attempt := 1; attempt <= 5; attempt++ {
		args := []string{"sandbox", "create", "--name", cfg.Name, "--no-tty"}
		if cfg.Image != "" {
			args = append(args, "--from", cfg.Image)
		}
		for _, p := range providers {
			args = append(args, "--provider", p)
		}
		args = append(args, "--upload", payloadDir+":/sandbox/.config", "--no-git-ignore")
		args = append(args, "--", "bash", "-c", envInit)

		cmd := exec.Command(cli, args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if cmd.Run() == nil {
			return nil
		}

		fmt.Printf("  Attempt %d failed, retrying in 10s...\n", attempt)
		del := exec.Command(cli, "sandbox", "delete", cfg.Name)
		del.Stdout = io.Discard
		del.Stderr = io.Discard
		del.Run()

		if attempt == 5 {
			return fmt.Errorf("failed after 5 attempts")
		}
		time.Sleep(10 * time.Second)
	}
	return nil // unreachable but required by compiler
}


func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err = io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
