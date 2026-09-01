package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/stackrox/harness-openshell/internal/openshell"
)

func NewApplyCmd(newClient openshell.Factory) *cobra.Command {
	var file, sandboxName, entrypoint, output string
	var attach, dryRun, setupOnly bool
	var gatewayName, workspace *string

	cmd := &cobra.Command{
		Use:   "apply [name] [flags]",
		Short: "Apply a harness configuration",
		Long: `Resolve a harness.openshell.dev/v1alpha1 workflow and execute its
planned reconciliation and sandbox run. Provision the gateway and referenced
providers with OpenShell first. Use --dry-run to render the action plan without
mutating anything, or -o yaml to output the resolved configuration with
host-interpolated and credential-bearing map values redacted.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if file == "" {
				return fmt.Errorf("flag -f/--file is required")
			}
			if len(args) == 1 && sandboxName == "" {
				sandboxName = args[0]
			}

			workflow, err := loadWorkflow(file, *gatewayName, *workspace, applyOverrides{
				Name: sandboxName, AgentType: entrypoint, ForceTTY: attach,
			})
			if err != nil {
				return err
			}
			if output != "" && !dryRun {
				return renderWorkflow(workflow, output)
			}

			var client openshell.Client
			// A non-dry-run apply always asks the SDK factory to resolve its target.
			// An empty target means the active CLI-compatible gateway registration;
			// dry-run remains fully offline when no target was requested.
			if !dryRun || workflow.Target.Direct != nil || workflow.Target.Gateway != "" {
				client, err = newClient(cmd.Context(), workflow.Target)
				if err != nil {
					desc := targetDescription(workflow.Target)
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
			return applyWorkflow(cmd.Context(), workflow, planned, current, client, applyOptions{
				SetupOnly: setupOnly, DryRun: dryRun, Output: output,
			})
		},
	}

	cmd.Flags().StringVarP(&file, "file", "f", "", "Path to harness YAML file")
	cmd.Flags().StringVar(&sandboxName, "name", "", "Override the sandbox name")
	cmd.Flags().StringVar(&entrypoint, "entrypoint", "", "Override the agent executable")
	cmd.Flags().BoolVar(&attach, "attach", false, "Attach a TTY for interactive execution")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Render the action plan without mutating anything")
	cmd.Flags().BoolVar(&setupOnly, "setup-only", false, "Reconcile providers and inference without running a sandbox")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Output format: yaml or json (dry-run also supports table)")
	gatewayName, workspace = registerTargetFlags(cmd)
	return cmd
}
