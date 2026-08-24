package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/stackrox/harness-openshell/internal/config"
	"github.com/stackrox/harness-openshell/internal/openshell"
	"github.com/stackrox/harness-openshell/internal/plan"
)

// NewPlanCmd constructs the "harness plan" command.
// It reads a config file, resolves environment variables, connects to the gateway
// (if specified), reads the current state, builds a reconciliation plan, and renders it.
func NewPlanCmd(harnessDir string, newClient openshell.Factory) *cobra.Command {
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

This is a read-only plan and mutates nothing. The legacy apply --dry-run
command is a separate path and is unchanged.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := parseOutputFormat(output)
			if err != nil {
				return err
			}
			if file == "" {
				return fmt.Errorf("flag -f/--file is required")
			}

			h, err := config.Load(file)
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}

			// Resolve env strictly before any gateway contact: a missing ${VAR}
			// must fail the plan without a network round-trip.
			resolved, err := config.Resolve(h, os.Getenv)
			if err != nil {
				return fmt.Errorf("resolving config: %w", err)
			}

			target := openshell.ResolveTarget(*gatewayName, *workspace, resolved.Spec.Target.Gateway, resolved.Spec.Target.Workspace, os.Getenv)

			// An empty or unreachable gateway is a valid read-only plan: render
			// the desired config against empty current state rather than failing.
			var current plan.CurrentState
			if target.Gateway != "" {
				client, err := newClient(cmd.Context(), target)
				if err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: gateway %q unreachable: %v (rendering desired config only)\n", target.Gateway, err)
				} else {
					defer client.Close()
					current, err = plan.ReadCurrentState(cmd.Context(), client, resolved)
					if err != nil {
						return err
					}
				}
			}

			p := plan.Build(resolved, current)

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
