package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/stackrox/harness-openshell/internal/config"
	"github.com/stackrox/harness-openshell/internal/openshell"
	"github.com/stackrox/harness-openshell/internal/plan"
)

// resolvedWorkflow is the single resolved input shared by plan and apply.
// Target precedence and execution defaults are applied before either
// command reads gateway state, so both commands build their action graph from
// the same desired object.
type resolvedWorkflow struct {
	Desired *config.Harness
	Input   *config.Harness
	Target  openshell.Target
	BaseDir string
}

// applyOverrides are apply-only overrides. They are applied to the desired
// object before planning, so apply never executes a value absent from its plan.
type applyOverrides struct {
	Name      string
	AgentType string
	ForceTTY  bool
}

func loadWorkflow(path, flagGateway, flagWorkspace string, overrides applyOverrides) (*resolvedWorkflow, error) {
	h, err := config.Load(path)
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}
	resolved, err := config.Resolve(h, os.Getenv)
	if err != nil {
		return nil, fmt.Errorf("resolving config: %w", err)
	}

	target := openshell.ResolveTarget(flagGateway, flagWorkspace, resolved.Spec.Target.Gateway, resolved.Spec.Target.Workspace, os.Getenv)
	externalGateway := flagGateway != "" || os.Getenv(openshell.EnvGateway) != ""
	if registration := resolved.Spec.Target.Registration; registration != nil && !externalGateway {
		target.Direct = &openshell.DirectConnection{
			Endpoint: registration.Endpoint,
			OIDC: openshell.OIDCConnection{
				Issuer:   registration.OIDC.Issuer,
				ClientID: registration.OIDC.ClientID,
				Audience: registration.OIDC.Audience,
			},
		}
	}
	// Store the effective target in the resolved desired object so plan and apply
	// consume the same value.
	resolved.Spec.Target.Gateway = target.Gateway
	resolved.Spec.Target.Workspace = target.Workspace

	if overrides.Name != "" {
		resolved.Metadata.Name = overrides.Name
	}
	if overrides.AgentType != "" {
		resolved.Spec.Agent.Type = overrides.AgentType
	}
	if overrides.ForceTTY {
		resolved.Spec.Sandbox.TTY = true
	}
	// Apply the image default only to workflows that declare a run; provider- or
	// inference-only workflows remain setup-only.
	if runConfigured(resolved) {
		resolved.Spec.Sandbox.Image = resolveSandboxImage(resolved.Spec.Sandbox.Image)
	}

	return &resolvedWorkflow{
		Desired: resolved,
		Input:   h,
		Target:  target,
		BaseDir: filepath.Dir(path),
	}, nil
}

func runConfigured(desired *config.Harness) bool {
	sandbox := desired.Spec.Sandbox
	return sandbox.Image != "" || len(sandbox.Providers) > 0 || sandbox.Policy != nil || len(sandbox.Env) > 0 || sandbox.TTY || sandbox.Keep ||
		desired.Spec.Agent.Type != "" || desired.Spec.Source.Repo != "" || len(desired.Spec.Payloads) > 0
}

func (w *resolvedWorkflow) buildPlan(ctx context.Context, client openshell.Client) (*plan.Plan, plan.CurrentState, error) {
	var current plan.CurrentState
	if client != nil {
		var err error
		current, err = plan.ReadCurrentState(ctx, client, w.Desired)
		if err != nil {
			return nil, current, err
		}
	}
	return plan.Build(w.Desired, current), current, nil
}
