package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Name      string            `toml:"name"`
	Image     string            `toml:"image"`
	Command   string            `toml:"command"`
	Keep      *bool             `toml:"keep"`
	Providers []string          `toml:"providers"`
	Env       map[string]string `toml:"env"`
}

func (c *Config) KeepSandbox() bool {
	if c.Keep == nil {
		return true
	}
	return *c.Keep
}

func parseConfig(path string) (*Config, error) {
	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if cfg.Name == "" {
		cfg.Name = "agent"
	}
	if cfg.Command == "" {
		cfg.Command = "claude --bare"
	}
	return &cfg, nil
}

func configureGateway(endpoint, mtlsDir, cli string) error {
	certFile := filepath.Join(mtlsDir, "tls.crt")
	if _, err := os.Stat(certFile); err != nil {
		fmt.Println("  No mTLS certs, using insecure mode")
		os.Setenv("OPENSHELL_GATEWAY_ENDPOINT", endpoint)
		os.Setenv("OPENSHELL_GATEWAY_INSECURE", "true")
		return nil
	}

	httpEndpoint := strings.Replace(endpoint, "https:", "http:", 1)

	// Register via http:// (skips cert validation probe), then patch to https + mTLS.
	// Let the CLI create the full metadata.json with all required fields.
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

	// Patch metadata.json — read what gateway add created, update only the
	// two fields we need. Preserve all other fields the CLI wrote.
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

	run(cli, "gateway", "select", "openshell")
	fmt.Println("  ✓ mTLS gateway configured")
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

var stageFilesFrom = "/etc/openshell/env/sandbox.env"

func stageFiles(cfg *Config, gwsDir, harnessDir string) error {
	if err := os.MkdirAll(harnessDir, 0o755); err != nil {
		return err
	}

	if envPath := stageFilesFrom; fileExists(envPath) {
		if err := copyFile(envPath, filepath.Join(harnessDir, "sandbox.env")); err != nil {
			return fmt.Errorf("copying sandbox.env: %w", err)
		}
		data, _ := os.ReadFile(envPath)
		lines := strings.Count(string(data), "\n")
		fmt.Printf("  Env: %d vars\n", lines)
	}

	if fileExists(filepath.Join(gwsDir, "credentials.json")) {
		entries, err := os.ReadDir(gwsDir)
		if err != nil {
			return fmt.Errorf("reading gws dir: %w", err)
		}
		for _, e := range entries {
			if !e.IsDir() && !strings.HasPrefix(e.Name(), "..") {
				src := filepath.Join(gwsDir, e.Name())
				dst := filepath.Join(harnessDir, e.Name())
				if err := copyFile(src, dst); err != nil {
					return fmt.Errorf("copying %s: %w", e.Name(), err)
				}
			}
		}
		fmt.Println("  GWS: staged")
	} else {
		fmt.Println("  GWS: not mounted (skipping)")
	}
	return nil
}

func createSandbox(cfg *Config, providers []string, cli string) error {
	fmt.Println("\n=== Creating sandbox ===")
	for attempt := 1; attempt <= 5; attempt++ {
		args := []string{"sandbox", "create", "--name", cfg.Name, "--no-tty"}
		if cfg.Image != "" {
			args = append(args, "--from", cfg.Image)
		}
		for _, p := range providers {
			args = append(args, "--provider", p)
		}
		if !cfg.KeepSandbox() {
			args = append(args, "--no-keep")
		}
		args = append(args, "--", "true")

		cmd := exec.Command(cli, args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if cmd.Run() == nil {
			return nil
		}

		fmt.Printf("  Attempt %d failed (supervisor race), retrying in 10s...\n", attempt)
		del := exec.Command(cli, "sandbox", "delete", cfg.Name)
		del.Stdout = io.Discard
		del.Stderr = io.Discard
		del.Run()

		if attempt == 5 {
			return fmt.Errorf("failed after 5 attempts")
		}
		time.Sleep(10 * time.Second)
	}
	return nil
}

func uploadFiles(name, harnessDir, cli string) error {
	fmt.Println("  Uploading to /sandbox/.config/openshell/...")
	cmd := exec.Command(cli, "sandbox", "upload", name, harnessDir, "/sandbox/.config", "--no-git-ignore")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func runStartup(name, cli string) error {
	fmt.Println("  Running startup...")
	cmd := exec.Command(cli, "sandbox", "exec", "--name", name, "--", "bash", "/sandbox/startup.sh")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func run(name string, args ...string) {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = io.Discard
	cmd.Run()
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

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func main() {
	endpoint := os.Getenv("GATEWAY_ENDPOINT")
	if endpoint == "" {
		endpoint = "https://openshell.openshell.svc.cluster.local:8080"
	}
	cli := os.Getenv("OPENSHELL_CLI")
	if cli == "" {
		cli = "openshell"
	}
	configPath := "/etc/openshell/sandbox/config.toml"

	if err := configureGateway(endpoint, "/secrets/mtls", cli); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: gateway config: %v\n", err)
		os.Exit(1)
	}

	cfg, err := parseConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("=== Sandbox Launcher ===")
	fmt.Printf("  Name:      %s\n", cfg.Name)
	fmt.Printf("  Image:     %s\n", cfg.Image)
	fmt.Printf("  Providers: %s\n", strings.Join(cfg.Providers, " "))
	fmt.Printf("  Command:   %s\n", cfg.Command)
	fmt.Printf("  Gateway:   %s\n", endpoint)
	fmt.Println()

	providers := checkProviders(cfg.Providers, cli)

	harnessDir := "/tmp/openshell"
	if err := stageFiles(cfg, "/secrets/gws", harnessDir); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: staging files: %v\n", err)
		os.Exit(1)
	}

	if err := createSandbox(cfg, providers, cli); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}

	if err := uploadFiles(cfg.Name, harnessDir, cli); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: upload failed: %v\n", err)
		os.Exit(1)
	}

	if err := runStartup(cfg.Name, cli); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: startup failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\nSandbox '%s' is ready.\n", cfg.Name)
	fmt.Printf("Connect with: openshell sandbox connect %s\n", cfg.Name)
	fmt.Printf("Or from inside the sandbox: %s\n", cfg.Command)

}
