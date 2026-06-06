package cmd

import (
	"fmt"
	"os"
	"path/filepath"
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

			// 1. Validate gateway is deployed and reachable
			if err := gw.InferenceGet(); err != nil {
				return fmt.Errorf("no active gateway — deploy one first: harness deploy")
			}

			// 2. Determine mode from gateway config
			//    Try all known gateway dirs; fall back to "direct" if no config found.
			gwCfg := loadFirstGatewayConfig(harnessDir)
			useLauncher := gwCfg != nil && gwCfg.UsesLauncher()

			// 3. Parse and validate the profile
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

			// 4. Validate providers are registered
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

			// 5. Run preflight checks for profile providers
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

			// 6. Deploy the sandbox
			status.Section("Creating sandbox")
			if useLauncher {
				return createViaLauncher(harnessDir, gwCfg, gw, profileName, cfg)
			}
			return createDirect(harnessDir, gw, gwCfg, profileName, cfg, registered)
		},
	}

	cmd.Flags().StringVar(&profileName, "profile", "default", "Profile name (from profiles/)")
	cmd.Flags().StringVar(&sandboxName, "name", "", "Sandbox name (overrides profile)")

	return cmd
}

// loadFirstGatewayConfig tries to load a gateway config from known gateway dirs.
// Returns nil if no valid config is found (backward compat — assumes direct mode).
func loadFirstGatewayConfig(harnessDir string) *gateway.GatewayConfig {
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
		if err == nil {
			return cfg
		}
	}
	return nil
}

// createViaLauncher deploys a sandbox using the launcher Job path (remote/OCP).
// Reuses the same logic as newRemote but does not deploy the gateway or providers.
func createViaLauncher(harnessDir string, gwCfg *gateway.GatewayConfig, gw gateway.Gateway, profileName string, cfg *profile.Config) error {
	// newRemote handles the launcher Job creation, log tailing, and status polling.
	// We call it directly — it will skip gateway/provider deployment because the
	// gateway is already reachable (checked above).
	err := upRemote(harnessDir, gwCfg, gw, profileName, cfg.Name)
	if err != nil {
		return err
	}
	// newRemote already prints the success message
	return nil
}

// createDirect deploys a sandbox via the local openshell CLI (direct mode).
// Same as newLocal but always non-interactive.
func createDirect(harnessDir string, gw gateway.Gateway, gwCfg *gateway.GatewayConfig, profileName string, cfg *profile.Config, registered []string) error {
	// Resolve Dockerfile path relative to harnessDir
	if cfg.From != "" && !filepath.IsAbs(cfg.From) {
		candidate := filepath.Join(harnessDir, cfg.From)
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			cfg.From = candidate
		}
	}

	// Inject non-secret provider env vars into sandbox env
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

	// Stage files
	tmpParent, err := os.MkdirTemp("", "harness-")
	if err != nil {
		return fmt.Errorf("creating staging dir: %w", err)
	}
	defer os.RemoveAll(tmpParent)
	harnessUploadDir := filepath.Join(tmpParent, "openshell")
	if err := profile.StageHarnessDir(cfg, harnessUploadDir); err != nil {
		return fmt.Errorf("staging files: %w", err)
	}

	// Build command — always non-interactive (no TTY)
	var sandboxCmd []string
	if cfg.Startup != "" {
		sandboxCmd = []string{"bash", "-c", fmt.Sprintf(". %s", cfg.Startup)}
	} else {
		sandboxCmd = []string{"true"}
	}

	// Create sandbox with retry
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
		gw.SandboxDelete(cfg.Name) // best-effort cleanup

		if attempt == 5 {
			return fmt.Errorf("sandbox create failed after 5 attempts: %w", err)
		}
		time.Sleep(5 * time.Second)
	}
	return nil // unreachable
}

func providerInList(name string, providers []string) bool {
	for _, p := range providers {
		if p == name {
			return true
		}
	}
	return false
}
