package profile

import (
	"fmt"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

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
		cfg.Command = "claude --bare"
	}
	return &cfg, nil
}
