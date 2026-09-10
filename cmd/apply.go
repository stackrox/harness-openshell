package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/stackrox/harness-openshell/internal/openshell"
)

func NewApplyCmd(newClient openshell.Factory) *cobra.Command {
	var file, sandboxName, entrypoint, output, resultFile string
	var attach, dryRun, setupOnly bool
	var gatewayName, workspace *string

	cmd := &cobra.Command{
		Use:   "apply [FILE] [flags]",
		Short: "Apply a harness configuration",
		Long: `Resolve a version 1 workflow and execute its
planned reconciliation and sandbox run. Provision the gateway and referenced
providers with OpenShell first. Use --dry-run to render the action plan without
mutating anything, or -o yaml to output the resolved configuration with
host-interpolated and credential-bearing map values redacted.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				if file != "" {
					return fmt.Errorf("workflow file specified both as an argument and with --file")
				}
				file = args[0]
			}
			return runApply(cmd.Context(), newClient, applyRequest{
				File:       file,
				Name:       sandboxName,
				Entrypoint: entrypoint,
				Attach:     attach,
				DryRun:     dryRun,
				SetupOnly:  setupOnly,
				Output:     output,
				ResultFile: resultFile,
				Gateway:    *gatewayName,
				Workspace:  *workspace,
			}, cmd.ErrOrStderr())
		},
	}

	cmd.Flags().StringVarP(&file, "file", "f", "", "Path to harness YAML file (or pass it as the first argument)")
	cmd.Flags().StringVar(&sandboxName, "name", "", "Override the sandbox name")
	cmd.Flags().StringVar(&entrypoint, "entrypoint", "", "Override the agent executable")
	cmd.Flags().BoolVar(&attach, "attach", false, "Attach a TTY for interactive execution")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Render the action plan without mutating anything")
	cmd.Flags().BoolVar(&setupOnly, "setup-only", false, "Verify provider references and configure inference without running a sandbox")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Output format: yaml or json (dry-run also supports table)")
	cmd.Flags().StringVar(&resultFile, "result-file", "", "Write host-derived execution result JSON to a new file")
	gatewayName, workspace = registerTargetFlags(cmd)
	return cmd
}
