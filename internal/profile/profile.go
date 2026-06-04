package profile

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/robbycochran/harness-openshell/internal/gateway"
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

func (c *Config) BuildSandboxEnv() string {
	if len(c.Env) == 0 {
		return ""
	}
	keys := make([]string, 0, len(c.Env))
	for k := range c.Env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "export %s=%s\n", k, c.Env[k])
	}
	return b.String()
}

// ValidateProviders checks which profile providers are registered on the
// gateway. Returns the list of registered providers and a list of missing ones.
func ValidateProviders(providers []string, gw gateway.Gateway) (registered, missing []string) {
	for _, name := range providers {
		if gw.ProviderGet(name) == nil {
			registered = append(registered, name)
		} else {
			missing = append(missing, name)
		}
	}
	return
}

// StageHarnessDir writes sandbox.env and copies GWS credentials to harnessDir.
func StageHarnessDir(cfg *Config, harnessDir string) error {
	if err := os.MkdirAll(harnessDir, 0o755); err != nil {
		return err
	}

	envContent := cfg.BuildSandboxEnv()
	if envContent != "" {
		if err := os.WriteFile(filepath.Join(harnessDir, "sandbox.env"), []byte(envContent), 0o644); err != nil {
			return fmt.Errorf("writing sandbox.env: %w", err)
		}
		lines := strings.Count(envContent, "\n")
		fmt.Printf("  Env: %d vars staged\n", lines)
	}

	if err := stageGWSCreds(harnessDir); err != nil {
		fmt.Printf("  GWS: %v\n", err)
	}
	return nil
}

func stageGWSCreds(harnessDir string) error {
	gwsPath, err := exec.LookPath("gws")
	if err != nil {
		return fmt.Errorf("not installed (skipping)")
	}

	check := exec.Command(gwsPath, "auth", "status")
	check.Stdout = io.Discard
	check.Stderr = io.Discard
	if check.Run() != nil {
		return fmt.Errorf("not authenticated (skipping)")
	}

	out, err := exec.Command(gwsPath, "auth", "export", "--unmasked").Output()
	if err != nil {
		return fmt.Errorf("export failed (skipping)")
	}
	if err := os.WriteFile(filepath.Join(harnessDir, "credentials.json"), out, 0o600); err != nil {
		return err
	}

	gwsConfigDir := os.Getenv("GWS_CONFIG_DIR")
	if gwsConfigDir == "" {
		home, _ := os.UserHomeDir()
		gwsConfigDir = filepath.Join(home, ".config", "gws")
	}
	clientSecret := filepath.Join(gwsConfigDir, "client_secret.json")
	if data, err := os.ReadFile(clientSecret); err == nil {
		os.WriteFile(filepath.Join(harnessDir, "client_secret.json"), data, 0o600)
	}

	fmt.Println("  GWS: exported")
	return nil
}
