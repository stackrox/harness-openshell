package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/stackrox/harness-openshell/internal/openshell"
)

func NewDeleteCmd(newClient openshell.Factory) *cobra.Command {
	var sandboxes bool
	var gatewayName, workspace *string

	cmd := &cobra.Command{
		Use:   "delete [NAME...] [--sandboxes]",
		Short: "Delete sandboxes",
		Long: `Delete specific sandboxes by name, or use --sandboxes to delete all
sandboxes in the selected workspace.

Examples:
  harness delete my-sandbox       Delete a specific sandbox
  harness delete agent test       Delete multiple sandboxes
  harness delete --sandboxes      Delete all sandboxes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && !sandboxes {
				return fmt.Errorf("specify sandbox name(s) or use --sandboxes")
			}
			if len(args) > 0 && sandboxes {
				return fmt.Errorf("sandbox names cannot be combined with --sandboxes")
			}

			ctx := cmd.Context()
			target := openshell.ResolveTarget(*gatewayName, *workspace, "", "", os.Getenv)
			client, err := newClient(ctx, target)
			if err != nil {
				return fmt.Errorf("create OpenShell client: %w", err)
			}
			defer client.Close()

			return deleteSandboxes(ctx, client, args, sandboxes)
		},
	}

	cmd.Flags().BoolVar(&sandboxes, "sandboxes", false, "Delete all sandboxes")
	gatewayName, workspace = registerTargetFlags(cmd)

	return cmd
}

func deleteSandboxes(ctx context.Context, client openshell.Client, names []string, all bool) error {
	if all {
		sandboxes, err := client.Sandboxes(ctx)
		if err != nil {
			return fmt.Errorf("listing sandboxes: %w", err)
		}
		names = make([]string, len(sandboxes))
		for i, sandbox := range sandboxes {
			names[i] = sandbox.Name
		}
	}

	var errs []error
	for _, name := range names {
		if err := client.DeleteSandbox(ctx, name); err != nil {
			errs = append(errs, fmt.Errorf("deleting sandbox %q: %w", name, err))
		}
	}
	return errors.Join(errs...)
}
