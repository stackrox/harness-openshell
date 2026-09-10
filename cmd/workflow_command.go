package cmd

import (
	"github.com/spf13/cobra"
	"github.com/stackrox/harness-openshell/internal/openshell"
)

// NewWorkflowCmd groups the commands that operate on repository workflows.
func NewWorkflowCmd(newClient openshell.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workflow",
		Short: "Run repository workflows",
		Long:  "Run repository-owned workflows in OpenShell sandboxes, locally or in CI.",
	}
	cmd.AddCommand(
		NewApplyCmd(newClient),
		NewPlanCmd(newClient),
	)
	return cmd
}
