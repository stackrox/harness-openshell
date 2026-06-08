package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/robbycochran/harness-openshell/internal/agent"
	"github.com/robbycochran/harness-openshell/internal/gateway"
	"github.com/robbycochran/harness-openshell/internal/preflight"
	"github.com/robbycochran/harness-openshell/internal/profile"
	"github.com/robbycochran/harness-openshell/internal/status"
	"github.com/spf13/cobra"
)

func NewCreateCmd(harnessDir, cli string) *cobra.Command {
	var (
		agentName   string
		agentFile   string
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

			agentPath := resolveAgentPath(harnessDir, agentName, agentFile)

			gw := gateway.New(cli)

			// 1. Check which gateway is active and whether it's local or remote.
			activeGW, err := activeGatewayInfo(gw)
			if err != nil {
				return err
			}
			isLocal := strings.Contains(activeGW.Endpoint, "127.0.0.1")

			status.Section("Gateway")
			status.OKf("%s (%s)", activeGW.Name, activeGW.Endpoint)
			agentCfg, err := agent.ParseFile(agentPath)
			if err != nil {
				return err
			}
			name := agentCfg.Name
			if sandboxName != "" {
				name = sandboxName
			}

			status.Section("Agent")
			fmt.Printf("  Name:  %s\n", name)
			fmt.Printf("  Image: %s\n", agentCfg.Image)

			// 3. Validate providers are registered
			status.Section("Providers")
			providerNames := agentCfg.ProviderNames()
			registered, missing := profile.ValidateProviders(providerNames, gw)
			for _, n := range registered {
				status.OKf("%s: attached", n)
			}
			for _, n := range missing {
				status.Failf("%s: not registered", n)
			}
			if len(missing) > 0 && len(registered) == 0 {
				return fmt.Errorf("no providers available — run: harness providers")
			}

			// 4. Run preflight checks
			status.Section("Preflight")
			providersPath := filepath.Join(harnessDir, "providers.toml")
			allProviders, err := preflight.LoadProviders(providersPath)
			if err != nil {
				status.Warn("could not load providers.toml — skipping preflight")
			} else {
				preflightOK := true
				for _, p := range allProviders {
					if !providerInList(p.Name, providerNames) {
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

			// 5. Determine whether the in-cluster runner is needed.
			needsRunner := false
			if !isLocal {
				needsRunner = profileHasCustomProviders(providerNames, allProviders)
			}

			// 6. Deploy the sandbox
			status.Section("Creating sandbox")
			if needsRunner {
				status.Info("Custom providers detected — using in-cluster runner")
				gwCfg := loadGatewayConfigForActive(harnessDir, activeGW)
				return createViaRunner(harnessDir, gwCfg, gw, agentName, name)
			}

			// Render payload and create directly
			payloadDir, err := os.MkdirTemp("", "harness-payload-")
			if err != nil {
				return fmt.Errorf("creating payload dir: %w", err)
			}
			defer os.RemoveAll(payloadDir)

			if err := agent.RenderPayload(agentCfg, harnessDir, payloadDir); err != nil {
				return fmt.Errorf("rendering payload: %w", err)
			}

			sandboxImage := agentCfg.Image
			if envImage := os.Getenv("SANDBOX_IMAGE"); envImage != "" {
				sandboxImage = envImage
			}

			cfg := &profile.Config{
				Name: name,
				From: sandboxImage,
			}

			return createSandbox(sandboxOpts{
				harnessDir: harnessDir,
				gw:         gw,
				cfg:        cfg,
				providers:  registered,
				noTTY:      true,
				retrySleep: 5 * time.Second,
				sandboxCmd: []string{"bash", "-c",
					". /sandbox/.config/openshell/sandbox.env 2>/dev/null && " +
						"cat /sandbox/.config/openshell/sandbox.env >> /sandbox/.bashrc 2>/dev/null; true"},
				payloadDir: payloadDir,
				onSuccess: func(n string) {
					fmt.Println()
					status.OKf("Sandbox created: %s — connect with: harness connect %s", n, n)
				},
			})
		},
	}

	cmd.Flags().StringVar(&agentName, "agent", "default", "Agent config name (from agents/)")
	cmd.Flags().StringVarP(&agentFile, "file", "f", "", "Path to agent YAML file (overrides --agent)")
	cmd.Flags().StringVar(&sandboxName, "name", "", "Sandbox name (overrides agent config)")

	return cmd
}

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

func profileHasCustomProviders(providerNames []string, allProviders []preflight.Provider) bool {
	custom := make(map[string]bool)
	for _, p := range allProviders {
		if p.Type == "custom" {
			custom[p.Name] = true
		}
	}
	for _, name := range providerNames {
		if custom[name] {
			return true
		}
	}
	return false
}

func loadGatewayConfigForActive(harnessDir string, active *gateway.GatewayInfo) *gateway.GatewayConfig {
	if active != nil && active.Name != "" {
		dir := filepath.Join(harnessDir, "gateways", active.Name)
		if cfg, err := gateway.LoadConfig(dir); err == nil {
			return cfg
		}
	}
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

func createViaRunner(harnessDir string, gwCfg *gateway.GatewayConfig, gw gateway.Gateway, agentPath, sandboxName string) error {
	if gwCfg == nil {
		return fmt.Errorf("no gateway config found for remote gateway — expected gateways/<name>/gateway.toml")
	}
	return upRemote(harnessDir, gwCfg, gw, agentPath, sandboxName)
}

func providerInList(name string, providers []string) bool {
	for _, p := range providers {
		if p == name {
			return true
		}
	}
	return false
}
