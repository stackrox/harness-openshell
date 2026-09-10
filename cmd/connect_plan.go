package cmd

import (
	"context"
	"fmt"
	"io"

	"github.com/stackrox/harness-openshell/internal/openshell"
	"github.com/stackrox/harness-openshell/internal/plan"
)

// connectAndBuildPlan uses the same target connection and offline fallback for
// plan and apply --dry-run. A failed read-only connection is represented by an
// uninspected plan; it must not be rendered as a confirmed missing provider.
func connectAndBuildPlan(
	ctx context.Context,
	newClient openshell.Factory,
	workflow *resolvedWorkflow,
	dryRun bool,
	stderr io.Writer,
) (openshell.Client, *plan.Plan, plan.CurrentState, error) {
	client, err := newClient(ctx, workflow.Target)
	if err != nil {
		desc := targetDescription(workflow.Target)
		if !dryRun {
			return nil, nil, plan.CurrentState{}, fmt.Errorf("connecting to %s: %w", desc, err)
		}
		if stderr == nil {
			stderr = io.Discard
		}
		fmt.Fprintf(stderr, "warning: %s unreachable: %v (rendering desired config only)\n", desc, err)
	}

	planned, current, err := workflow.buildPlan(ctx, client)
	if err != nil {
		if client != nil {
			_ = client.Close()
		}
		return nil, nil, plan.CurrentState{}, err
	}
	return client, planned, current, nil
}
