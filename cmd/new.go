package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/robbycochran/harness-openshell/internal/gateway"
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

			if remote {
				return newRemote(harnessDir, profileName, sandboxName, noTTY)
			}

			gw := gateway.NewCLI(cli)
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

func newRemote(harnessDir, profileName, sandboxName string, noTTY bool) error {
	args := []string{"--remote", "--profile", profileName}
	if sandboxName != "" {
		args = append(args, "--name", sandboxName)
	}
	if noTTY {
		args = append(args, "--no-tty")
	}
	return runner.RunScript(harnessDir, "new.sh", args...)
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
	os.RemoveAll(harnessUploadDir)
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

		fmt.Printf("  Attempt %d failed (supervisor race), retrying in 5s...\n", attempt)
		gw.SandboxDelete(cfg.Name)

		if attempt == 5 {
			return fmt.Errorf("failed after 5 attempts")
		}
		time.Sleep(opts.retrySleep)
	}
	return nil
}
