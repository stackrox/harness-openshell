package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/robbycochran/harness-openshell/internal/agent"
	"github.com/robbycochran/harness-openshell/internal/gateway"
	"github.com/robbycochran/harness-openshell/internal/status"
	"github.com/spf13/cobra"
)

func NewCreateCmd(harnessDir, cli string) *cobra.Command {
	var (
		agentName   string
		agentProfile string
		sandboxName string
	)

	cmd := &cobra.Command{
		Use:   "create [flags]",
		Short: "Create a sandbox without attaching",
		Long:  "Validate gateway readiness, run preflight checks, and deploy a sandbox. Does not attach interactively — use 'openshell sandbox connect' afterward.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 && sandboxName == "" {
				sandboxName = args[0]
			}

			agentCfg, err := resolveAgentConfig(harnessDir, agentName, agentProfile)
			if err != nil {
				return err
			}

			gw := gateway.New(cli)

			// 1. Check which gateway is active.
			activeGW, err := activeGatewayInfo(gw)
			if err != nil {
				return err
			}

			status.Header("Gateway")
			status.OKf("%s (%s)", activeGW.Name, activeGW.Endpoint)
			name := agentCfg.Name
			if sandboxName != "" {
				name = sandboxName
			}

			sandboxImage := resolveSandboxImage(agentCfg.Image)

			status.Header("Agent")
			status.Infof("Name:  %s", name)
			status.Infof("Image: %s", sandboxImage)

			// 3. Ensure providers are registered
			registered := ensureProviders(harnessDir, gw, agentCfg, false)

			// 4. Deploy the sandbox
			status.Header("Sandbox")
			payloadDir, err := os.MkdirTemp("", "harness-payload-")
			if err != nil {
				return fmt.Errorf("creating payload dir: %w", err)
			}
			defer os.RemoveAll(payloadDir)

			if err := agent.RenderPayload(agentCfg, harnessDir, payloadDir); err != nil {
				return fmt.Errorf("rendering payload: %w", err)
			}

			return createSandbox(sandboxOpts{
				harnessDir: harnessDir,
				gw:         gw,
				name:       name,
				image:      sandboxImage,
				providers:  registered,
				noTTY:      true,
				retrySleep: 5 * time.Second,
				sandboxCmd: []string{"true"},
				payloadDir: payloadDir,
				env:        agentCfg.BuildEnvMap(),
				onSuccess: func(n string) {
					fmt.Println()
					status.OKf("Sandbox created: %s — connect with: openshell sandbox connect %s", n, n)
				},
			})
		},
	}

	cmd.Flags().StringVar(&agentName, "agent", "default", "Agent config name (from agents/)")
	cmd.Flags().StringVarP(&agentProfile, "agent-profile", "f", "", "Path to agent YAML file (overrides --agent)")
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


