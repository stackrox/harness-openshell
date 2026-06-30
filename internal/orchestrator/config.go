package orchestrator

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type OrchestratorConfig struct {
	Mode         string   `yaml:"mode"`
	Entrypoint   string   `yaml:"entrypoint"`
	Task         string   `yaml:"task"`
	TTY          bool     `yaml:"tty"`
	Sentinel     bool     `yaml:"sentinel"`
	PollInterval int      `yaml:"poll_interval"`
	MaxFailures  int      `yaml:"max_failures"`
	Heartbeat    int      `yaml:"heartbeat"`
	OnComplete   []string `yaml:"on_complete"`
	OnPropose    []string `yaml:"on_propose"`
	SessionDir   string   `yaml:"session_dir"`
}

func LoadConfig(path string) (*OrchestratorConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}
	var cfg OrchestratorConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *OrchestratorConfig) ApplyDefaults() {
	if c.Mode == "" {
		c.Mode = "once"
	}
	if c.Entrypoint == "" {
		c.Entrypoint = "claude"
	}
	if c.PollInterval <= 0 {
		c.PollInterval = 300
	}
	if c.MaxFailures <= 0 {
		c.MaxFailures = 5
	}
	if c.Heartbeat == 0 {
		c.Heartbeat = 60
	}
	if c.SessionDir == "" {
		c.SessionDir = "/sandbox/.harness"
	}
}

func (c *OrchestratorConfig) Validate() error {
	switch c.Mode {
	case "once", "watch":
	default:
		return fmt.Errorf("invalid mode %q: must be \"once\" or \"watch\"", c.Mode)
	}
	switch c.Entrypoint {
	case "claude", "codex", "opencode":
	default:
		return fmt.Errorf("unsupported entrypoint %q: must be claude, codex, or opencode", c.Entrypoint)
	}
	return nil
}
