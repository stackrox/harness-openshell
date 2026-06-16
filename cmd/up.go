package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/robbycochran/harness-openshell/internal/agent"
	"github.com/robbycochran/harness-openshell/internal/gateway"
	"github.com/robbycochran/harness-openshell/internal/k8s"
	"github.com/robbycochran/harness-openshell/internal/status"
	"github.com/spf13/cobra"
)

func NewUpCmd(harnessDir, cli string) *cobra.Command {
	var (
		gatewayName     string
		gatewayProfile  string
		agentName       string
		agentProfile    string
		sandboxName     string
		noTTY           bool
		providerRefresh bool
	)

	cmd := &cobra.Command{
		Use:   "up [flags]",
		Short: "Deploy gateway, register providers, and create a sandbox",
		Long:  "Deploy gateway and register providers if needed, then render an agent config into a running sandbox.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if gatewayName != "" && gatewayProfile != "" {
				return fmt.Errorf("--gateway and --gateway-profile are mutually exclusive")
			}
			if len(args) > 0 && sandboxName == "" {
				sandboxName = args[0]
			}

			harness, err := resolveHarness(harnessDir, agentName, agentProfile)
			if err != nil {
				return err
			}
			agentCfg := harness.Agent
			agentPath := resolveAgentPath(harnessDir, agentName, agentProfile)

			gw := gateway.New(cli)
			if err := gw.CheckMinVersion("0.0.59"); err != nil {
				status.Warn(fmt.Sprintf("OpenShell version: %v", err))
			}

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

			return upLocal(upLocalOpts{
				harnessDir:      harnessDir,
				gw:              gw,
				gwCfg:           gwCfg,
				ensureLocal:     !isRemote,
				agentCfg:        agentCfg,
				agentPath:       agentPath,
				sandboxName:     sandboxName,
				noTTY:           noTTY,
				providerRefresh: providerRefresh,
				harness:         harness,
				retrySleep:      5 * time.Second,
			})
		},
	}

	cmd.Flags().StringVar(&gatewayName, "gateway", "", "Gateway profile name (local, kind, ocp)")
	cmd.Flags().StringVar(&gatewayProfile, "gateway-profile", "", "Path to gateway profile YAML (overrides --gateway)")
	cmd.Flags().StringVar(&agentName, "agent", "default", "Agent config name (from profiles/agent-<name>.yaml)")
	cmd.Flags().StringVarP(&agentProfile, "agent-profile", "f", "", "Path to agent YAML file (overrides --agent)")
	cmd.Flags().StringVar(&sandboxName, "name", "", "Sandbox name (overrides agent config)")
	cmd.Flags().BoolVar(&noTTY, "no-tty", false, "Non-interactive mode (for testing)")
	cmd.Flags().BoolVar(&providerRefresh, "provider-refresh", false, "Delete and recreate all providers")

	return cmd
}

type upLocalOpts struct {
	harnessDir      string
	gw              gateway.Gateway
	gwCfg           *gateway.GatewayConfig
	ensureLocal     bool
	agentCfg        *agent.AgentConfig
	agentPath       string
	sandboxName     string
	noTTY           bool
	providerRefresh bool
	harness         *agent.Harness
	retrySleep      time.Duration
}

func upLocal(opts upLocalOpts) error {
	gw := opts.gw

	// 1. Parse agent config
	agentCfg := opts.agentCfg
	if agentCfg == nil {
		var err error
		agentCfg, err = agent.ParseFile(opts.agentPath)
		if err != nil {
			return err
		}
	}
	sandboxName := agentCfg.Name
	if opts.sandboxName != "" {
		sandboxName = opts.sandboxName
	}
	noTTY := opts.noTTY || agentCfg.NoTTY()

	sandboxImage := resolveSandboxImage(agentCfg.Image)

	// Top-level context
	status.Infof("Agent: %s (%s)", sandboxName, filepath.Base(opts.agentPath))
	status.Infof("Image: %s", sandboxImage)
	if agentCfg.Task != "" {
		status.Infof("Task:  %s", agentCfg.Task)
	}

	// 2. Ensure gateway
	if opts.ensureLocal {
		if err := deployLocal(gw); err != nil {
			return fmt.Errorf("deploy failed: %w", err)
		}
	} else if gw.InferenceGet() != nil {
		if opts.gwCfg == nil {
			return fmt.Errorf("no active gateway — use --gateway local or: harness deploy ocp")
		}
		kc := k8s.New("", k8s.DefaultNamespace())
		clusterRunner := k8s.New("", "")
		if err := deployFromConfig(opts.harnessDir, opts.gwCfg, gw, kc, clusterRunner); err != nil {
			return fmt.Errorf("deploy failed: %w", err)
		}
	}

	// 3. Ensure providers needed by the agent are registered
	registered := ensureProviders(opts.harnessDir, gw, agentCfg, opts.providerRefresh, opts.harness)

	// 4. Render payload
	payloadDir, err := os.MkdirTemp("", "harness-payload-")
	if err != nil {
		return fmt.Errorf("creating payload dir: %w", err)
	}
	defer os.RemoveAll(payloadDir)

	if err := agent.RenderPayload(agentCfg, opts.harnessDir, payloadDir); err != nil {
		return fmt.Errorf("rendering payload: %w", err)
	}

	// 5. Create sandbox
	status.Header("Sandbox")
	var sandboxCmd []string
	if noTTY {
		sandboxCmd = []string{"true"}
	} else {
		sandboxCmd = []string{"bash", "/sandbox/.config/openshell/run.sh"}
	}

	return createSandbox(sandboxOpts{
		harnessDir: opts.harnessDir,
		gw:         gw,
		name:       sandboxName,
		image:      sandboxImage,
		providers:  registered,
		noTTY:      noTTY,
		retrySleep: opts.retrySleep,
		sandboxCmd: sandboxCmd,
		payloadDir: payloadDir,
		env:        agentCfg.BuildEnvMap(),
	})
}

var Version = "dev"

// DefaultAgentConfig holds the embedded default agent YAML, set from main.go.
var DefaultAgentConfig []byte

