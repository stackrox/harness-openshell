package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/robbycochran/harness-openshell/internal/agent"
	"github.com/robbycochran/harness-openshell/internal/gateway"
	"github.com/robbycochran/harness-openshell/internal/status"
	"github.com/spf13/cobra"
)

func NewApplyCmd(harnessDir, cli string) *cobra.Command {
	var (
		file            string
		agentName       string
		gatewayName     string
		gatewayProfile  string
		sandboxName     string
		task            string
		entrypoint      string
		attach          bool
		providerRefresh bool
		dryRun          bool
		output          string
	)

	cmd := &cobra.Command{
		Use:   "apply [flags]",
		Short: "Apply an agent configuration to create a sandbox",
		Long: `Resolve an agent config against the profiles directory and running gateway,
then deploy a sandbox. Use --dry-run to validate without deploying, or
-o yaml to output the fully resolved configuration.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if gatewayName != "" && gatewayProfile != "" {
				return fmt.Errorf("--gateway and --gateway-profile are mutually exclusive")
			}
			if len(args) > 0 && sandboxName == "" {
				sandboxName = args[0]
			}

			harness, err := resolveHarness(harnessDir, agentName, file)
			if err != nil {
				return err
			}
			agentCfg := harness.Agent
			agentPath := resolveAgentPath(harnessDir, agentName, file)

			// CLI overrides
			if entrypoint != "" {
				agentCfg.Entrypoint = entrypoint
			}
			if task != "" && !attach {
				// Headless task: set TTY=false so BuildRunSh generates --print
				f := false
				agentCfg.TTY = &f
			}
			if task != "" {
				if strings.HasPrefix(task, "@") {
					path := task[1:]
					if path == "" {
						return fmt.Errorf("--task @: missing file path after @")
					}
					agentCfg.Task = path
				} else {
					tmpTask, err := os.CreateTemp("", "harness-task-*.md")
					if err != nil {
						return fmt.Errorf("creating task file: %w", err)
					}
					defer os.Remove(tmpTask.Name())
					if _, err := tmpTask.WriteString(task); err != nil {
						tmpTask.Close()
						return fmt.Errorf("writing task: %w", err)
					}
					tmpTask.Close()
					agentCfg.Task = tmpTask.Name()
				}
			}

			// Print config path (skip for structured output)
			if output == "" {
				status.Infof("Config: %s", agentPath)
			}

			// Resolve output modes before touching the gateway
			if output == "yaml" || output == "json" {
				return renderOutput(harnessDir, harness, output)
			}

			gw := gateway.New(cli)
			if err := gw.CheckMinVersion("0.0.59"); err != nil {
				status.Warn(fmt.Sprintf("OpenShell version: %v", err))
			}

			// Resolve gateway
			var gwCfg *gateway.GatewayConfig
			gwTarget := gatewayName
			if gatewayProfile != "" {
				gwCfg, err = resolveGatewayConfigFromFile(gatewayProfile)
				if err != nil {
					return err
				}
				gwTarget = gwCfg.Gateway.Type
			} else {
				if gwTarget == "" {
					if agentCfg.Gateway != "" {
						gwTarget = agentCfg.Gateway
					} else {
						gwTarget = "local"
					}
				}
				gwCfg, _ = resolveGatewayConfigWithHarness(harnessDir, gwTarget, harness)
			}
			isRemote := gwTarget != "local"

			if dryRun {
				return dryRunApply(gw, agentCfg, gwTarget, isRemote)
			}

			return upLocal(upLocalOpts{
				harnessDir:      harnessDir,
				gw:              gw,
				gwCfg:           gwCfg,
				ensureLocal:     !isRemote,
				agentCfg:        agentCfg,
				agentPath:       agentPath,
				sandboxName:     sandboxName,
				noTTY:           !attach,
				providerRefresh: providerRefresh,
				harness:         harness,
				retrySleep:      5 * time.Second,
			})
		},
	}

	cmd.Flags().StringVarP(&file, "file", "f", "", "Path to harness/agent YAML file")
	cmd.Flags().StringVar(&agentName, "agent", "default", "Agent config name (from profiles/)")
	cmd.Flags().StringVar(&gatewayName, "gateway", envOr("OPENSHELL_GATEWAY", ""), "Gateway profile name (local, kind, ocp)")
	cmd.Flags().StringVar(&gatewayProfile, "gateway-profile", "", "Path to gateway profile YAML")
	cmd.Flags().StringVar(&sandboxName, "name", "", "Sandbox name (overrides agent config)")
	cmd.Flags().StringVar(&task, "task", "", "Task to pass to the agent (inline text or @filepath)")
	cmd.Flags().StringVar(&entrypoint, "entrypoint", "", "Override agent entrypoint (claude, opencode, bash)")
	cmd.Flags().BoolVar(&attach, "attach", false, "Attach TTY after creation (interactive mode)")
	cmd.Flags().BoolVar(&providerRefresh, "provider-refresh", false, "Delete and recreate all providers")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate configuration without deploying")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Output format: yaml or json")

	return cmd
}

func renderOutput(harnessDir string, h *agent.Harness, format string) error {
	builtinProviders := loadProviderProfiles(harnessDir)

	gwName := h.Agent.Gateway
	if gwName == "" {
		gwName = "local"
	}
	if len(h.Gateways) == 0 {
		if gwData := loadGatewayProfile(harnessDir, gwName); gwData != nil {
			h.Gateways[gwName] = gwData
		}
	}

	switch format {
	case "yaml":
		out, err := agent.RenderHarness(h, builtinProviders)
		if err != nil {
			return fmt.Errorf("rendering harness: %w", err)
		}
		fmt.Print(string(out))
	case "json":
		data := map[string]any{
			"agent":     h.Agent,
			"gateways":  mapKeys(h.Gateways),
			"providers": mapKeys(h.Providers),
			"hasPolicy": h.Policy != nil,
		}
		out, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			return fmt.Errorf("marshaling json: %w", err)
		}
		fmt.Println(string(out))
	}
	return nil
}

func mapKeys(m map[string][]byte) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func dryRunApply(gw gateway.Gateway, agentCfg *agent.AgentConfig, gwTarget string, isRemote bool) error {
	status.Header("Dry Run")
	allPass := true

	// 1. Agent config
	status.OKf("agent: %s (entrypoint: %s)", agentCfg.Name, agentCfg.EffectiveEntrypoint())

	// 2. Image
	image := resolveSandboxImage(agentCfg.Image)
	status.OKf("image: %s", image)

	// 3. Gateway
	if isRemote {
		if gw.InferenceGet() != nil {
			status.Failf("gateway: %s (not reachable)", gwTarget)
			allPass = false
		} else {
			status.OKf("gateway: %s (reachable)", gwTarget)
		}
	} else {
		status.OKf("gateway: %s (local)", gwTarget)
	}

	// 4. Providers
	for _, p := range agentCfg.Providers {
		if gw.ProviderGet(p.Profile) == nil {
			status.OKf("provider: %s (registered)", p.Profile)
		} else {
			status.Warnf("provider: %s (not registered, will be created)", p.Profile)
		}
	}

	// 5. Env vars
	env := agentCfg.BuildEnvMap()
	resolved := 0
	missing := 0
	for k, v := range env {
		if v != "" {
			resolved++
		} else {
			status.Warnf("env: %s (empty)", k)
			missing++
		}
	}
	if resolved > 0 {
		status.OKf("env: %d vars resolved", resolved)
	}
	if missing > 0 {
		status.Warnf("env: %d vars empty", missing)
	}

	// 6. Task file
	if agentCfg.Task != "" {
		if _, err := os.Stat(agentCfg.Task); err != nil {
			status.Failf("task: %s (not found)", agentCfg.Task)
			allPass = false
		} else {
			status.OKf("task: %s", agentCfg.Task)
		}
	}

	fmt.Println()
	if allPass {
		status.OK("Ready to apply")
	} else {
		status.Fail("Issues found -- fix before applying")
		return fmt.Errorf("dry-run failed")
	}
	return nil
}
