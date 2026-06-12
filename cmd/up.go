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
		local       bool
		remote          bool
		agentName       string
		agentFile       string
		sandboxName     string
		noTTY           bool
		providerRefresh bool
	)

	cmd := &cobra.Command{
		Use:   "up [flags]",
		Short: "Deploy gateway, register providers, and create a sandbox",
		Long:  "Deploy gateway and register providers if needed, then render an agent config into a running sandbox.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if local && remote {
				return fmt.Errorf("--local and --remote are mutually exclusive")
			}
			if len(args) > 0 && sandboxName == "" {
				sandboxName = args[0]
			}

			agentCfg, err := resolveAgentConfig(harnessDir, agentName, agentFile)
			if err != nil {
				return err
			}
			agentPath := resolveAgentPath(harnessDir, agentName, agentFile)

			gw := gateway.New(cli)
			if err := gw.CheckMinVersion("0.0.59"); err != nil {
				status.Warn(fmt.Sprintf("OpenShell version: %v", err))
			}

			gwName := "local"
			if remote {
				gwName = "ocp"
			} else if !local && agentCfg.Gateway != "" {
				gwName = agentCfg.Gateway
			}
			isRemote := gwName != "local"
			gwCfg, _ := resolveGatewayConfig(harnessDir, gwName)

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
				retrySleep:      5 * time.Second,
			})
		},
	}

	cmd.Flags().BoolVar(&local, "local", false, "Ensure local podman gateway (default when --remote is not specified)")
	cmd.Flags().BoolVar(&remote, "remote", false, "Ensure OCP gateway")
	cmd.Flags().StringVar(&agentName, "agent", "default", "Agent config name (from agents/)")
	cmd.Flags().StringVarP(&agentFile, "file", "f", "", "Path to agent YAML file (overrides --agent)")
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
			return fmt.Errorf("no active gateway — use --local or: harness deploy ocp")
		}
		kc := k8s.New("", k8s.DefaultNamespace())
		clusterRunner := k8s.New("", "")
		if err := deployFromConfig(opts.harnessDir, opts.gwCfg, gw, kc, clusterRunner); err != nil {
			return fmt.Errorf("deploy failed: %w", err)
		}
	}

	// 3. Ensure providers needed by the agent are registered
	registered := ensureProviders(opts.harnessDir, gw, agentCfg, opts.providerRefresh)

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

