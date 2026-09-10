package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/stackrox/harness-openshell/internal/openshell"
)

// NewPlanCmd constructs the "harness workflow plan" command.
// It reads a config file, resolves environment variables, connects to the gateway
// (if specified), reads the current state, builds a reconciliation plan, and renders it.
func NewPlanCmd(newClient openshell.Factory) *cobra.Command {
	var (
		file   string
		output string
	)
	// Assigned by registerTargetFlags below; RunE reads them at execution time.
	var gatewayName, workspace *string

	cmd := &cobra.Command{
		Use:   "plan [FILE] [flags]",
		Short: "Read-only reconciliation plan",
		Long: `Generate a reconciliation plan showing the actions harness would take.

This is a read-only plan and mutates nothing. Apply
uses this same resolved desired object and action-decision engine.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				if file != "" {
					return fmt.Errorf("workflow file specified both as an argument and with --file")
				}
				file = args[0]
			}
			format, err := parseOutputFormat(output)
			if err != nil {
				return err
			}
			if file == "" {
				return fmt.Errorf("flag -f/--file is required")
			}

			workflow, err := loadWorkflow(file, *gatewayName, *workspace, applyOverrides{})
			if err != nil {
				return err
			}

			// An empty target means the active/default OpenShell gateway, so use
			// the same factory path as apply. If it cannot be reached, preserve
			// the read-only fallback and render the desired config without
			// claiming that references are absent.
			var client openshell.Client
			client, err = newClient(cmd.Context(), workflow.Target)
			if err != nil {
				desc := targetDescription(workflow.Target)
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s unreachable: %v (rendering desired config only)\n", desc, err)
			} else if client != nil {
				defer client.Close()
			}

			p, _, err := workflow.buildPlan(cmd.Context(), client)
			if err != nil {
				return err
			}
			p = redactedPlan(p, workflow.Desired, workflow.Input)

			if format != formatTable {
				return printStructured(format, p)
			}
			for _, section := range p.TableSections() {
				fmt.Println(strings.ToUpper(section.Title))
				printTable(section.Headers, section.Rows)
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&file, "file", "f", "", "Path to harness YAML (or pass it as the first argument)")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Output format (table, json, yaml)")
	gatewayName, workspace = registerTargetFlags(cmd)

	return cmd
}
