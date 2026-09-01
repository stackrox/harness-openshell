package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/stackrox/harness-openshell/internal/agent"
	"github.com/stackrox/harness-openshell/internal/gateway"
	"github.com/stackrox/harness-openshell/internal/openshell"
	"github.com/stackrox/harness-openshell/internal/status"
)

func NewApplyCmd(harnessDir, cli string, newClient openshell.Factory) *cobra.Command {
	var (
		file        string
		agentName   string
		sandboxName string
		task        string
		entrypoint  string
		attach      bool
		dryRun      bool
		setupOnly   bool
		output      string
	)
	var gatewayName, workspace *string

	cmd := &cobra.Command{
		Use:   "apply [flags]",
		Short: "Apply an agent configuration to create a sandbox",
		Long: `Resolve a harness.openshell.dev/v1alpha1 workflow and execute its
planned reconciliation and sandbox run. Legacy agent configs remain supported
through a compatibility adapter. Provision the gateway with OpenShell first.
Use --dry-run to render the v1alpha1 action plan without mutating anything, or
-o yaml to output the fully resolved configuration.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 && sandboxName == "" {
				sandboxName = args[0]
			}

			// An explicit v1alpha1 document takes the canonical path. Legacy files
			// and the built-in default continue through resolveHarness below; this is
			// the only compatibility branch and no v1alpha1 value is converted back
			// into agent.AgentConfig.
			if file != "" {
				canonical, err := isCanonicalWorkflow(file)
				if err != nil {
					return err
				}
				if canonical {
					if cmd.Flags().Changed("agent") {
						return fmt.Errorf("--agent selects a legacy profile and cannot be used with a v1alpha1 workflow")
					}
					if task != "" {
						return fmt.Errorf("--task is not supported for v1alpha1 workflows; declare agent arguments and payloads in the workflow")
					}
					workflow, err := loadCanonicalWorkflow(file, *gatewayName, *workspace, canonicalOverrides{
						Name: sandboxName, AgentType: entrypoint, ForceTTY: attach,
					})
					if err != nil {
						return err
					}
					if output != "" && !dryRun {
						return renderCanonicalWorkflow(workflow, output)
					}
					gw := gateway.New(cli)
					// The CLI version gate only matters for the CLI run path. A
					// direct target never uses the CLI, so skip it there and let
					// applyCanonical reject any CLI-only run configuration with a
					// clear message.
					if !dryRun && workflow.Target.Direct == nil && !canonicalSDKRunEligible(workflow) {
						if err := checkOpenShellVersion(gw); err != nil {
							return err
						}
					}

					// A direct target carries its own connection with no CLI
					// gateway name, so connect on that too — otherwise apply
					// fails the gateway guard for a reachable direct registration.
					var client openshell.Client
					if workflow.Target.Direct != nil || workflow.Target.Gateway != "" {
						client, err = newClient(cmd.Context(), workflow.Target)
						if err != nil {
							desc := fmt.Sprintf("gateway %q", workflow.Target.Gateway)
							if workflow.Target.Direct != nil {
								desc = "direct target"
							}
							if !dryRun {
								return fmt.Errorf("connecting to %s: %w", desc, err)
							}
							fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s unreachable: %v (rendering desired config only)\n", desc, err)
						} else {
							defer client.Close()
						}
					}
					planned, current, err := workflow.buildPlan(cmd.Context(), client)
					if err != nil {
						return err
					}
					return applyCanonical(cmd.Context(), workflow, planned, current, client, gw, canonicalApplyOptions{
						SetupOnly: setupOnly, DryRun: dryRun, Output: output, RetrySleep: 5 * time.Second,
					})
				}
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
				// Headless task: set TTY=false so the agent adapter dispatches with
				// --print (claude/codex) or run (opencode) instead of interactive -p.
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
			if err := checkOpenShellVersion(gw); err != nil {
				return err
			}

			// The harness runs against a gateway OpenShell already provisioned;
			// it never provisions one. Resolve the target once here — this is the
			// single owner of apply's gateway selection ($OPENSHELL_GATEWAY >
			// active marker, the latter owned by the SDK) — and thread it through
			// so reconcile and sandbox-create act on the same gateway.
			target := resolveApplyTarget(*gatewayName, *workspace)

			if dryRun {
				return dryRunApply(cmd.Context(), gw, newClient, target, agentCfg)
			}

			return upLocal(upLocalOpts{
				harnessDir:  harnessDir,
				gw:          gw,
				target:      target,
				agentCfg:    agentCfg,
				agentPath:   agentPath,
				sandboxName: sandboxName,
				noTTY:       !attach,
				setupOnly:   setupOnly,
				harness:     harness,
				newClient:   newClient,
				retrySleep:  5 * time.Second,
			})
		},
	}

	cmd.Flags().StringVarP(&file, "file", "f", "", "Path to harness/agent YAML file")
	cmd.Flags().StringVar(&agentName, "agent", "default", "Agent config name (from profiles/)")
	cmd.Flags().StringVar(&sandboxName, "name", "", "Sandbox name (overrides agent config)")
	cmd.Flags().StringVar(&task, "task", "", "Task to pass to the agent (inline text or @filepath)")
	cmd.Flags().StringVar(&entrypoint, "entrypoint", "", "Override agent entrypoint (claude, opencode, bash)")
	cmd.Flags().BoolVar(&attach, "attach", false, "Attach TTY after creation (interactive mode)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate configuration without deploying")
	cmd.Flags().BoolVar(&setupOnly, "setup-only", false, "Reconcile providers/inference on the active gateway, but do not create a sandbox or run the agent")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Output format: yaml or json")
	gatewayName, workspace = registerTargetFlags(cmd)

	return cmd
}

func checkOpenShellVersion(gw *gateway.CLI) error {
	if err := gw.CheckMinVersion(gateway.MinOpenShellVersion); err != nil {
		if errors.Is(err, gateway.ErrVersionBelowMinimum) {
			return fmt.Errorf("incompatible openshell CLI: %w", err)
		}
		status.Warn(fmt.Sprintf("OpenShell version: %v", err))
	}
	return nil
}

func renderOutput(harnessDir string, h *agent.Harness, format string) error {
	builtinProviders := loadProviderProfiles(harnessDir)

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

func dryRunApply(ctx context.Context, gw gateway.Gateway, newClient openshell.Factory, target openshell.Target, agentCfg *agent.AgentConfig) error {
	status.Header("Dry Run")
	allPass := true

	status.OKf("agent: %s (entrypoint: %s)", agentCfg.Name, agentCfg.EffectiveEntrypoint())

	image := resolveSandboxImage(agentCfg.Image)
	status.OKf("image: %s", image)

	// Report the resolved target — the same gateway apply will act on. An empty
	// name means the SDK will resolve the active gateway. Probe reachability
	// over the SDK health check, the same preflight apply's run path uses.
	gwName := target.Gateway
	if gwName == "" {
		gwName = "(active)"
	}
	if checkGatewayReachable(ctx, newClient, target) != nil {
		status.Failf("gateway: %s (not reachable)", gwName)
		allPass = false
	} else {
		status.OKf("gateway: %s (reachable)", gwName)
	}

	for _, p := range agentCfg.Providers {
		if gw.ProviderGet(p.Profile) == nil {
			status.OKf("provider: %s (registered)", p.Profile)
		} else {
			status.Warnf("provider: %s (not registered, will be created)", p.Profile)
		}
	}

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
