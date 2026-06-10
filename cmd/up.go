package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/robbycochran/harness-openshell/internal/agent"
	"github.com/robbycochran/harness-openshell/internal/gateway"
	"github.com/robbycochran/harness-openshell/internal/k8s"
	"github.com/robbycochran/harness-openshell/internal/profile"
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

			gwName := "local"
			if remote {
				gwName = "ocp"
			}
			gwDir := filepath.Join(harnessDir, "gateways", gwName)
			gwCfg, _ := gateway.LoadConfig(gwDir)

			if remote {
				return upRemote(harnessDir, gwCfg, gw, agentPath, sandboxName)
			}
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

func upRemote(harnessDir string, gwCfg *gateway.GatewayConfig, gw gateway.Gateway, agentPath, sandboxName string) error {
	ctx := context.Background()
	namespace := k8s.DefaultNamespace()
	kc := k8s.New("", namespace)
	clusterRunner := k8s.New("", "")

	// Parse agent config early so we can show context
	agentCfg, err := agent.ParseFile(agentPath)
	if err != nil {
		return err
	}
	name := agentCfg.Name
	if sandboxName != "" {
		name = sandboxName
	}

	sandboxImage := resolveSandboxImage(agentCfg.Image)

	// Top-level context
	status.Infof("Agent: %s (%s)", name, filepath.Base(agentPath))
	status.Infof("Image: %s", sandboxImage)

	// 1. Ensure gateway and namespace
	gwReachable := gw.InferenceGet() == nil
	_, nsErr := kc.RunKubectl(ctx, "get", "namespace", namespace)
	nsExists := nsErr == nil
	if !gwReachable || !nsExists {
		if gwCfg == nil {
			return fmt.Errorf("no active gateway and no gateway config — use: harness deploy ocp")
		}
		if err := deployFromConfig(harnessDir, gwCfg, gw, kc, clusterRunner); err != nil {
			return fmt.Errorf("deploy failed: %w", err)
		}
	}

	// 2. Ensure providers needed by the agent
	providerNames := agentCfg.ProviderNames()
	if len(providerNames) > 0 {
		_, missing := profile.ValidateProviders(providerNames, gw)
		if len(missing) > 0 {
			if err := registerProviders(harnessDir, gw, false, gwCfg, false); err != nil {
				return fmt.Errorf("provider registration failed: %w", err)
			}
		}
	}

	// 4. ConfigMap from agent.yaml
	out, err := kc.RunKubectl(ctx, "create", "configmap", "sandbox-"+name,
		"--from-file=agent.yaml="+agentPath,
		"--dry-run=client", "-o", "yaml")
	if err != nil {
		return fmt.Errorf("creating config configmap: %w", err)
	}
	if _, err := kc.RunKubectlOpts(ctx, k8s.KubectlOpts{
		Args:  []string{"apply", "-f", "-"},
		Stdin: strings.NewReader(out),
	}); err != nil {
		return fmt.Errorf("applying config configmap: %w", err)
	}

	// 5. Clean up old job
	jobName := "sandbox-" + name
	kc.RunKubectl(ctx, "delete", "job", jobName, "--grace-period=30")
	kc.RunKubectl(ctx, "delete", "pod", "-l", "job-name="+jobName, "--grace-period=30")

	// 6. Apply runner Job (gwCfg provides defaults + RUNNER_IMAGE env override)
	runnerImage := defaultRunnerImage()
	runnerSA := "openshell-launcher"
	gatewayEndpoint := "https://openshell.openshell.svc.cluster.local:8080"
	mtlsSecret := "openshell-client-tls"
	if gwCfg != nil {
		if gwCfg.Images.Runner != "" {
			runnerImage = gwCfg.Images.Runner
		}
		runnerSA = gwCfg.Launcher.ServiceAccount
		gatewayEndpoint = gwCfg.Launcher.GatewayEndpoint
		mtlsSecret = gwCfg.Secrets.MTLS
	}

	job := map[string]any{
		"apiVersion": "batch/v1",
		"kind":       "Job",
		"metadata":   map[string]any{"name": jobName, "namespace": namespace},
		"spec": map[string]any{
			"backoffLimit": 0,
			"template": map[string]any{
				"spec": map[string]any{
					"serviceAccountName": runnerSA,
					"restartPolicy":      "Never",
					"containers": []map[string]any{{
						"name":            "runner",
						"image":           runnerImage,
						"imagePullPolicy": "Always",
						"command":         []string{"harness", "launch"},
						"env":             runnerEnv(gatewayEndpoint, sandboxImage),
						"volumeMounts": []map[string]any{
							{"name": "config", "mountPath": "/etc/openshell/sandbox", "readOnly": true},
							{"name": "gateway-mtls", "mountPath": "/secrets/mtls", "readOnly": true},
						},
					}},
					"volumes": []map[string]any{
						{"name": "config", "configMap": map[string]any{"name": "sandbox-" + name}},
						{"name": "gateway-mtls", "secret": map[string]any{"secretName": mtlsSecret}},
					},
				},
			},
		},
	}
	if err := kc.ApplyYAML(ctx, job); err != nil {
		return fmt.Errorf("applying runner job: %w", err)
	}

	// 7. Wait for runner pod
	status.Header("Sandbox")
	status.Info("Waiting for runner...")
	kc.RunKubectl(ctx, "wait", "--for=condition=ready", "pod",
		"-l", "job-name="+jobName, "--timeout=120s")

	// 8. Tail logs in background
	logCmd := exec.CommandContext(ctx, "kubectl", "-n", namespace,
		"logs", "-f", "-l", "job-name="+jobName)
	logCmd.Stdout = os.Stdout
	logCmd.Stderr = os.Stderr
	logCmd.Start()

	// 9. Poll job status (10 min timeout)
	var jobStatus string
	deadline := time.Now().Add(15 * time.Minute)
	for time.Now().Before(deadline) {
		jobStatus, err = kc.RunKubectl(ctx, "get", "job", jobName,
			"-o", "jsonpath={.status.conditions[0].type}")
		if err != nil {
			return fmt.Errorf("checking runner job status: %w", err)
		}
		if jobStatus == "Complete" || jobStatus == "Failed" || jobStatus == "SuccessCriteriaMet" {
			break
		}
		time.Sleep(2 * time.Second)
	}

	if logCmd.Process != nil {
		logCmd.Process.Kill()
		logCmd.Wait()
	}

	if jobStatus == "Complete" || jobStatus == "SuccessCriteriaMet" {
		fmt.Println()
		status.OKf("Connect with: harness connect %s", name)
		return nil
	}
	if jobStatus == "" {
		return fmt.Errorf("runner job timed out — check: kubectl logs -n %s -l job-name=%s", namespace, jobName)
	}
	return fmt.Errorf("runner job failed (status: %s) — check: kubectl logs -n %s -l job-name=%s", jobStatus, namespace, jobName)
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
	} else {
		if err := gw.InferenceGet(); err != nil {
			return fmt.Errorf("no active gateway — use --local or --remote")
		}
	}

	// 3. Ensure providers needed by the agent are registered
	providerNames := agentCfg.ProviderNames()
	var registered []string
	if len(providerNames) > 0 {
		var missing []string
		registered, missing = profile.ValidateProviders(providerNames, gw)
		if len(missing) > 0 {
			if err := registerProviders(opts.harnessDir, gw, false, opts.gwCfg, false); err != nil {
				status.Warn(fmt.Sprintf("provider registration: %v", err))
			}
			registered, missing = profile.ValidateProviders(providerNames, gw)
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

	cfg := &profile.Config{
		Name: sandboxName,
		From: sandboxImage,
	}

	// 5. Create sandbox
	status.Header("Sandbox")
	envInit := ". /sandbox/.config/openshell/sandbox.env 2>/dev/null && " +
		"cat /sandbox/.config/openshell/sandbox.env >> /sandbox/.bashrc 2>/dev/null; "
	var sandboxCmd []string
	if noTTY {
		sandboxCmd = []string{"bash", "-c", envInit + "true"}
	} else {
		sandboxCmd = []string{"bash", "-c", envInit + "exec bash /sandbox/.config/openshell/run.sh"}
	}

	return createSandbox(sandboxOpts{
		harnessDir: opts.harnessDir,
		gw:         gw,
		cfg:        cfg,
		providers:  registered,
		noTTY:      noTTY,
		retrySleep: opts.retrySleep,
		sandboxCmd: sandboxCmd,
		payloadDir: payloadDir,
	})
}

var Version = "dev"

// DefaultAgentConfig holds the embedded default agent YAML, set from main.go.
var DefaultAgentConfig []byte

func defaultSandboxImage() string {
	if v := os.Getenv("SANDBOX_IMAGE"); v != "" {
		return v
	}
	return versionedImage("sandbox")
}

func defaultRunnerImage() string {
	if v := os.Getenv("RUNNER_IMAGE"); v != "" {
		return v
	}
	return versionedImage("runner")
}

func versionedImage(name string) string {
	base := "ghcr.io/robbycochran/harness-openshell"
	if Version == "" || Version == "dev" {
		return base + ":" + name
	}
	return base + ":" + name + "-" + Version
}

func runnerEnv(gatewayEndpoint, sandboxImage string) []map[string]any {
	env := []map[string]any{
		{"name": "GATEWAY_ENDPOINT", "value": gatewayEndpoint},
		{"name": "HOME", "value": "/tmp"},
	}
	if sandboxImage != "" {
		env = append(env, map[string]any{"name": "SANDBOX_IMAGE", "value": sandboxImage})
	}
	return env
}
