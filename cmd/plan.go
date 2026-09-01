package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/stackrox/harness-openshell/internal/openshell"
)

// NewPlanCmd constructs the "harness plan" command.
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
		Use:   "plan",
		Short: "Read-only reconciliation plan",
		Long: `Generate a reconciliation plan showing the actions harness would take.

This is a read-only plan and mutates nothing. For a v1alpha1 workflow, apply
uses this same resolved desired object and action-decision engine.`,
		RunE: func(cmd *cobra.Command, args []string) error {
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

			// An empty or unreachable gateway is a valid read-only plan: render
			// the desired config against empty current state rather than failing.
			// A direct target carries its own connection with no CLI gateway name,
			// so connect on that too — otherwise plan silently renders empty
			// current state for a reachable direct registration.
			var client openshell.Client
			if workflow.Target.Direct != nil || workflow.Target.Gateway != "" {
				client, err = newClient(cmd.Context(), workflow.Target)
				if err != nil {
					desc := fmt.Sprintf("gateway %q", workflow.Target.Gateway)
					if workflow.Target.Direct != nil {
						desc = "direct target"
					}
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s unreachable: %v (rendering desired config only)\n", desc, err)
				} else {
					defer client.Close()
				}
			}

			p, _, err := workflow.buildPlan(cmd.Context(), client)
			if err != nil {
				return err
			}

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

	cmd.Flags().StringVarP(&file, "file", "f", "", "Path to harness YAML (required)")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Output format (table, json, yaml)")
	gatewayName, workspace = registerTargetFlags(cmd)

	return cmd
}
