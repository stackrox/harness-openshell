package preflight

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/robbycochran/harness-openshell/internal/gateway"
	"github.com/robbycochran/harness-openshell/internal/status"
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
	Providers       []string       `toml:"providers"`
	ProvidersCustom []string       `toml:"providers-custom"`
	Upstream        UpstreamConfig `toml:"upstream"`
	ChartVersion    string         `toml:"-"`
}

type UpstreamConfig struct {
	ChartVersion string `toml:"chart-version"`
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
	cf.ChartVersion = cf.Upstream.ChartVersion
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

func runQuiet(command string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, "bash", "-c", command).Run() == nil
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

func loadEnabledProviders(harnessDir string) ([]Provider, error) {
	providersPath := os.Getenv("PROVIDERS_TOML")
	if providersPath == "" {
		providersPath = filepath.Join(harnessDir, "providers.toml")
	}
	configPath := os.Getenv("CONFIG_TOML")
	if configPath == "" {
		configPath = filepath.Join(harnessDir, "openshell.toml")
	}
	all, err := LoadProviders(providersPath)
	if err != nil {
		return nil, err
	}
	config, _ := LoadConfig(configPath)
	return EnabledProviders(all, config), nil
}

func RunCheck(harnessDir string, gw gateway.Gateway, strict bool) error {
	providers, err := loadEnabledProviders(harnessDir)
	if err != nil {
		return err
	}

	hasFailures := false

	fmt.Println("=== OpenShell CLI ===")
	cliPath := gw.CLIPath()
	cliFound := cliPath != ""
	if !cliFound {
		status.Fail("not found on PATH")
		hasFailures = true
	} else {
		ver := gw.CLIVersion()
		if ver != "" {
			status.OK(ver)
		} else {
			status.OK("openshell")
		}
		status.Detail(cliPath)
	}

	activeGW := ""
	if cliFound {
		activeGW = gw.ActiveGateway()
	}
	isK8s := strings.Contains(activeGW, "-remote-")

	gwOK := false
	if isK8s {
		status.Section("K8s gateway")
		kubectlPath, _ := exec.LookPath("kubectl")
		if kubectlPath == "" {
			status.Fail("kubectl not found")
			hasFailures = true
		} else {
			ctx := runOutput("kubectl", "config", "current-context")
			if ctx != "" {
				status.OKf("Cluster: %s", ctx)
				if cliFound {
					if gw.InferenceGet() == nil {
						gwOK = true
						model := gw.InferenceModel()
						if model != "" {
							status.OKf("Gateway reachable (model: %s)", model)
						} else {
							status.OK("Gateway reachable")
						}
					} else {
						status.Fail("Gateway unreachable")
					}
				}
			} else {
				status.Fail("No cluster (kubectl not configured)")
				hasFailures = true
			}
		}
	} else {
		status.Section("Podman gateway")
		if cliFound {
			if gw.InferenceGet() == nil {
				gwOK = true
				model := gw.InferenceModel()
				if model != "" {
					status.OKf("Reachable (model: %s)", model)
				} else {
					status.OK("Reachable")
				}
			} else {
				status.Info("Not running")
			}

			podmanPath, _ := exec.LookPath("podman")
			if podmanPath != "" {
				ver := runOutput("podman", "--version")
				status.OKf("Podman: %s", ver)
			} else {
				status.Fail("Podman not found")
				hasFailures = true
			}
		} else {
			status.Info("CLI not available")
		}
	}

	if cliFound && gwOK {
		gwLabel := "podman"
		if isK8s {
			gwLabel = "k8s"
		}
		status.Section(fmt.Sprintf("Registered providers (%s)", gwLabel))
		for _, p := range providers {
			if p.Type != "openshell" {
				continue
			}
			if gw.ProviderGet(p.Name) == nil {
				status.OK(p.Name)
			} else {
				status.Failf("%s: not registered — run: harness providers", p.Name)
				hasFailures = true
			}
		}
	}

	status.Section("Provider inputs")
	for _, p := range providers {
		ok, details := CheckProvider(p)
		if ok {
			status.OK(p.Name)
		} else {
			status.Fail(p.Name)
			if p.Required {
				hasFailures = true
			}
		}
		status.Detail(p.Description)

		for _, d := range details {
			status.Sub(d)
		}

		if p.Upstream != "" && !ok {
			status.Sub(fmt.Sprintf("upstream: %s", p.Upstream))
		}
		fmt.Println()
	}

	status.Summary(!hasFailures)
	if hasFailures && strict {
		return fmt.Errorf("preflight: required checks failed")
	}
	return nil
}

func RunAvailable(harnessDir string) error {
	providers, err := loadEnabledProviders(harnessDir)
	if err != nil {
		return err
	}

	var available []string
	for _, p := range providers {
		if p.Type != "openshell" {
			continue
		}
		ok, _ := CheckProvider(p)
		if ok {
			available = append(available, p.Name)
		}
	}
	fmt.Println(strings.Join(available, " "))
	return nil
}

func RunNames(harnessDir string) error {
	providers, err := loadEnabledProviders(harnessDir)
	if err != nil {
		return err
	}

	var names []string
	for _, p := range providers {
		if p.Type == "openshell" {
			names = append(names, p.Name)
		}
	}
	fmt.Println(strings.Join(names, " "))
	return nil
}

func runOutput(name string, args ...string) string {
	cmd := exec.Command(name, args...)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
