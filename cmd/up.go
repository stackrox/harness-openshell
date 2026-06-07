package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/robbycochran/harness-openshell/internal/gateway"
	"github.com/robbycochran/harness-openshell/internal/k8s"
	"github.com/robbycochran/harness-openshell/internal/preflight"
	"github.com/robbycochran/harness-openshell/internal/profile"
	"github.com/robbycochran/harness-openshell/internal/status"
	"github.com/spf13/cobra"
)

func NewUpCmd(harnessDir, cli string) *cobra.Command {
	var (
		local       bool
		remote      bool
		profileName string
		sandboxName string
		noTTY       bool
	)

	cmd := &cobra.Command{
		Use:   "up [flags]",
		Short: "Deploy gateway, register providers, and create a sandbox",
		Long:  "Deploy gateway and register providers if needed, then create a sandbox from a profile.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if local && remote {
				return fmt.Errorf("--local and --remote are mutually exclusive")
			}
			if len(args) > 0 && sandboxName == "" {
				sandboxName = args[0]
			}

			gw := gateway.New(cli)

			// Load gateway config for the selected target
			gwName := "local"
			if remote {
				gwName = "ocp"
			}
			gwDir := filepath.Join(harnessDir, "gateways", gwName)
			gwCfg, _ := gateway.LoadConfig(gwDir) // nil is fine — backward compat

			if remote {
				return upRemote(harnessDir, gwCfg, gw, profileName, sandboxName)
			}
			return upLocal(upLocalOpts{
				harnessDir:  harnessDir,
				gw:          gw,
				gwCfg:       gwCfg,
				ensureLocal: local,
				profileName: profileName,
				sandboxName: sandboxName,
				noTTY:       noTTY,
				retrySleep:  5 * time.Second,
			})
		},
	}

	cmd.Flags().BoolVar(&local, "local", false, "Ensure local podman gateway")
	cmd.Flags().BoolVar(&remote, "remote", false, "Ensure OCP gateway")
	cmd.Flags().StringVar(&profileName, "profile", "default", "Profile name (from profiles/)")
	cmd.Flags().StringVar(&sandboxName, "name", "", "Sandbox name (overrides profile)")
	cmd.Flags().BoolVar(&noTTY, "no-tty", false, "Non-interactive mode (for testing)")

	return cmd
}

type upLocalOpts struct {
	harnessDir  string
	gw          gateway.Gateway
	gwCfg       *gateway.GatewayConfig
	ensureLocal bool
	profileName string
	sandboxName string
	noTTY       bool
	retrySleep  time.Duration
}

func upRemote(harnessDir string, gwCfg *gateway.GatewayConfig, gw gateway.Gateway, profileName, sandboxName string) error {
	ctx := context.Background()
	namespace := k8s.DefaultNamespace()
	kc := k8s.New("", namespace)
	clusterRunner := k8s.New("", "")

	// 1. Ensure gateway
	if err := gw.InferenceGet(); err != nil {
		if gwCfg == nil {
			return fmt.Errorf("no active gateway and no gateway config — use: harness deploy ocp")
		}
		status.Section("Deploying gateway")
		if err := deployFromConfig(harnessDir, gwCfg, gw, kc, clusterRunner); err != nil {
			return fmt.Errorf("deploy failed: %w", err)
		}
	}

	// 2. Ensure providers
	providers, err := gw.ProviderList()
	if err != nil {
		return fmt.Errorf("listing providers: %w", err)
	}
	if len(providers) == 0 {
		status.Section("Registering providers")
		if err := registerProviders(harnessDir, gw, false, gwCfg); err != nil {
			return fmt.Errorf("provider registration failed: %w", err)
		}
	}

	// 3. Ensure credentials
	if err := ensureCreds(kc, namespace, false); err != nil {
		return fmt.Errorf("credentials setup failed: %w", err)
	}

	// Parse profile
	cfg, err := profile.Parse(harnessDir, profileName)
	if err != nil {
		return err
	}
	if sandboxName != "" {
		cfg.Name = sandboxName
	}

	// Resolve sandbox image for remote deploys.
	// SANDBOX_IMAGE env var overrides everything (dev/CI builds).
	// Otherwise the profile's 'from' field is authoritative — each profile
	// specifies its own image (default.toml uses the custom sandbox,
	// ci.toml uses the upstream community base).
	if envImage := os.Getenv("SANDBOX_IMAGE"); envImage != "" {
		cfg.From = envImage
	} else if cfg.From != "" {
		fromPath := cfg.From
		if !filepath.IsAbs(fromPath) {
			fromPath = filepath.Join(harnessDir, fromPath)
		}
		if info, err := os.Stat(fromPath); err == nil && info.IsDir() {
			cfg.From = envOr("SANDBOX_IMAGE", "ghcr.io/robbycochran/harness-openshell:sandbox")
		}
	}

	profilePath := filepath.Join(harnessDir, "profiles", profileName+".toml")

	// 1. ConfigMap from profile
	out, err := kc.RunKubectl(ctx, "create", "configmap", "sandbox-"+cfg.Name,
		"--from-file=config.toml="+profilePath,
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

	// 2. ConfigMap from env (conditional)
	envContent := cfg.BuildSandboxEnv()
	if envContent != "" {
		out, err := kc.RunKubectl(ctx, "create", "configmap", "sandbox-"+cfg.Name+"-env",
			"--from-literal=sandbox.env="+envContent,
			"--dry-run=client", "-o", "yaml")
		if err == nil {
			if _, err := kc.RunKubectlOpts(ctx, k8s.KubectlOpts{
				Args:  []string{"apply", "-f", "-"},
				Stdin: strings.NewReader(out),
			}); err != nil {
				return fmt.Errorf("applying env configmap: %w", err)
			}
		}
	}

	// 3. Clean up old job (best-effort — may not exist)
	jobName := "sandbox-" + cfg.Name
	kc.RunKubectl(ctx, "delete", "job", jobName, "--grace-period=30")
	kc.RunKubectl(ctx, "delete", "pod", "-l", "job-name="+jobName, "--grace-period=30")

	// 4. Apply launcher Job
	launcherImage := envOr("LAUNCHER_IMAGE", "ghcr.io/robbycochran/harness-openshell:launcher")
	launcherSA := "openshell-launcher"
	launcherEndpoint := "https://openshell.openshell.svc.cluster.local:8080"
	mtlsSecret := "openshell-client-tls"
	if gwCfg != nil {
		launcherImage = gwCfg.Images.Launcher
		launcherSA = gwCfg.Launcher.ServiceAccount
		launcherEndpoint = gwCfg.Launcher.GatewayEndpoint
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
					"serviceAccountName": launcherSA,
					"restartPolicy":      "Never",
					"containers": []map[string]any{{
						"name":            "launcher",
						"image":           launcherImage,
						"imagePullPolicy": "Always",
						"env": launcherEnv(launcherEndpoint, cfg.From),
						"volumeMounts": []map[string]any{
							{"name": "config", "mountPath": "/etc/openshell/sandbox", "readOnly": true},
							{"name": "gateway-mtls", "mountPath": "/secrets/mtls", "readOnly": true},
							{"name": "sandbox-env", "mountPath": "/etc/openshell/env", "readOnly": true},
						},
					}},
					"volumes": []map[string]any{
						{"name": "config", "configMap": map[string]any{"name": "sandbox-" + cfg.Name}},
						{"name": "gateway-mtls", "secret": map[string]any{"secretName": mtlsSecret}},
						{"name": "sandbox-env", "configMap": map[string]any{"name": "sandbox-" + cfg.Name + "-env", "optional": true}},
					},
				},
			},
		},
	}
	if err := kc.ApplyYAML(ctx, job); err != nil {
		return fmt.Errorf("applying launcher job: %w", err)
	}

	// 5. Wait for launcher pod
	fmt.Println()
	status.Info("Waiting for launcher...")
	kc.RunKubectl(ctx, "wait", "--for=condition=ready", "pod",
		"-l", "job-name="+jobName, "--timeout=120s")

	// 6. Tail logs in background
	logCmd := exec.CommandContext(ctx, "kubectl", "-n", namespace,
		"logs", "-f", "-l", "job-name="+jobName)
	logCmd.Stdout = os.Stdout
	logCmd.Stderr = os.Stderr
	logCmd.Start()

	// 7. Poll job status (10 min timeout)
	var jobStatus string
	deadline := time.Now().Add(10 * time.Minute)
	for time.Now().Before(deadline) {
		jobStatus, err = kc.RunKubectl(ctx, "get", "job", jobName,
			"-o", "jsonpath={.status.conditions[0].type}")
		if err != nil {
			return fmt.Errorf("checking launcher job status: %w", err)
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

	fmt.Println()
	if jobStatus == "Complete" || jobStatus == "SuccessCriteriaMet" {
		status.OKf("Sandbox ready. Connect with: harness connect %s", cfg.Name)
		return nil
	}
	if jobStatus == "" {
		return fmt.Errorf("launcher job timed out — check: kubectl logs -n %s -l job-name=%s", namespace, jobName)
	}
	return fmt.Errorf("launcher job failed (status: %s) — check: kubectl logs -n %s -l job-name=%s", jobStatus, namespace, jobName)
}

func upLocal(opts upLocalOpts) error {
	gw := opts.gw

	// 1. Ensure gateway
	if opts.ensureLocal {
		fmt.Println("=== Ensuring local gateway ===")
		if err := deployLocal(gw); err != nil {
			return fmt.Errorf("deploy failed: %w", err)
		}
	} else {
		if err := gw.InferenceGet(); err != nil {
			return fmt.Errorf("no active gateway — use --local or --remote")
		}
	}

	// 2. Ensure providers
	providers, err := gw.ProviderList()
	if err != nil {
		return fmt.Errorf("listing providers: %w", err)
	}
	if len(providers) == 0 {
		status.Section("Registering providers")
		if err := registerProviders(opts.harnessDir, gw, false, opts.gwCfg); err != nil {
			return fmt.Errorf("provider registration failed: %w", err)
		}
	}

	// 3. Parse profile
	cfg, err := profile.Parse(opts.harnessDir, opts.profileName)
	if err != nil {
		return err
	}
	if opts.sandboxName != "" {
		cfg.Name = opts.sandboxName
	}

	// Resolve Dockerfile path relative to harnessDir
	if cfg.From != "" && !filepath.IsAbs(cfg.From) {
		candidate := filepath.Join(opts.harnessDir, cfg.From)
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			cfg.From = candidate
		}
	}

	fmt.Println()
	fmt.Println("=== Sandbox ===")
	fmt.Printf("  Profile: %s\n", opts.profileName)
	fmt.Printf("  From:    %s\n", cfg.From)

	// 4. Validate providers against profile
	status.Section("Providers")
	registered, missing := profile.ValidateProviders(cfg.Providers, gw)
	for _, name := range registered {
		status.OKf("%s: attached", name)
	}
	for _, name := range missing {
		status.Failf("%s: not registered (skipping)", name)
	}
	if len(missing) > 0 && len(registered) == 0 {
		fmt.Println()
		status.Warn("no providers available — run: harness providers")
	}

	// 5. Inject non-secret provider env vars into sandbox env
	providersPath := filepath.Join(opts.harnessDir, "providers.toml")
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

	// 6. Stage files
	// The upload preserves the source dir name as a subdirectory at the destination.
	// startup.sh expects files at /sandbox/.config/openshell/sandbox.env, so the
	// staging dir must be named "openshell".
	tmpParent, err := os.MkdirTemp("", "harness-")
	if err != nil {
		return fmt.Errorf("creating staging dir: %w", err)
	}
	defer os.RemoveAll(tmpParent)
	harnessUploadDir := filepath.Join(tmpParent, "openshell")
	if err := profile.StageHarnessDir(cfg, harnessUploadDir); err != nil {
		return fmt.Errorf("staging files: %w", err)
	}

	// 6. Build command
	var sandboxCmd []string
	if cfg.Startup != "" {
		if opts.noTTY {
			sandboxCmd = []string{"bash", "-c", fmt.Sprintf(". %s", cfg.Startup)}
		} else {
			sandboxCmd = []string{"bash", "-c", fmt.Sprintf(". %s && exec %s", cfg.Startup, cfg.Command)}
		}
	} else {
		if opts.noTTY {
			sandboxCmd = []string{"true"}
		} else {
			sandboxCmd = []string{"bash", "-c", fmt.Sprintf("exec %s", cfg.Command)}
		}
	}

	// 7. Create sandbox with retry
	fmt.Println()
	fmt.Println("=== Creating sandbox ===")
	for attempt := 1; attempt <= 5; attempt++ {
		err := gw.SandboxCreate(gateway.SandboxCreateOpts{
			Name:      cfg.Name,
			From:      cfg.From,
			Providers: registered,
			TTY:       !opts.noTTY,
			Keep:      cfg.KeepSandbox(),
			UploadSrc: harnessUploadDir,
			UploadDst: "/sandbox/.config",
			Command:   sandboxCmd,
		})
		if err == nil {
			return nil
		}

		fmt.Printf("  Attempt %d failed: %v, retrying in 5s...\n", attempt, err)
		gw.SandboxDelete(cfg.Name) // best-effort cleanup

		if attempt == 5 {
			return fmt.Errorf("sandbox create failed after 5 attempts: %w", err)
		}
		time.Sleep(opts.retrySleep)
	}
	return nil // unreachable but required by compiler
}

func launcherEnv(gatewayEndpoint, sandboxImage string) []map[string]any {
	env := []map[string]any{
		{"name": "GATEWAY_ENDPOINT", "value": gatewayEndpoint},
		{"name": "HOME", "value": "/tmp"},
	}
	if sandboxImage != "" {
		env = append(env, map[string]any{"name": "SANDBOX_IMAGE", "value": sandboxImage})
	}
	return env
}
