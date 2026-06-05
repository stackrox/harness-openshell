package preflight

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

type Provider struct {
	Name        string  `toml:"name"`
	Type        string  `toml:"type"`
	Description string  `toml:"description"`
	Required    bool    `toml:"required"`
	Method      string  `toml:"method"`
	Upstream    string  `toml:"upstream"`
	Inputs      []Input `toml:"inputs"`
}

type Input struct {
	Key    string `toml:"key"`
	Kind   string `toml:"kind"`
	Secret bool   `toml:"secret"`
}

type ProvidersFile struct {
	Providers []Provider `toml:"providers"`
}

type ConfigFile struct {
	Providers       []string `toml:"providers"`
	ProvidersCustom []string `toml:"providers-custom"`
}

func LoadProviders(path string) ([]Provider, error) {
	var pf ProvidersFile
	if _, err := toml.DecodeFile(path, &pf); err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return pf.Providers, nil
}

func LoadConfig(path string) (*ConfigFile, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, nil
	}
	var cf ConfigFile
	if _, err := toml.DecodeFile(path, &cf); err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return &cf, nil
}

func EnabledProviders(all []Provider, config *ConfigFile) []Provider {
	if config == nil {
		return all
	}
	enabled := make(map[string]bool)
	for _, n := range config.Providers {
		enabled[n] = true
	}
	for _, n := range config.ProvidersCustom {
		enabled[n] = true
	}
	var result []Provider
	for _, p := range all {
		if enabled[p.Name] {
			result = append(result, p)
		}
	}
	return result
}

func CheckInput(inp Input) (bool, string) {
	switch inp.Kind {
	case "env":
		val := os.Getenv(inp.Key)
		if val != "" {
			display := val
			if inp.Secret {
				display = MaskValue(val, 4)
			}
			return true, fmt.Sprintf("✓ local env: %s=%s", inp.Key, display)
		}
		return false, fmt.Sprintf("✗ local env: %s not set  →  export %s=...", inp.Key, inp.Key)

	case "file":
		path := expandPath(inp.Key)
		if _, err := os.Stat(path); err == nil {
			meta := FileMetadata(path)
			if meta != nil && inp.Secret {
				safe := pickKeys(meta, "project", "type")
				masked := pickKeysExcept(meta, "project", "type")
				parts := formatMeta(safe)
				parts = append(parts, formatMeta(masked)...)
				if len(parts) > 0 {
					return true, fmt.Sprintf("✓ local file: %s (%s)", inp.Key, strings.Join(parts, ", "))
				}
			} else if meta != nil {
				parts := formatMeta(meta)
				if len(parts) > 0 {
					return true, fmt.Sprintf("✓ local file: %s (%s)", inp.Key, strings.Join(parts, ", "))
				}
			}
			return true, fmt.Sprintf("✓ local file: %s", inp.Key)
		}
		return false, fmt.Sprintf("✗ local file: %s not found", inp.Key)

	case "check":
		expanded := os.ExpandEnv(inp.Key)
		ok := runQuiet(expanded)
		sym := "✓"
		if !ok {
			sym = "✗"
		}
		return ok, fmt.Sprintf("%s check: %s", sym, inp.Key)
	}

	return false, fmt.Sprintf("%s: unknown kind '%s'", inp.Key, inp.Kind)
}

func CheckProvider(p Provider) (bool, []string) {
	issues := 0
	var details []string
	for _, inp := range p.Inputs {
		ok, detail := CheckInput(inp)
		if !ok {
			issues++
		}
		details = append(details, detail)
	}
	return issues == 0, details
}

func MaskValue(val string, show int) string {
	if val == "" || len(val) <= show {
		return "***"
	}
	return val[:show] + "***"
}

func FileMetadata(path string) map[string]string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil
	}
	meta := make(map[string]string)
	if qp, ok := raw["quota_project_id"].(string); ok {
		meta["project"] = qp
		if t, ok := raw["type"].(string); ok {
			meta["type"] = t
		}
	}
	if installed, ok := raw["installed"].(map[string]any); ok {
		if cid, ok := installed["client_id"].(string); ok {
			meta["client_id"] = MaskValue(cid, 4)
		}
	}
	if len(meta) == 0 {
		return nil
	}
	return meta
}

func expandPath(p string) string {
	if strings.HasPrefix(p, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, p[2:])
	}
	return os.ExpandEnv(p)
}

func runQuiet(cmd string) bool {
	ctx := exec.Command("bash", "-c", cmd)
	ctx.Stdout = nil
	ctx.Stderr = nil
	done := make(chan error, 1)
	go func() { done <- ctx.Run() }()
	select {
	case err := <-done:
		return err == nil
	case <-time.After(5 * time.Second):
		ctx.Process.Kill()
		return false
	}
}

func pickKeys(m map[string]string, keys ...string) map[string]string {
	result := make(map[string]string)
	for _, k := range keys {
		if v, ok := m[k]; ok && v != "" {
			result[k] = v
		}
	}
	return result
}

func pickKeysExcept(m map[string]string, except ...string) map[string]string {
	skip := make(map[string]bool)
	for _, k := range except {
		skip[k] = true
	}
	result := make(map[string]string)
	for k, v := range m {
		if !skip[k] && v != "" {
			result[k] = v
		}
	}
	return result
}

func formatMeta(m map[string]string) []string {
	var parts []string
	for k, v := range m {
		if v != "" {
			parts = append(parts, fmt.Sprintf("%s=%s", k, v))
		}
	}
	return parts
}
