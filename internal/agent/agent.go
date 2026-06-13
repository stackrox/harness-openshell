package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type ProviderRef struct {
	Profile string            `yaml:"profile"`
	Env     map[string]string `yaml:"env,omitempty"`
}

type AgentConfig struct {
	Name       string            `yaml:"name"`
	Gateway    string            `yaml:"gateway,omitempty"`
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

func expandEnvVar(key, value string) string {
	expanded := os.ExpandEnv(value)
	if expanded == "" {
		expanded = os.Getenv(key)
	}
	return expanded
}

func (c *AgentConfig) BuildEnvMap() map[string]string {
	env := make(map[string]string)
	for k, v := range c.Env {
		if val := expandEnvVar(k, v); val != "" {
			env[k] = val
		}
	}
	for _, p := range c.Providers {
		for k, v := range p.Env {
			if val := expandEnvVar(k, v); val != "" {
				env[k] = val
			}
		}
	}
	return env
}

func (c *AgentConfig) BuildRunSh() string {
	var b strings.Builder
	b.WriteString("#!/usr/bin/env bash\nset -euo pipefail\n\n")
	b.WriteString("PAYLOAD_DIR=\"$(cd \"$(dirname \"$0\")\" && pwd)\"\n\n")
	b.WriteString("# Prepend payload bin to PATH\n")
	b.WriteString("export PATH=\"$PAYLOAD_DIR/bin:$PATH\"\n\n")
	b.WriteString("# Validate entrypoint\n")
	entrypoint := c.EffectiveEntrypoint()
	epBin := strings.Fields(entrypoint)[0]
	fmt.Fprintf(&b, "if ! command -v %q >/dev/null 2>&1; then\n", epBin)
	fmt.Fprintf(&b, "  echo \"ERROR: entrypoint %q not found in PATH\" >&2\n", epBin)
	b.WriteString("  exit 1\n")
	b.WriteString("fi\n\n")
	b.WriteString("# Execute entrypoint\n")
	if c.Task != "" {
		fmt.Fprintf(&b, "exec %s -p \"$(cat \"$PAYLOAD_DIR/task.md\")\"\n", entrypoint)
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
		// Absolute includes are allowed as-is (the user authors the config);
		// relative includes must stay within the base directory.
		src := inc
		if !filepath.IsAbs(src) {
			src = filepath.Join(baseDir, src)
			rel, err := filepath.Rel(filepath.Clean(baseDir), filepath.Clean(src))
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return fmt.Errorf("include path %q escapes base directory", inc)
			}
		}
		clean := filepath.Clean(src)
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
