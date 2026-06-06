package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/robbycochran/harness-openshell/internal/gateway"
	"github.com/robbycochran/harness-openshell/internal/preflight"
	"github.com/robbycochran/harness-openshell/internal/profile"
	"github.com/robbycochran/harness-openshell/internal/status"
	"github.com/spf13/cobra"
)

func NewCreateCmd(harnessDir, cli string) *cobra.Command {
	var (
		profileName string
		sandboxName string
	)

	cmd := &cobra.Command{
		Use:   "create [flags]",
		Short: "Create a sandbox without attaching",
		Long:  "Validate gateway readiness, run preflight checks, and deploy a sandbox. Does not attach interactively — use 'harness connect' afterward.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 && sandboxName == "" {
				sandboxName = args[0]
			}

			gw := gateway.New(cli)

			// 1. Check which gateway is active and whether it's local or remote.
			activeGW, err := activeGatewayInfo(gw)
			if err != nil {
				return err
			}
			isLocal := strings.Contains(activeGW.Endpoint, "127.0.0.1")

			status.Section("Gateway")
			status.OKf("%s (%s)", activeGW.Name, activeGW.Endpoint)

			// 2. Parse the profile
			cfg, err := profile.Parse(harnessDir, profileName)
			if err != nil {
				return err
			}
			if sandboxName != "" {
				cfg.Name = sandboxName
			}

			status.Section("Profile")
			fmt.Printf("  Name: %s\n", cfg.Name)
			fmt.Printf("  From: %s\n", cfg.From)

			// 3. Validate providers are registered
			status.Section("Providers")
			registered, missing := profile.ValidateProviders(cfg.Providers, gw)
			for _, name := range registered {
				status.OKf("%s: attached", name)
			}
			for _, name := range missing {
				status.Failf("%s: not registered", name)
			}
			if len(missing) > 0 && len(registered) == 0 {
				return fmt.Errorf("no providers available — run: harness providers")
			}

			// 4. Run preflight checks for profile providers
			status.Section("Preflight")
			providersPath := filepath.Join(harnessDir, "providers.toml")
			allProviders, err := preflight.LoadProviders(providersPath)
			if err != nil {
				status.Warn("could not load providers.toml — skipping preflight")
			} else {
				preflightOK := true
				for _, p := range allProviders {
					if !providerInList(p.Name, cfg.Providers) {
						continue
					}
					ok, details := preflight.CheckProvider(p)
					if ok {
						status.OKf("%s: ready", p.Name)
					} else {
						status.Failf("%s: prerequisites missing", p.Name)
						if p.Required {
							preflightOK = false
						}
					}
					for _, d := range details {
						status.Detail(d)
					}
				}
				if !preflightOK {
					return fmt.Errorf("preflight checks failed — fix issues above")
				}
			}

			// 5. Determine whether the launcher is needed.
			//    The launcher bridges cluster-side secrets (mTLS, GWS creds)
			//    into the sandbox. It's only needed when:
			//      - the gateway is remote (not local), AND
			//      - the profile references custom providers (type="custom" in providers.toml)
			needsLauncher := false
			if !isLocal {
				needsLauncher = profileHasCustomProviders(cfg.Providers, allProviders)
			}

			// 6. Deploy the sandbox
			status.Section("Creating sandbox")
			if needsLauncher {
				status.Info("Custom providers detected — using in-cluster launcher")
				gwCfg := loadGatewayConfigForActive(harnessDir, activeGW)
				return createViaLauncher(harnessDir, gwCfg, gw, profileName, cfg)
			}
			return createDirect(harnessDir, gw, profileName, cfg, registered)
		},
	}

	cmd.Flags().StringVar(&profileName, "profile", "default", "Profile name (from profiles/)")
	cmd.Flags().StringVar(&sandboxName, "name", "", "Sandbox name (overrides profile)")

	return cmd
}

// activeGatewayInfo returns the currently selected gateway.
func activeGatewayInfo(gw gateway.Gateway) (*gateway.GatewayInfo, error) {
	gateways, err := gw.GatewayList()
	if err != nil {
		return nil, fmt.Errorf("could not list gateways: %w — deploy one first: harness deploy", err)
	}
	for _, g := range gateways {
		if g.Active {
			return &g, nil
		}
	}
	return nil, fmt.Errorf("no active gateway — deploy one first: harness deploy")
}

// profileHasCustomProviders checks whether any of the profile's requested
// providers are type="custom" in providers.toml. Custom providers require
// the in-cluster launcher to bridge secrets the workstation doesn't have.
func profileHasCustomProviders(profileProviders []string, allProviders []preflight.Provider) bool {
	custom := make(map[string]bool)
	for _, p := range allProviders {
		if p.Type == "custom" {
			custom[p.Name] = true
		}
	}
	for _, name := range profileProviders {
		if custom[name] {
			return true
		}
	}
	return false
}

// loadGatewayConfigForActive tries to find the gateway.toml that matches the
// active gateway. Falls back to scanning all gateway dirs if no name match.
func loadGatewayConfigForActive(harnessDir string, active *gateway.GatewayInfo) *gateway.GatewayConfig {
	// Try exact name match first (e.g., active gateway named "ocp" → gateways/ocp/)
	if active != nil && active.Name != "" {
		dir := filepath.Join(harnessDir, "gateways", active.Name)
		if cfg, err := gateway.LoadConfig(dir); err == nil {
			return cfg
		}
	}
	// Fall back to scanning all gateway dirs for a remote config
	gwDir := filepath.Join(harnessDir, "gateways")
	entries, err := os.ReadDir(gwDir)
	if err != nil {
		return nil
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		cfg, err := gateway.LoadConfig(filepath.Join(gwDir, e.Name()))
		if err == nil && !cfg.IsLocal() {
			return cfg
		}
	}
	return nil
}

// createViaLauncher deploys a sandbox using the in-cluster launcher Job.
// The launcher mounts cluster secrets (mTLS certs, custom provider credentials)
// and creates the sandbox from inside the cluster.
func createViaLauncher(harnessDir string, gwCfg *gateway.GatewayConfig, gw gateway.Gateway, profileName string, cfg *profile.Config) error {
	if gwCfg == nil {
		return fmt.Errorf("no gateway config found for remote gateway — expected gateways/<name>/gateway.toml")
	}
	return upRemote(harnessDir, gwCfg, gw, profileName, cfg.Name)
}

// createDirect deploys a sandbox via the openshell CLI (no launcher needed).
func createDirect(harnessDir string, gw gateway.Gateway, profileName string, cfg *profile.Config, registered []string) error {
	if cfg.From != "" && !filepath.IsAbs(cfg.From) {
		candidate := filepath.Join(harnessDir, cfg.From)
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			cfg.From = candidate
		}
	}

	providersPath := filepath.Join(harnessDir, "providers.toml")
	if allProviders, err := preflight.LoadProviders(providersPath); err == nil {
		providerEnv := preflight.ProviderEnvVars(allProviders, cfg.Providers)
		if cfg.Env == nil {
			cfg.Env = make(map[string]string)
		}
		for k, v := range providerEnv {
			if _, exists := cfg.Env[k]; !exists {
				cfg.Env[k] = v
			}
		}
	}

	tmpParent, err := os.MkdirTemp("", "harness-")
	if err != nil {
		return fmt.Errorf("creating staging dir: %w", err)
	}
	defer os.RemoveAll(tmpParent)
	harnessUploadDir := filepath.Join(tmpParent, "openshell")
	if err := profile.StageHarnessDir(cfg, harnessUploadDir); err != nil {
		return fmt.Errorf("staging files: %w", err)
	}

	var sandboxCmd []string
	if cfg.Startup != "" {
		sandboxCmd = []string{"bash", "-c", fmt.Sprintf(". %s", cfg.Startup)}
	} else {
		sandboxCmd = []string{"true"}
	}

	for attempt := 1; attempt <= 5; attempt++ {
		err := gw.SandboxCreate(gateway.SandboxCreateOpts{
			Name:      cfg.Name,
			From:      cfg.From,
			Providers: registered,
			TTY:       false,
			Keep:      cfg.KeepSandbox(),
			UploadSrc: harnessUploadDir,
			UploadDst: "/sandbox/.config",
			Command:   sandboxCmd,
		})
		if err == nil {
			fmt.Println()
			status.OKf("Sandbox created: %s — connect with: harness connect %s", cfg.Name, cfg.Name)
			return nil
		}

		fmt.Printf("  Attempt %d failed: %v, retrying in 5s...\n", attempt, err)
		gw.SandboxDelete(cfg.Name)

		if attempt == 5 {
			return fmt.Errorf("sandbox create failed after 5 attempts: %w", err)
		}
		time.Sleep(5 * time.Second)
	}
	return nil
}

func providerInList(name string, providers []string) bool {
	for _, p := range providers {
		if p == name {
			return true
		}
	}
	return false
}
