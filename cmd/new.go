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
	"github.com/robbycochran/harness-openshell/internal/profile"
	"github.com/robbycochran/harness-openshell/internal/runner"
	"github.com/robbycochran/harness-openshell/internal/status"
	"github.com/spf13/cobra"
)

func NewNewCmd(harnessDir, cli string) *cobra.Command {
	var (
		local       bool
		remote      bool
		profileName string
		sandboxName string
		noTTY       bool
	)

	cmd := &cobra.Command{
		Use:   "new [flags]",
		Short: "Create a new sandbox",
		Long:  "Deploy gateway and providers if needed, then create a sandbox from a profile.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 && sandboxName == "" {
				sandboxName = args[0]
			}

			gw := gateway.NewCLI(cli)

			if remote {
				return newRemote(harnessDir, gw, profileName, sandboxName)
			}
			return newLocal(newLocalOpts{
				harnessDir:  harnessDir,
				gw:          gw,
				ensureLocal: local,
				profileName: profileName,
				sandboxName: sandboxName,
				noTTY:       noTTY,
				runScript: func(name string, args ...string) error {
					return runner.RunScript(harnessDir, name, args...)
				},
				retrySleep: 5 * time.Second,
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

type newLocalOpts struct {
	harnessDir  string
	gw          gateway.Gateway
	ensureLocal bool
	profileName string
	sandboxName string
	noTTY       bool
	runScript   func(name string, args ...string) error
	retrySleep  time.Duration
}

func newRemote(harnessDir string, gw gateway.Gateway, profileName, sandboxName string) error {
	ctx := context.Background()
	namespace := os.Getenv("OPENSHELL_NAMESPACE")
	if namespace == "" {
		namespace = "openshell"
	}

	// 1. Ensure gateway
	if err := gw.InferenceGet(); err != nil {
		fmt.Println("=== Deploying gateway ===")
		if err := deployRemote(harnessDir, gw, ""); err != nil {
			return fmt.Errorf("deploy failed: %w", err)
		}
	}

	// 2. Ensure providers
	providers, _ := gw.ProviderList()
	if len(providers) == 0 {
		fmt.Println("\n=== Registering providers ===")
		if err := registerProviders(harnessDir, gw, false); err != nil {
			return fmt.Errorf("provider registration failed: %w", err)
		}
	}

	// 3. Ensure credentials
	if err := ensureCreds(namespace, false); err != nil {
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

	kc := k8s.New("", namespace)
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
	kc.RunKubectl(ctx, "delete", "job", jobName, "--force", "--grace-period=0")
	kc.RunKubectl(ctx, "delete", "pod", "-l", "job-name="+jobName, "--force", "--grace-period=0")

	// 4. Apply launcher Job
	job := map[string]any{
		"apiVersion": "batch/v1",
		"kind":       "Job",
		"metadata":   map[string]any{"name": jobName, "namespace": namespace},
		"spec": map[string]any{
			"backoffLimit": 0,
			"template": map[string]any{
				"spec": map[string]any{
					"serviceAccountName": "openshell-launcher",
					"restartPolicy":      "Never",
					"containers": []map[string]any{{
						"name":            "launcher",
						"image":           "quay.io/rcochran/openshell:launcher",
						"imagePullPolicy": "Always",
						"env": []map[string]any{
							{"name": "GATEWAY_ENDPOINT", "value": "https://openshell.openshell.svc.cluster.local:8080"},
							{"name": "HOME", "value": "/tmp"},
						},
						"volumeMounts": []map[string]any{
							{"name": "config", "mountPath": "/etc/openshell/sandbox", "readOnly": true},
							{"name": "gws", "mountPath": "/secrets/gws", "readOnly": true},
							{"name": "gateway-mtls", "mountPath": "/secrets/mtls", "readOnly": true},
							{"name": "sandbox-env", "mountPath": "/etc/openshell/env", "readOnly": true},
						},
					}},
					"volumes": []map[string]any{
						{"name": "config", "configMap": map[string]any{"name": "sandbox-" + cfg.Name}},
						{"name": "gws", "secret": map[string]any{"secretName": "openshell-gws", "optional": true}},
						{"name": "gateway-mtls", "secret": map[string]any{"secretName": "openshell-client-tls"}},
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
	fmt.Println("Waiting for launcher...")
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
		jobStatus, _ = kc.RunKubectl(ctx, "get", "job", jobName,
			"-o", "jsonpath={.status.conditions[0].type}")
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
		fmt.Printf("Sandbox ready. Connect with: harness connect %s\n", cfg.Name)
		return nil
	}
	if jobStatus == "" {
		return fmt.Errorf("launcher job timed out — check: kubectl logs -n %s -l job-name=%s", namespace, jobName)
	}
	return fmt.Errorf("launcher job failed (status: %s) — check: kubectl logs -n %s -l job-name=%s", jobStatus, namespace, jobName)
}

func newLocal(opts newLocalOpts) error {
	gw := opts.gw

	// 1. Ensure gateway
	if opts.ensureLocal {
		fmt.Println("=== Ensuring local gateway ===")
		if err := opts.runScript("deploy.sh", "--local"); err != nil {
			return fmt.Errorf("deploy failed: %w", err)
		}
	} else {
		if err := gw.InferenceGet(); err != nil {
			return fmt.Errorf("no active gateway — use --local or --remote")
		}
	}

	// 2. Ensure providers
	providers, _ := gw.ProviderList()
	if len(providers) == 0 {
		fmt.Println("\n=== Registering providers ===")
		if err := opts.runScript("providers.sh"); err != nil {
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

	fmt.Println()
	fmt.Println("=== Sandbox ===")
	fmt.Printf("  Profile: %s\n", opts.profileName)
	fmt.Printf("  Image:   %s\n", cfg.Image)

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
		fmt.Println("WARNING: no providers available. Run: harness providers")
	}

	// 5. Stage files
	harnessUploadDir := "/tmp/openshell"
	if err := os.RemoveAll(harnessUploadDir); err != nil {
		return fmt.Errorf("cleaning staging dir: %w", err)
	}
	if err := profile.StageHarnessDir(cfg, harnessUploadDir); err != nil {
		return fmt.Errorf("staging files: %w", err)
	}

	// 6. Build command
	var sandboxCmd []string
	if opts.noTTY {
		sandboxCmd = []string{"bash", "/sandbox/startup.sh"}
	} else {
		sandboxCmd = []string{"bash", "-c", fmt.Sprintf(". /sandbox/startup.sh && exec %s", cfg.Command)}
	}

	// 7. Create sandbox with retry
	fmt.Println()
	fmt.Println("=== Creating sandbox ===")
	for attempt := 1; attempt <= 5; attempt++ {
		err := gw.SandboxCreate(gateway.SandboxCreateOpts{
			Name:      cfg.Name,
			Image:     cfg.Image,
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
	return nil
}
