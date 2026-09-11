package cmd

import (
	"context"
	"errors"
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
	OutputDir  string
	ResultFile string
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
func (s applyService) run(ctx context.Context, req applyRequest) (runErr error) {
	if req.File == "" {
		return fmt.Errorf("flag -f/--file is required")
	}
	var result *applyResult
	if req.ResultFile != "" {
		if req.DryRun || req.Output != "" || req.SetupOnly {
			return fmt.Errorf("--result-file requires execution; cannot combine with --dry-run, --output, or --setup-only")
		}
		var file *os.File
		var err error
		result, file, err = startApplyResult(req.ResultFile)
		if err != nil {
			return err
		}
		defer func() { runErr = errors.Join(runErr, result.finish(ctx, file, runErr)) }()
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
	if result != nil && !runConfigured(workflow.Desired) {
		return fmt.Errorf("--result-file requires a workflow with a sandbox run")
	}

	result.setPhase("plan")
	client, planned, current, err := s.connectAndPlan(ctx, workflow, req.DryRun)
	if err != nil {
		return err
	}
	if client != nil {
		defer client.Close()
	}
	return executeResolvedWorkflow(ctx, workflow, planned, current, client, applyOptions{
		SetupOnly: req.SetupOnly, DryRun: req.DryRun, Output: req.Output, OutputDir: req.OutputDir, Result: result,
	})
}

// connectAndPlan connects to the selected target when needed and builds the
// plan used by the subsequent execution step.
func (s applyService) connectAndPlan(ctx context.Context, workflow *resolvedWorkflow, dryRun bool) (openshell.Client, *plan.Plan, plan.CurrentState, error) {
	return connectAndBuildPlan(ctx, s.newClient, workflow, dryRun, s.stderr)
}

// executeResolvedWorkflow runs the fully resolved and planned workflow through
// preflight, reconcile, and optional sandbox execution.
func executeResolvedWorkflow(ctx context.Context, workflow *resolvedWorkflow, p *plan.Plan, current plan.CurrentState, client openshell.Client, opts applyOptions) error {
	if opts.DryRun {
		return renderPlan(redactedPlan(p, workflow.Desired, workflow.Input), opts.Output)
	}
	opts.Result.setPhase("preflight")
	if client == nil || !current.Reachable {
		return fmt.Errorf("%s is not reachable or authenticated", targetDescription(workflow.Target))
	}
	if err := preflightPlan(p); err != nil {
		return err
	}
	if err := verifyProviderReferences(ctx, client, workflow.Desired); err != nil {
		return err
	}

	var req preparedRun
	opts.Result.setPhase("prepare")
	if !opts.SetupOnly && runConfigured(workflow.Desired) {
		var (
			cleanup func()
			err     error
		)
		req, cleanup, err = buildRunRequest(workflow, opts.OutputDir)
		if err != nil {
			return err
		}
		defer cleanup()
		if opts.Result != nil {
			opts.Result.SourceCommit = req.SourceCommit
		}
	}

	opts.Result.setPhase("reconcile")
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
	opts.Result.setPhase("execute")
	return run.Run(ctx, executor, req.SandboxRunRequest, os.Stdin, os.Stdout, os.Stderr)
}
