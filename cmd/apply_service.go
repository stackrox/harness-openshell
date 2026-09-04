package cmd

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/stackrox/harness-openshell/internal/openshell"
	"github.com/stackrox/harness-openshell/internal/plan"
	"github.com/stackrox/harness-openshell/internal/reconcile"
	"github.com/stackrox/harness-openshell/internal/run"
	"github.com/stackrox/harness-openshell/internal/status"
)

type applyRequest struct {
	File       string
	Name       string
	Entrypoint string
	Attach     bool
	DryRun     bool
	SetupOnly  bool
	Output     string
	Gateway    string
	Workspace  string
}

type applyService struct {
	newClient openshell.Factory
	stderr    io.Writer
}

// runApply executes an apply request through the service layer.
func runApply(ctx context.Context, newClient openshell.Factory, req applyRequest, stderr io.Writer) error {
	return applyService{newClient: newClient, stderr: stderr}.run(ctx, req)
}

// run loads, resolves, plans, and executes one workflow request.
func (s applyService) run(ctx context.Context, req applyRequest) error {
	if req.File == "" {
		return fmt.Errorf("flag -f/--file is required")
	}

	workflow, err := loadWorkflow(req.File, req.Gateway, req.Workspace, applyOverrides{
		Name: req.Name, AgentType: req.Entrypoint, ForceTTY: req.Attach,
	})
	if err != nil {
		return err
	}
	if req.Output != "" && !req.DryRun {
		return renderWorkflow(workflow, req.Output)
	}

	client, planned, current, err := s.connectAndPlan(ctx, workflow, req.DryRun)
	if err != nil {
		return err
	}
	if client != nil {
		defer client.Close()
	}
	return executeResolvedWorkflow(ctx, workflow, planned, current, client, applyOptions{
		SetupOnly: req.SetupOnly, DryRun: req.DryRun, Output: req.Output,
	})
}

// connectAndPlan connects to the selected target when needed and builds the
// plan used by the subsequent execution step.
func (s applyService) connectAndPlan(ctx context.Context, workflow *resolvedWorkflow, dryRun bool) (openshell.Client, *plan.Plan, plan.CurrentState, error) {
	var (
		client openshell.Client
		err    error
	)
	if !dryRun || workflow.Target.Direct != nil || workflow.Target.Gateway != "" {
		client, err = s.newClient(ctx, workflow.Target)
		if err != nil {
			desc := targetDescription(workflow.Target)
			if !dryRun {
				return nil, nil, plan.CurrentState{}, fmt.Errorf("connecting to %s: %w", desc, err)
			}
			out := s.stderr
			if out == nil {
				out = io.Discard
			}
			fmt.Fprintf(out, "warning: %s unreachable: %v (rendering desired config only)\n", desc, err)
		}
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

// executeResolvedWorkflow runs the fully resolved and planned workflow through
// preflight, reconcile, and optional sandbox execution.
func executeResolvedWorkflow(ctx context.Context, workflow *resolvedWorkflow, p *plan.Plan, current plan.CurrentState, client openshell.Client, opts applyOptions) error {
	if opts.DryRun {
		return renderPlan(p, opts.Output)
	}
	if client == nil || !current.Reachable {
		return fmt.Errorf("%s is not reachable or authenticated", targetDescription(workflow.Target))
	}
	if err := preflightPlan(workflow.Desired, p); err != nil {
		return err
	}
	if err := verifySandboxProviders(ctx, client, workflow.Desired); err != nil {
		return err
	}

	var req run.SandboxRunRequest
	if !opts.SetupOnly && runConfigured(workflow.Desired) {
		var (
			cleanup func()
			err     error
		)
		req, cleanup, err = buildRunRequest(workflow)
		if err != nil {
			return err
		}
		defer cleanup()
	}

	if err := reconcileProviders(ctx, client, workflow.Desired.Spec.Providers); err != nil {
		return err
	}
	if inferenceConfigured(workflow.Desired.Spec.Inference) {
		result, err := reconcile.ReconcileInference(ctx, client, workflow.Desired.Spec.Inference)
		if err != nil {
			return fmt.Errorf("reconciling inference: %w", err)
		}
		status.OKf("inference: %s (model %s)", result.Action, workflow.Desired.Spec.Inference.Model)
	}
	if opts.SetupOnly {
		status.OK("Setup complete (--setup-only): skipping sandbox creation")
		return nil
	}
	if !runConfigured(workflow.Desired) {
		status.OK("Reconciliation complete: workflow declares no sandbox run")
		return nil
	}
	executor, ok := client.(openshell.SandboxExecutionClient)
	if !ok {
		return fmt.Errorf("configured OpenShell client does not support SDK sandbox execution")
	}
	return run.Run(ctx, executor, req, os.Stdin, os.Stdout, os.Stderr)
}
