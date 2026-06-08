package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type ProviderRef struct {
	Profile string            `yaml:"profile"`
	Config  map[string]string `yaml:"config,omitempty"`
}

type AgentConfig struct {
	Name       string            `yaml:"name"`
	Providers  []ProviderRef     `yaml:"providers"`
	Env        map[string]string `yaml:"env,omitempty"`
	Task       string            `yaml:"task,omitempty"`
	Entrypoint string            `yaml:"entrypoint,omitempty"`
	TTY        *bool             `yaml:"tty,omitempty"`
	Policy     string            `yaml:"policy,omitempty"`
	Image      string            `yaml:"image,omitempty"`
	Include    []string          `yaml:"include,omitempty"`
}

func (c *AgentConfig) NoTTY() bool {
	if c.TTY == nil {
		return false
	}
	return !*c.TTY
}

func (c *AgentConfig) EffectiveEntrypoint() string {
	if c.Entrypoint == "" {
		return "claude"
	}
	return c.Entrypoint
}

func (c *AgentConfig) ProviderNames() []string {
	names := make([]string, len(c.Providers))
	for i, p := range c.Providers {
		names[i] = p.Profile
	}
	return names
}

func ParseFile(path string) (*AgentConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading agent config: %w", err)
	}
	return Parse(data)
}

func Parse(data []byte) (*AgentConfig, error) {
	var cfg AgentConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing agent config: %w", err)
	}
	if cfg.Name == "" {
		return nil, fmt.Errorf("agent config: name is required")
	}
	for i, p := range cfg.Providers {
		if p.Profile == "" {
			return nil, fmt.Errorf("agent config: providers[%d].profile is required", i)
		}
	}
	return &cfg, nil
}

func (c *AgentConfig) BuildEnvSh() string {
	env := make(map[string]string)
	for k, v := range c.Env {
		env[k] = v
	}
	for _, p := range c.Providers {
		for k, v := range p.Config {
			env[k] = v
		}
	}
	if len(env) == 0 {
		return ""
	}
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "export %s=%q\n", k, os.ExpandEnv(env[k]))
	}
	return b.String()
}

func (c *AgentConfig) BuildRunSh() string {
	var b strings.Builder
	b.WriteString("#!/usr/bin/env bash\nset -euo pipefail\n\n")
	b.WriteString("PAYLOAD_DIR=\"$(cd \"$(dirname \"$0\")\" && pwd)\"\n\n")
	b.WriteString("# Source environment\n")
	b.WriteString("if [[ -f \"$PAYLOAD_DIR/env.sh\" ]]; then\n")
	b.WriteString("  . \"$PAYLOAD_DIR/env.sh\"\n")
	b.WriteString("fi\n\n")
	b.WriteString("# Prepend payload bin to PATH\n")
	b.WriteString("export PATH=\"$PAYLOAD_DIR/bin:$PATH\"\n\n")
	b.WriteString("# Git auth\n")
	b.WriteString("gh auth setup-git 2>/dev/null || true\n\n")
	b.WriteString("# Validate entrypoint\n")
	entrypoint := c.EffectiveEntrypoint()
	epBin := strings.Fields(entrypoint)[0]
	fmt.Fprintf(&b, "if ! command -v %q >/dev/null 2>&1; then\n", epBin)
	fmt.Fprintf(&b, "  echo \"ERROR: entrypoint %q not found in PATH\" >&2\n", epBin)
	b.WriteString("  exit 1\n")
	b.WriteString("fi\n\n")
	b.WriteString("# Execute entrypoint\n")
	if c.Task != "" {
		fmt.Fprintf(&b, "exec %s \"$PAYLOAD_DIR/task.md\"\n", entrypoint)
	} else {
		fmt.Fprintf(&b, "exec %s\n", entrypoint)
	}
	return b.String()
}

func RenderPayload(cfg *AgentConfig, baseDir, destDir string) error {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("creating payload dir: %w", err)
	}
	binDir := filepath.Join(destDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return fmt.Errorf("creating bin dir: %w", err)
	}

	if envContent := cfg.BuildEnvSh(); envContent != "" {
		if err := os.WriteFile(filepath.Join(destDir, "env.sh"), []byte(envContent), 0o644); err != nil {
			return fmt.Errorf("writing env.sh: %w", err)
		}
		// sandbox.env is sourced by the sandbox image's startup.sh on boot,
		// making env vars available to sandbox exec sessions.
		if err := os.WriteFile(filepath.Join(destDir, "sandbox.env"), []byte(envContent), 0o644); err != nil {
			return fmt.Errorf("writing sandbox.env: %w", err)
		}
	}

	runSh := cfg.BuildRunSh()
	if err := os.WriteFile(filepath.Join(destDir, "run.sh"), []byte(runSh), 0o755); err != nil {
		return fmt.Errorf("writing run.sh: %w", err)
	}

	if cfg.Task != "" {
		taskSrc := cfg.Task
		if !filepath.IsAbs(taskSrc) {
			taskSrc = filepath.Join(baseDir, taskSrc)
		}
		data, err := os.ReadFile(taskSrc)
		if err != nil {
			return fmt.Errorf("reading task file %s: %w", cfg.Task, err)
		}
		expanded := os.ExpandEnv(string(data))
		if err := os.WriteFile(filepath.Join(destDir, "task.md"), []byte(expanded), 0o644); err != nil {
			return fmt.Errorf("writing task.md: %w", err)
		}
	}

	for _, inc := range cfg.Include {
		src := inc
		if !filepath.IsAbs(src) {
			src = filepath.Join(baseDir, src)
		}
		clean := filepath.Clean(src)
		if !strings.HasPrefix(clean, filepath.Clean(baseDir)) && !filepath.IsAbs(inc) {
			return fmt.Errorf("include path %q escapes base directory", inc)
		}
		data, err := os.ReadFile(clean)
		if err != nil {
			return fmt.Errorf("reading include %s: %w", inc, err)
		}
		dst := filepath.Join(destDir, filepath.Base(inc))
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return fmt.Errorf("writing include %s: %w", filepath.Base(inc), err)
		}
	}

	return nil
}
