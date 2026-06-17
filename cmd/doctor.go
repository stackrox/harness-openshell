package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/robbycochran/harness-openshell/internal/agent"
	"github.com/robbycochran/harness-openshell/internal/gateway"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

type CheckResult struct {
	Group   string `json:"group"`
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

type CheckFunc func(cfg *agent.AgentConfig, cli, harnessDir string) []CheckResult

func NewDoctorCmd(harnessDir, cli string) *cobra.Command {
	var (
		agentFile string
		agentName string
		output    string
	)

	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Validate environment for configured sandbox",
		Long: `Check that prerequisites are met for running a sandbox.

Phase 1 (offline): checks openshell binary, target dependencies, and
provider credentials without requiring a running gateway.

Phase 2 (online): if the gateway is reachable, checks provider registration.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := parseOutputFormat(output)
			if err != nil {
				return err
			}

			h, err := resolveHarness(harnessDir, agentName, agentFile)
			if err != nil {
				return fmt.Errorf("resolving config: %w", err)
			}

			checks := []CheckFunc{
				checkOpenShell,
				checkTargetDeps,
				checkProviderEnvVars,
			}

			var results []CheckResult
			for _, fn := range checks {
				results = append(results, fn(h.Agent, cli, harnessDir)...)
			}

			results = append(results, checkOnline(h.Agent, cli)...)

			if format != formatTable {
				return printStructured(format, results)
			}

			printDoctorTable(results)

			for _, r := range results {
				if r.Status == "fail" {
					return fmt.Errorf("one or more checks failed")
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&agentFile, "file", "f", "", "Path to harness YAML")
	cmd.Flags().StringVar(&agentName, "agent", "default", "Agent config name")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Output format (table, json, yaml)")

	return cmd
}

func checkOpenShell(cfg *agent.AgentConfig, cli, _ string) []CheckResult {
	path, err := exec.LookPath(cli)
	if err != nil {
		return []CheckResult{{
			Group:   "openshell",
			Name:    "binary",
			Status:  "fail",
			Message: fmt.Sprintf("%s not found on PATH", cli),
		}}
	}

	out, err := exec.Command(path, "--version").Output()
	if err != nil {
		return []CheckResult{{
			Group:   "openshell",
			Name:    "binary",
			Status:  "warn",
			Message: fmt.Sprintf("found at %s, version unknown", path),
		}}
	}

	version := strings.TrimSpace(string(out))
	return []CheckResult{{
		Group:   "openshell",
		Name:    "binary",
		Status:  "pass",
		Message: version,
	}}
}

func checkTargetDeps(cfg *agent.AgentConfig, _, _ string) []CheckResult {
	target := cfg.Gateway
	if target == "" {
		target = "local"
	}

	switch target {
	case "local":
		return checkLocalDeps()
	case "kind":
		return checkKindDeps()
	case "ocp":
		return checkRemoteDeps()
	default:
		return checkLocalDeps()
	}
}

func checkLocalDeps() []CheckResult {
	if _, err := exec.LookPath("podman"); err == nil {
		if err := exec.Command("podman", "info").Run(); err == nil {
			ver := ""
			if out, e := exec.Command("podman", "version", "--format", "{{.Client.Version}}").Output(); e == nil {
				ver = " " + strings.TrimSpace(string(out))
			}
			return []CheckResult{{Group: "target", Name: "local", Status: "pass", Message: "podman" + ver + " running"}}
		}
	}
	if _, err := exec.LookPath("docker"); err == nil {
		if err := exec.Command("docker", "info").Run(); err == nil {
			return []CheckResult{{Group: "target", Name: "local", Status: "pass", Message: "docker running"}}
		}
	}
	return []CheckResult{{Group: "target", Name: "local", Status: "fail", Message: "no container runtime (podman or docker) responding"}}
}

func checkKindDeps() []CheckResult {
	var results []CheckResult
	results = append(results, checkLocalDeps()...)

	if _, err := exec.LookPath("kubectl"); err != nil {
		results = append(results, CheckResult{Group: "target", Name: "kubectl", Status: "fail", Message: "kubectl not found on PATH"})
	} else {
		results = append(results, CheckResult{Group: "target", Name: "kubectl", Status: "pass", Message: "found"})
	}

	if _, err := exec.LookPath("kind"); err != nil {
		results = append(results, CheckResult{Group: "target", Name: "kind", Status: "fail", Message: "kind not found on PATH"})
	} else {
		results = append(results, CheckResult{Group: "target", Name: "kind", Status: "pass", Message: "found"})
	}

	return results
}

func checkRemoteDeps() []CheckResult {
	var results []CheckResult

	kubectlFound := false
	if _, err := exec.LookPath("kubectl"); err == nil {
		kubectlFound = true
		results = append(results, CheckResult{Group: "target", Name: "kubectl", Status: "pass", Message: "found"})
	} else if _, err := exec.LookPath("oc"); err == nil {
		kubectlFound = true
		results = append(results, CheckResult{Group: "target", Name: "oc", Status: "pass", Message: "found"})
	}
	if !kubectlFound {
		results = append(results, CheckResult{Group: "target", Name: "kubectl", Status: "fail", Message: "neither kubectl nor oc found on PATH"})
	}

	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		home, _ := os.UserHomeDir()
		kubeconfig = filepath.Join(home, ".kube", "config")
	}
	if _, err := os.Stat(kubeconfig); err != nil {
		results = append(results, CheckResult{Group: "target", Name: "kubeconfig", Status: "fail", Message: "kubeconfig not found at " + kubeconfig})
	} else {
		results = append(results, CheckResult{Group: "target", Name: "kubeconfig", Status: "pass", Message: kubeconfig})
	}

	return results
}

type providerProfile struct {
	ID          string              `yaml:"id"`
	DisplayName string              `yaml:"display_name"`
	Credentials []providerCredential `yaml:"credentials"`
}

type providerCredential struct {
	Name     string   `yaml:"name"`
	EnvVars  []string `yaml:"env_vars"`
	Required bool     `yaml:"required"`
	Refresh  *struct {
		Strategy string `yaml:"strategy"`
	} `yaml:"refresh,omitempty"`
}

func checkProviderEnvVars(cfg *agent.AgentConfig, cli, harnessDir string) []CheckResult {
	if len(cfg.Providers) == 0 {
		return nil
	}

	var results []CheckResult
	for _, p := range cfg.Providers {
		profile := loadProviderProfile(p.Profile, cli, harnessDir)
		if profile == nil {
			results = append(results, CheckResult{
				Group:   "provider",
				Name:    p.Profile,
				Status:  "warn",
				Message: "no profile found, cannot check credentials",
			})
			continue
		}

		allGatewayManaged := true
		allSet := true
		var missing []string
		for _, cred := range profile.Credentials {
			if !cred.Required {
				continue
			}
			// Gateway-managed credentials (OAuth refresh, service account JWT)
			// are handled by the gateway, not set by the user as env vars.
			if cred.Refresh != nil {
				continue
			}
			allGatewayManaged = false
			found := false
			for _, ev := range cred.EnvVars {
				if os.Getenv(ev) != "" {
					found = true
					break
				}
			}
			if !found {
				allSet = false
				missing = append(missing, cred.EnvVars[0])
			}
		}

		if allGatewayManaged {
			r := checkGatewayManagedProvider(p.Profile)
			results = append(results, r)
		} else if allSet {
			results = append(results, CheckResult{
				Group:   "provider",
				Name:    p.Profile,
				Status:  "pass",
				Message: "credentials set",
			})
		} else {
			results = append(results, CheckResult{
				Group:   "provider",
				Name:    p.Profile,
				Status:  "fail",
				Message: "missing: " + strings.Join(missing, ", "),
			})
		}
	}

	return results
}

func checkGatewayManagedProvider(name string) CheckResult {
	switch name {
	case "google-workspace":
		gwsPath, _ := exec.LookPath("gws")
		if gwsPath == "" {
			return CheckResult{Group: "provider", Name: name, Status: "fail", Message: "gws CLI not installed (brew install googleworkspace/cli/gws)"}
		}
		if err := exec.Command(gwsPath, "auth", "export", "--unmasked").Run(); err != nil {
			return CheckResult{Group: "provider", Name: name, Status: "fail", Message: "not authenticated (run: gws auth login)"}
		}
		return CheckResult{Group: "provider", Name: name, Status: "pass", Message: "authenticated (gateway-managed OAuth)"}
	case "google-vertex-ai":
		home, _ := os.UserHomeDir()
		adcPath := envOr("GOOGLE_APPLICATION_CREDENTIALS",
			filepath.Join(home, ".config", "gcloud", "application_default_credentials.json"))
		if _, err := os.Stat(adcPath); err != nil {
			return CheckResult{Group: "provider", Name: name, Status: "fail", Message: "ADC not found (run: gcloud auth application-default login)"}
		}
		return CheckResult{Group: "provider", Name: name, Status: "pass", Message: "ADC found (gateway-managed refresh)"}
	default:
		return CheckResult{Group: "provider", Name: name, Status: "pass", Message: "gateway-managed credentials"}
	}
}

func loadProviderProfile(name, cli, harnessDir string) *providerProfile {
	if profile := loadProfileFromOpenShell(name, cli); profile != nil {
		return profile
	}
	return loadProfileFromDisk(name, harnessDir)
}

func loadProfileFromOpenShell(name, cli string) *providerProfile {
	path, err := exec.LookPath(cli)
	if err != nil {
		return nil
	}
	out, err := exec.Command(path, "provider", "profile", "export", name).Output()
	if err != nil {
		return nil
	}
	var p providerProfile
	if err := yaml.Unmarshal(out, &p); err != nil {
		return nil
	}
	return &p
}

func loadProfileFromDisk(name, harnessDir string) *providerProfile {
	if harnessDir == "" {
		return nil
	}
	candidates := []string{
		filepath.Join(harnessDir, "profiles", "providers", name+".yaml"),
	}
	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var p providerProfile
		if err := yaml.Unmarshal(data, &p); err != nil {
			continue
		}
		return &p
	}
	return nil
}

func checkOnline(cfg *agent.AgentConfig, cli string) []CheckResult {
	gw := gateway.New(cli)
	if gw.ActiveGateway() == "" {
		return []CheckResult{{
			Group:   "gateway",
			Name:    "status",
			Status:  "warn",
			Message: "no active gateway (Phase 2 checks skipped)",
		}}
	}

	_, err := gw.ProviderList()
	if err != nil {
		return []CheckResult{{
			Group:   "gateway",
			Name:    "status",
			Status:  "warn",
			Message: "gateway not reachable (Phase 2 checks skipped)",
		}}
	}

	var results []CheckResult
	results = append(results, CheckResult{
		Group:   "gateway",
		Name:    "status",
		Status:  "pass",
		Message: "connected",
	})

	for _, p := range cfg.Providers {
		if gw.ProviderGet(p.Profile) == nil {
			results = append(results, CheckResult{
				Group:   "gateway",
				Name:    p.Profile,
				Status:  "pass",
				Message: "registered",
			})
		} else {
			results = append(results, CheckResult{
				Group:   "gateway",
				Name:    p.Profile,
				Status:  "warn",
				Message: "not registered (will be registered on apply)",
			})
		}
	}

	return results
}

func printDoctorTable(results []CheckResult) {
	groups := []string{"openshell", "target", "provider", "gateway"}
	groupLabels := map[string]string{
		"openshell": "OPENSHELL",
		"target":    "TARGET",
		"provider":  "PROVIDER",
		"gateway":   "GATEWAY",
	}

	for _, g := range groups {
		var groupResults []CheckResult
		for _, r := range results {
			if r.Group == g {
				groupResults = append(groupResults, r)
			}
		}
		if len(groupResults) == 0 {
			continue
		}

		fmt.Println(groupLabels[g])
		for _, r := range groupResults {
			icon := "  "
			switch r.Status {
			case "pass":
				icon = "OK"
			case "warn":
				icon = "!!"
			case "fail":
				icon = "XX"
			}
			fmt.Printf("  %-16s %s  %s\n", r.Name, icon, r.Message)
		}
		fmt.Println()
	}
}
