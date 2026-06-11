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
		remote      bool
		agentName   string
		agentFile   string
		sandboxName string
		noTTY       bool
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
			}
			gwDir := filepath.Join(harnessDir, "gateways", gwName)
			gwCfg, _ := gateway.LoadConfig(gwDir)

			return upLocal(upLocalOpts{
				harnessDir:  harnessDir,
				gw:          gw,
				gwCfg:       gwCfg,
				ensureLocal: !remote,
				agentCfg:    agentCfg,
				agentPath:   agentPath,
				sandboxName: sandboxName,
				noTTY:       noTTY,
				retrySleep:  5 * time.Second,
			})
		},
	}

	cmd.Flags().BoolVar(&local, "local", false, "Ensure local podman gateway (default when --remote is not specified)")
	cmd.Flags().BoolVar(&remote, "remote", false, "Ensure OCP gateway")
	cmd.Flags().StringVar(&agentName, "agent", "default", "Agent config name (from agents/)")
	cmd.Flags().StringVarP(&agentFile, "file", "f", "", "Path to agent YAML file (overrides --agent)")
	cmd.Flags().StringVar(&sandboxName, "name", "", "Sandbox name (overrides agent config)")
	cmd.Flags().BoolVar(&noTTY, "no-tty", false, "Non-interactive mode (for testing)")

	return cmd
}

func resolveAgentPath(harnessDir, agentName, agentFile string) string {
	if agentFile != "" {
		return agentFile
	}
	return filepath.Join(harnessDir, "agents", agentName+".yaml")
}

// resolveAgentConfig parses the agent config from disk, falling back to the
// embedded default when the file does not exist and no explicit --file was given.
func resolveAgentConfig(harnessDir, agentName, agentFile string) (*agent.AgentConfig, error) {
	path := resolveAgentPath(harnessDir, agentName, agentFile)
	cfg, err := agent.ParseFile(path)
	if err == nil {
		return cfg, nil
	}
	if agentFile != "" || agentName != "default" || len(DefaultAgentConfig) == 0 {
		return nil, err
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		return nil, err
	}
	return agent.Parse(DefaultAgentConfig)
}

type upLocalOpts struct {
	harnessDir  string
	gw          gateway.Gateway
	gwCfg       *gateway.GatewayConfig
	ensureLocal bool
	agentCfg    *agent.AgentConfig
	agentPath   string
	sandboxName string
	noTTY       bool
	retrySleep  time.Duration
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
	providerNames := agentCfg.ProviderNames()
	var registered []string
	if len(providerNames) > 0 {
		var missing []string
		registered, missing = gateway.ValidateProviders(providerNames, gw)
		if len(missing) > 0 {
			if err := registerProviders(opts.harnessDir, gw, false, opts.gwCfg, false); err != nil {
				status.Warn(fmt.Sprintf("provider registration: %v", err))
			}
			registered, missing = gateway.ValidateProviders(providerNames, gw)
		}
		status.Header("Providers")
		for _, name := range registered {
			status.OKf("%s", name)
		}
		for _, name := range missing {
			status.Failf("%s (not registered)", name)
		}
	}

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

func versionedImage(name string) string {
	base := "ghcr.io/robbycochran/harness-openshell"
	if Version == "" || Version == "dev" {
		return base + ":" + name
	}
	return base + ":" + name + "-" + Version
}
