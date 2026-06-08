package profile

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

// ProviderChecker checks if a provider is registered.
type ProviderChecker interface {
	ProviderGet(name string) error
}

type Config struct {
	Name      string            `toml:"name"`
	From      string            `toml:"from"`
	Command   string            `toml:"command"`
	Startup   string            `toml:"startup"`
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
		fmt.Fprintf(&b, "export %s=%q\n", k, c.Env[k])
	}
	return b.String()
}

// Parse reads a profile TOML file and returns a Config with defaults applied.
func Parse(harnessDir, name string) (*Config, error) {
	path := filepath.Join(harnessDir, "profiles", name+".toml")
	return ParseFile(path)
}

// ParseFile reads a profile TOML file by path.
func ParseFile(path string) (*Config, error) {
	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if cfg.Name == "" {
		cfg.Name = "agent"
	}
	if cfg.Command == "" {
		cfg.Command = "claude"
	}
	return &cfg, nil
}

// ValidateProviders checks which profile providers are registered on the
// gateway. Returns the list of registered providers and a list of missing ones.
func ValidateProviders(providers []string, gw ProviderChecker) (registered, missing []string) {
	for _, name := range providers {
		if gw.ProviderGet(name) == nil {
			registered = append(registered, name)
		} else {
			missing = append(missing, name)
		}
	}
	return
}

// StageHarnessDir writes sandbox.env to harnessDir.
func StageHarnessDir(cfg *Config, harnessDir string) error {
	if err := os.MkdirAll(harnessDir, 0o755); err != nil {
		return err
	}

	envContent := cfg.BuildSandboxEnv()
	if envContent != "" {
		if err := os.WriteFile(filepath.Join(harnessDir, "sandbox.env"), []byte(envContent), 0o600); err != nil {
			return fmt.Errorf("writing sandbox.env: %w", err)
		}
		lines := strings.Count(envContent, "\n")
		fmt.Printf("  Env: %d vars staged\n", lines)
	}

	return nil
}
